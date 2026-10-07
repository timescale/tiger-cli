package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/fs"
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
// fixture skills into dir, with removed holding any lines reporting removed
// skills.
func skillsInstallOutput(dir, clientName string, removed ...string) string {
	return fmt.Sprintf("Installed 2 skills to %s:\n  alpha\n  beta\n%s\nRestart %s to load the new skills.\n",
		dir, strings.Join(removed, ""), clientName)
}

func TestSkillsInstallCmd(t *testing.T) {
	// The fixture repo exercises skill discovery and symlink resolution:
	// file and directory symlinks between skills (resolved), and ones that
	// leave skills/, are absolute, dangle, or loop (skipped), plus entries
	// that aren't skills.
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
		tarEntry{name: "skills/beta/references/loop-dir", link: "."},
		tarEntry{name: "skills/no-skill-md/README.md", body: "no SKILL.md"},
	)
	// installedSkills is the tree the fixture installs into dir (relative to
	// the home directory).
	installedSkills := func(dir string) map[string]string {
		return map[string]string{
			dir + "/alpha/.tiger-cli":                    skillMarkerContent,
			dir + "/alpha/SKILL.md":                      "alpha skill",
			dir + "/alpha/references/guide.md":           "alpha guide",
			dir + "/alpha/scripts/run.sh":                "[executable] #!/bin/sh",
			dir + "/beta/.tiger-cli":                     skillMarkerContent,
			dir + "/beta/SKILL.md":                       "beta skill",
			dir + "/beta/references/alpha-guide.md":      "alpha guide",
			dir + "/beta/references/alpha-refs/guide.md": "alpha guide",
			// A directory symlink cycle is followed once, then cut off.
			dir + "/beta/references/loop-dir/alpha-guide.md":      "alpha guide",
			dir + "/beta/references/loop-dir/alpha-refs/guide.md": "alpha guide",
		}
	}

	codexHome := t.TempDir()
	claudeHome := t.TempDir()
	claudeConfigHome := t.TempDir()
	claudeEmptyEnvHome := t.TempDir()
	claudeConfigDir := filepath.Join(claudeConfigHome, "claude-config")
	kiroHome := t.TempDir()
	antigravityHome := t.TempDir()
	aliasHome := t.TempDir()

	// reinstallHome holds a previous install: one of our skills with a stale
	// file (replaced), one no longer available (removed), skills of the
	// user's, hidden or not, and a symlink of the user's to a skill of ours
	// (kept), and both kinds of temporary directory left by an install that
	// was killed (quietly removed).
	reinstallHome := t.TempDir()
	writeTree(t, reinstallHome, map[string]string{
		".claude/skills/alpha/.tiger-cli":              skillMarkerContent,
		".claude/skills/alpha/SKILL.md":                "old alpha",
		".claude/skills/alpha/stale.md":                "stale",
		".claude/skills/retired/.tiger-cli":            skillMarkerContent,
		".claude/skills/retired/SKILL.md":              "retired skill",
		".claude/skills/other/SKILL.md":                "user's skill",
		".claude/skills/.tiger-beta-123/.tiger-cli":    skillMarkerContent,
		".claude/skills/.tiger-beta-123/SKILL.md":      "leftover",
		".claude/skills/.tiger-old-456/old/.tiger-cli": skillMarkerContent,
		".claude/skills/.tiger-old-456/old/SKILL.md":   "leftover",
		".claude/skills/.hidden/SKILL.md":              "user's hidden entry",
		".claude/skills/linked":                        "-> " + filepath.Join(reinstallHome, "marked"),
		"marked/.tiger-cli":                            skillMarkerContent,
		"marked/SKILL.md":                              "marked skill",
	})
	reinstallTree := installedSkills(".claude/skills")
	reinstallTree[".claude/skills/other/SKILL.md"] = "user's skill"
	reinstallTree[".claude/skills/.hidden/SKILL.md"] = "user's hidden entry"
	reinstallTree[".claude/skills/linked"] = "-> " + filepath.Join(reinstallHome, "marked")
	reinstallTree["marked/.tiger-cli"] = skillMarkerContent
	reinstallTree["marked/SKILL.md"] = "marked skill"

	// conflictTree holds a symlink and a directory of the user's under the
	// names of skills being installed, which aren't replaced without --force.
	// The symlink counts as the user's even though it points at one of our
	// skills, since Tiger CLI never creates symlinks.
	conflictHome := t.TempDir()
	conflictTree := map[string]string{
		".agents/skills/alpha":         "-> " + filepath.Join(conflictHome, "marked"),
		".agents/skills/beta/SKILL.md": "user's beta",
		"marked/.tiger-cli":            skillMarkerContent,
		"marked/SKILL.md":              "marked skill",
	}
	writeTree(t, conflictHome, conflictTree)

	// forceHome holds a symlink and a directory of the user's under the names
	// of skills being installed. --force replaces both; the symlink is
	// replaced as a link, leaving its target alone.
	forceHome := t.TempDir()
	writeTree(t, forceHome, map[string]string{
		".agents/skills/alpha":         "-> " + filepath.Join(forceHome, "elsewhere"),
		".agents/skills/beta/SKILL.md": "user's beta",
		".agents/skills/beta/stale.md": "stale",
		"elsewhere/SKILL.md":           "symlink target",
	})
	forceTree := installedSkills(".agents/skills")
	forceTree["elsewhere/SKILL.md"] = "symlink target"

	customHome := t.TempDir()

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
			wantErr: "no skills found",
		},
		{
			name:    "existing skills not installed by Tiger CLI",
			args:    []string{"skills", "install", "codex"},
			opts:    []runOption{withEnv("HOME", conflictHome), withSkillsTarball(tarball)},
			wantErr: fmt.Sprintf("skills already exist in %s and weren't installed by Tiger CLI: alpha, beta. Use --force to replace them", filepath.Join(conflictHome, ".agents", "skills")),
			checks:  []checkFunc{checkTree(conflictHome, conflictTree)},
		},
		{
			name:       "installs to ~/.agents/skills",
			args:       []string{"skills", "install", "codex"},
			opts:       []runOption{withEnv("HOME", codexHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(filepath.Join(codexHome, ".agents", "skills"), "Codex"),
			checks:     []checkFunc{checkTree(codexHome, installedSkills(".agents/skills"))},
		},
		{
			name:       "installs to claude code's skills directory",
			args:       []string{"skills", "install", "claude-code"},
			opts:       []runOption{withEnv("HOME", claudeHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(filepath.Join(claudeHome, ".claude", "skills"), "Claude Code"),
			checks:     []checkFunc{checkTree(claudeHome, installedSkills(".claude/skills"))},
		},
		{
			name: "respects CLAUDE_CONFIG_DIR",
			args: []string{"skills", "install", "claude-code"},
			opts: []runOption{
				withEnv("HOME", claudeConfigHome),
				withEnv("CLAUDE_CONFIG_DIR", claudeConfigDir),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput(filepath.Join(claudeConfigDir, "skills"), "Claude Code"),
			checks:     []checkFunc{checkTree(claudeConfigHome, installedSkills("claude-config/skills"))},
		},
		{
			name: "empty CLAUDE_CONFIG_DIR counts as unset",
			args: []string{"skills", "install", "claude-code"},
			opts: []runOption{
				withEnv("HOME", claudeEmptyEnvHome),
				withEnv("CLAUDE_CONFIG_DIR", ""),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput(filepath.Join(claudeEmptyEnvHome, ".claude", "skills"), "Claude Code"),
			checks:     []checkFunc{checkTree(claudeEmptyEnvHome, installedSkills(".claude/skills"))},
		},
		{
			name:       "installs to kiro's skills directory",
			args:       []string{"skills", "install", "kiro-cli"},
			opts:       []runOption{withEnv("HOME", kiroHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(filepath.Join(kiroHome, ".kiro", "skills"), "Kiro CLI"),
			checks:     []checkFunc{checkTree(kiroHome, installedSkills(".kiro/skills"))},
		},
		{
			name:       "installs to antigravity's skills directory",
			args:       []string{"skills", "install", "antigravity"},
			opts:       []runOption{withEnv("HOME", antigravityHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(filepath.Join(antigravityHome, ".gemini", "antigravity", "skills"), "Google Antigravity"),
			checks:     []checkFunc{checkTree(antigravityHome, installedSkills(".gemini/antigravity/skills"))},
		},
		{
			name: "reinstall replaces our skills and removes ones no longer available",
			args: []string{"skills", "install", "claude-code"},
			opts: []runOption{withEnv("HOME", reinstallHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(filepath.Join(reinstallHome, ".claude", "skills"), "Claude Code",
				"Removed retired (no longer available).\n"),
			checks: []checkFunc{
				checkTree(reinstallHome, reinstallTree),
				// Running again over a current install leaves it unchanged.
				func(t *testing.T, _ cmdResult) {
					again := runCommand(t, []string{"skills", "install", "claude-code"}, nil)
					if again.err != nil {
						t.Fatalf("second install failed: %v", again.err)
					}
					assertOutput(t, again.stdout, skillsInstallOutput(filepath.Join(reinstallHome, ".claude", "skills"), "Claude Code"))
				},
				checkTree(reinstallHome, reinstallTree),
			},
		},
		{
			name:       "--force replaces skills not installed by Tiger CLI",
			args:       []string{"skills", "install", "codex", "--force"},
			opts:       []runOption{withEnv("HOME", forceHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(filepath.Join(forceHome, ".agents", "skills"), "Codex"),
			checks:     []checkFunc{checkTree(forceHome, forceTree)},
		},
		{
			name:       "--skills-dir overrides the client's directory",
			args:       []string{"skills", "install", "claude-code", "--skills-dir", "~/custom"},
			opts:       []runOption{withEnv("HOME", customHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(filepath.Join(customHome, "custom"), "Claude Code"),
			checks:     []checkFunc{checkTree(customHome, installedSkills("custom"))},
		},
		{
			name:       "add alias and case-insensitive client name",
			args:       []string{"skills", "add", "CODEX"},
			opts:       []runOption{withEnv("HOME", aliasHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput(filepath.Join(aliasHome, ".agents", "skills"), "Codex"),
			checks:     []checkFunc{checkTree(aliasHome, installedSkills(".agents/skills"))},
		},
	})
}
