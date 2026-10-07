package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// tarEntry is one entry of a test tarball, named relative to the repo root.
type tarEntry struct {
	name string
	body string
	link string // symlink target, if set
	mode int64  // file mode; 0o644 if unset
}

// skillsTarball builds a gzipped tarball shaped like GitHub's: a pax global
// header followed by everything nested under a top-level directory named after
// the repo and commit.
func skillsTarball(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	headers := []*tar.Header{
		{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "abc123"}},
		{Name: "timescale-pg-aiguide-abc123/", Typeflag: tar.TypeDir, Mode: 0o755},
	}
	for _, e := range headers {
		if err := tw.WriteHeader(e); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range entries {
		hdr := &tar.Header{Name: "timescale-pg-aiguide-abc123/" + e.name, Mode: e.mode}
		if hdr.Mode == 0 {
			hdr.Mode = 0o644
		}
		switch {
		case e.link != "":
			hdr.Typeflag = tar.TypeSymlink
			hdr.Linkname = e.link
		case e.name[len(e.name)-1] == '/':
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0o755
		default:
			hdr.Typeflag = tar.TypeReg
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withSkillsServer points the skills download at a local server standing in
// for GitHub. The server expects the tarball API request, then answers it
// with handle.
func withSkillsServer(handle http.HandlerFunc) runOption {
	return withSkillsServerDownload(handle, nil)
}

// withSkillsTarball serves body as the repo tarball, via a redirect to a
// separate download URL as GitHub does.
func withSkillsTarball(body []byte) runOption {
	return withSkillsServerDownload(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/download", http.StatusFound)
	}, body)
}

// withSkillsServerDownload is withSkillsServer, also serving download at
// /download if set.
func withSkillsServerDownload(handle http.HandlerFunc, download []byte) runOption {
	return withSetup(func(t *testing.T) {
		mux := http.NewServeMux()
		if download != nil {
			mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
				w.Write(download)
			})
		}
		mux.HandleFunc("/repos/timescale/pg-aiguide/tarball/main", func(w http.ResponseWriter, r *http.Request) {
			got := map[string]string{
				"method":               r.Method,
				"Accept":               r.Header.Get("Accept"),
				"X-GitHub-Api-Version": r.Header.Get("X-GitHub-Api-Version"),
			}
			want := map[string]string{
				"method":               http.MethodGet,
				"Accept":               "application/vnd.github+json",
				"X-GitHub-Api-Version": "2022-11-28",
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("tarball request mismatch (-want +got):\n%s", diff)
			}
			handle(w, r)
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		original := skillsTarballURL
		skillsTarballURL = srv.URL + "/repos/timescale/pg-aiguide/tarball/main"
		t.Cleanup(func() { skillsTarballURL = original })
	})
}

// readTree returns every file and symlink under root, keyed by slash-separated
// path. Files map to their content (prefixed with "[executable] " if so) and
// symlinks to "-> target". Symlinks are not followed.
func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			tree[rel] = "-> " + target
		case d.Type().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode().Perm()&0o111 != 0 {
				tree[rel] = "[executable] " + string(data)
			} else {
				tree[rel] = string(data)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to read tree %s: %v", root, err)
	}
	return tree
}

// checkTree asserts the exact files and symlinks under root (see readTree).
func checkTree(root string, want map[string]string) checkFunc {
	return func(t *testing.T, _ cmdResult) {
		t.Helper()
		if diff := cmp.Diff(want, readTree(t, root)); diff != "" {
			t.Errorf("tree %s mismatch (-want +got):\n%s", root, diff)
		}
	}
}

// writeTree creates the given files and symlinks under root (see readTree for
// the format, minus executable bits).
func writeTree(t *testing.T, root string, tree map[string]string) {
	t.Helper()
	for rel, content := range tree {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if target, ok := strings.CutPrefix(content, "-> "); ok {
			if err := os.Symlink(target, p); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// skillsInstallOutput is the exact stdout of a successful install of the
// fixture skills. linkLine is the line describing the client directory, if
// any.
func skillsInstallOutput(home, linkLine, clientName string) string {
	return fmt.Sprintf("Installed 2 skills to %s:\n  alpha\n  beta\n%s\nRestart %s to load the new skills.\n",
		filepath.Join(home, ".agents", "skills"), linkLine, clientName)
}

func TestSkillsInstallCmd(t *testing.T) {
	// The fixture repo exercises skill discovery and symlink resolution:
	// file and directory symlinks between skills, one that climbs out of
	// skills/ but stays in the repo, and absolute, dangling, and looping ones
	// (skipped), plus entries that aren't valid skills.
	tarball := skillsTarball(t,
		tarEntry{name: "README.md", body: "repo readme"},
		tarEntry{name: "skills/"},
		tarEntry{name: "skills/README.md", body: "not a skill"},
		tarEntry{name: "skills/alpha/"},
		tarEntry{name: "skills/alpha/SKILL.md", body: "alpha skill"},
		tarEntry{name: "skills/alpha/references/guide.md", body: "alpha guide"},
		tarEntry{name: "skills/alpha/scripts/run.sh", body: "#!/bin/sh", mode: 0o755},
		tarEntry{name: "skills/beta/SKILL.md", body: "beta skill"},
		tarEntry{name: "skills/beta/references/alpha-guide.md", link: "../../alpha/references/guide.md"},
		tarEntry{name: "skills/beta/references/alpha-refs", link: "../../alpha/references"},
		tarEntry{name: "skills/beta/references/readme.md", link: "../../../README.md"},
		tarEntry{name: "skills/beta/references/absolute.md", link: "/etc/passwd"},
		tarEntry{name: "skills/beta/references/dangling.md", link: "missing.md"},
		tarEntry{name: "skills/beta/references/loop.md", link: "loop.md"},
		tarEntry{name: "skills/Bad_Name/SKILL.md", body: "invalid name"},
		tarEntry{name: "skills/no-skill-md/README.md", body: "no SKILL.md"},
	)
	// installedSkills is the tree the fixture installs under ~/.agents/skills.
	installedSkills := map[string]string{
		".agents/skills/alpha/SKILL.md":                      "alpha skill",
		".agents/skills/alpha/references/guide.md":           "alpha guide",
		".agents/skills/alpha/scripts/run.sh":                "[executable] #!/bin/sh",
		".agents/skills/beta/SKILL.md":                       "beta skill",
		".agents/skills/beta/references/alpha-guide.md":      "alpha guide",
		".agents/skills/beta/references/alpha-refs/guide.md": "alpha guide",
		".agents/skills/beta/references/readme.md":           "repo readme",
	}
	// withLinks returns installedSkills plus symlinks to each skill from the
	// client directory linkDir (relative to home).
	withLinks := func(home, linkDir string) map[string]string {
		tree := map[string]string{
			linkDir + "/alpha": "-> " + filepath.Join(home, ".agents", "skills", "alpha"),
			linkDir + "/beta":  "-> " + filepath.Join(home, ".agents", "skills", "beta"),
		}
		maps.Copy(tree, installedSkills)
		return tree
	}

	codexHome := t.TempDir()
	claudeHome := t.TempDir()
	claudeConfigHome := t.TempDir()
	claudeConfigDir := filepath.Join(claudeConfigHome, "claude-config")
	kiroHome := t.TempDir()
	antigravityHome := t.TempDir()
	aliasHome := t.TempDir()

	// reinstallHome holds a previous install to be replaced: a stale skill
	// directory, a client entry that's a real directory, a client symlink
	// pointing elsewhere, and an unrelated skill that must be left alone.
	reinstallHome := t.TempDir()
	writeTree(t, reinstallHome, map[string]string{
		".agents/skills/alpha/SKILL.md": "old alpha",
		".agents/skills/alpha/stale.md": "stale",
		".agents/skills/other/SKILL.md": "unrelated skill",
		".claude/skills/alpha":          "-> /somewhere/else/alpha",
		".claude/skills/beta/SKILL.md":  "old beta copy",
	})
	reinstallTree := withLinks(reinstallHome, ".claude/skills")
	reinstallTree[".agents/skills/other/SKILL.md"] = "unrelated skill"

	// sharedHome has ~/.claude/skills symlinked to ~/.agents/skills, so the
	// skills are already visible to Claude Code and no per-skill links are made.
	sharedHome := t.TempDir()
	writeTree(t, sharedHome, map[string]string{
		".claude/skills": "-> " + filepath.Join(sharedHome, ".agents", "skills"),
	})
	sharedTree := map[string]string{".claude/skills": "-> " + filepath.Join(sharedHome, ".agents", "skills")}
	maps.Copy(sharedTree, installedSkills)

	rateLimitReset := time.Date(2030, 1, 1, 15, 4, 0, 0, time.UTC).Unix()

	runCmdTests(t, []cmdTest{
		{
			name:    "too many arguments",
			args:    []string{"skills", "install", "codex", "cursor"},
			wantErr: "accepts at most 1 arg(s), received 2",
		},
		{
			name:    "no client and no TTY",
			args:    []string{"skills", "install"},
			wantErr: "TTY not detected - specify a client as an argument (e.g. 'tiger skills install claude-code')",
		},
		{
			name:    "unsupported client",
			args:    []string{"skills", "install", "bogus"},
			wantErr: "unsupported client: bogus. Supported clients: claude-code, cursor, devin, codex, gemini, gemini-cli, vscode, code, vs-code, antigravity, agy, kiro-cli, copilot, copilot-cli",
		},
		{
			name: "download fails",
			args: []string{"skills", "install", "codex"},
			opts: []runOption{
				withEnv("HOME", t.TempDir()),
				withSkillsServer(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}),
			},
			wantErr: "failed to download skills: unexpected status from GitHub: 500 Internal Server Error",
		},
		{
			name: "rate limited",
			args: []string{"skills", "install", "codex"},
			opts: []runOption{
				withEnv("HOME", t.TempDir()),
				withSkillsServer(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("X-RateLimit-Remaining", "0")
					w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(rateLimitReset, 10))
					w.WriteHeader(http.StatusForbidden)
				}),
			},
			wantErr: "failed to download skills: GitHub API rate limit exceeded. Try again after 15:04 UTC",
		},
		{
			name: "invalid archive",
			args: []string{"skills", "install", "codex"},
			opts: []runOption{
				withEnv("HOME", t.TempDir()),
				withSkillsServer(func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte("not a tarball"))
				}),
			},
			wantErr: "failed to read skills archive: gzip: invalid header",
		},
		{
			name: "no skills in archive",
			args: []string{"skills", "install", "codex"},
			opts: []runOption{
				withEnv("HOME", t.TempDir()),
				withSkillsTarball(skillsTarball(t, tarEntry{name: "README.md", body: "repo readme"})),
			},
			wantErr: "no skills found in timescale/pg-aiguide",
		},
		{
			name:       "installs to ~/.agents/skills",
			args:       []string{"skills", "install", "codex"},
			opts:       []runOption{withEnv("HOME", codexHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(codexHome, "", "Codex"),
			checks:     []checkFunc{checkTree(codexHome, installedSkills)},
		},
		{
			name: "symlinks into claude code's skills directory",
			args: []string{"skills", "install", "claude-code"},
			opts: []runOption{withEnv("HOME", claudeHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(claudeHome,
				fmt.Sprintf("Linked skills into %s.\n", filepath.Join(claudeHome, ".claude", "skills")), "Claude Code"),
			checks: []checkFunc{checkTree(claudeHome, withLinks(claudeHome, ".claude/skills"))},
		},
		{
			name: "respects CLAUDE_CONFIG_DIR",
			args: []string{"skills", "install", "claude-code"},
			opts: []runOption{
				withEnv("HOME", claudeConfigHome),
				withEnv("CLAUDE_CONFIG_DIR", claudeConfigDir),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput(claudeConfigHome,
				fmt.Sprintf("Linked skills into %s.\n", filepath.Join(claudeConfigDir, "skills")), "Claude Code"),
			checks: []checkFunc{checkTree(claudeConfigHome, withLinks(claudeConfigHome, "claude-config/skills"))},
		},
		{
			name: "symlinks into kiro's skills directory",
			args: []string{"skills", "install", "kiro-cli"},
			opts: []runOption{withEnv("HOME", kiroHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(kiroHome,
				fmt.Sprintf("Linked skills into %s.\n", filepath.Join(kiroHome, ".kiro", "skills")), "Kiro CLI"),
			checks: []checkFunc{checkTree(kiroHome, withLinks(kiroHome, ".kiro/skills"))},
		},
		{
			name: "symlinks into antigravity's skills directory",
			args: []string{"skills", "install", "antigravity"},
			opts: []runOption{withEnv("HOME", antigravityHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(antigravityHome,
				fmt.Sprintf("Linked skills into %s.\n", filepath.Join(antigravityHome, ".gemini", "antigravity", "skills")), "Google Antigravity"),
			checks: []checkFunc{checkTree(antigravityHome, withLinks(antigravityHome, ".gemini/antigravity/skills"))},
		},
		{
			name: "reinstall replaces existing skills and links",
			args: []string{"skills", "install", "claude-code"},
			opts: []runOption{withEnv("HOME", reinstallHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(reinstallHome,
				fmt.Sprintf("Linked skills into %s.\n", filepath.Join(reinstallHome, ".claude", "skills")), "Claude Code"),
			checks: []checkFunc{
				checkTree(reinstallHome, reinstallTree),
				// Running again over a current install leaves it unchanged.
				func(t *testing.T, _ cmdResult) {
					again := runCommand(t, []string{"skills", "install", "claude-code"}, nil)
					if again.err != nil {
						t.Fatalf("second install failed: %v", again.err)
					}
				},
				checkTree(reinstallHome, reinstallTree),
			},
		},
		{
			name:       "client directory already shared with ~/.agents/skills",
			args:       []string{"skills", "install", "claude-code"},
			opts:       []runOption{withEnv("HOME", sharedHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(sharedHome, "", "Claude Code"),
			checks:     []checkFunc{checkTree(sharedHome, sharedTree)},
		},
		{
			name:       "add alias and case-insensitive client name",
			args:       []string{"skills", "add", "CODEX"},
			opts:       []runOption{withEnv("HOME", aliasHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(aliasHome, "", "Codex"),
			checks:     []checkFunc{checkTree(aliasHome, installedSkills)},
		},
	})
}
