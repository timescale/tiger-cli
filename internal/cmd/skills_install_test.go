package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
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
// fixture skills into dirs, with removed holding any sections reporting
// removed skills.
func skillsInstallOutput(dirs []string, removed ...string) string {
	var b strings.Builder
	b.WriteString("Installed 2 skills:\n  alpha\n  beta\n\nInstalled to:\n")
	for _, dir := range dirs {
		b.WriteString("  " + dir + "\n")
	}
	for _, r := range removed {
		b.WriteString("\n" + r)
	}
	b.WriteString("\nRestart your coding agents to load the new skills.\n")
	return b.String()
}

// withRunSkillsPicker stubs the interactive install location picker: it
// expects to be offered exactly want, and answers with choose. A nil want
// expects the picker not to be shown at all.
func withRunSkillsPicker(want []skillsPickerItem, choose []bool) runOption {
	return withSetup(func(t *testing.T) {
		original := runSkillsPicker
		runSkillsPicker = func(_ *cobra.Command, _ string, items []skillsPickerItem) ([]bool, error) {
			if want == nil {
				t.Error("install location picker shown, want none")
				return nil, errors.New("picker shown")
			}
			if diff := cmp.Diff(want, items, cmp.AllowUnexported(skillsPickerItem{})); diff != "" {
				t.Errorf("picker items mismatch (-want +got):\n%s", diff)
			}
			return choose, nil
		}
		t.Cleanup(func() { runSkillsPicker = original })
	})
}

// pickerItems returns the items the picker offers with the default install
// locations (the home directory shown as ~), selecting those in selected.
func pickerItems(selected ...string) []skillsPickerItem {
	items := []skillsPickerItem{
		{label: "Universal", dir: "~/.agents/skills"},
		{label: "Claude Code", dir: "~/.claude/skills"},
		{label: "Google Antigravity", dir: "~/.gemini/config/skills"},
		{label: "Kiro CLI", dir: "~/.kiro/skills"},
	}
	for i := range items {
		items[i].selected = slices.Contains(selected, items[i].label)
	}
	return items
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

	// home creates a home directory seeded with tree (see writeTree).
	home := func(tree map[string]string) string {
		dir := t.TempDir()
		writeTree(t, dir, tree)
		return dir
	}
	// withTrees merges trees into one.
	withTrees := func(trees ...map[string]string) map[string]string {
		merged := map[string]string{}
		for _, tree := range trees {
			maps.Copy(merged, tree)
		}
		return merged
	}

	universalHome := home(nil)
	claudeHome := home(nil)
	multiHome := home(nil)
	claudeConfigHome := home(nil)
	claudeConfigDir := filepath.Join(claudeConfigHome, "claude-config")
	claudeEmptyEnvHome := home(nil)
	claudeRelativeEnvHome := home(nil)
	claudePickerEnvHome := home(nil)
	antigravityHome := home(nil)
	aliasHome := home(nil)
	universalPickerHome := home(nil)
	customHome := home(nil)

	// existingClaude is an earlier install for Claude Code, which the picker
	// starts with selected.
	existingClaude := installedSkills(".claude/skills")
	existingClaudeHome := home(existingClaude)
	pickerHome := home(withTrees(installedSkills(".agents/skills"), existingClaude))
	unreadableHome := home(existingClaude)

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
	reinstallTree := withTrees(installedSkills(".claude/skills"), map[string]string{
		".claude/skills/other/SKILL.md":   "user's skill",
		".claude/skills/.hidden/SKILL.md": "user's hidden entry",
		".claude/skills/linked":           "-> " + filepath.Join(reinstallHome, "marked"),
		"marked/.tiger-cli":               skillMarkerContent,
		"marked/SKILL.md":                 "marked skill",
	})

	// conflictTree holds things of the user's under the names of skills being
	// installed, in both of the locations being installed to, which aren't
	// replaced without --force. The symlink counts as the user's even though
	// it points at one of our skills, since Tiger CLI never creates symlinks.
	conflictHome := t.TempDir()
	conflictTree := map[string]string{
		".agents/skills/alpha":           "-> " + filepath.Join(conflictHome, "marked"),
		".agents/skills/beta/.tiger-cli": skillMarkerContent,
		".agents/skills/beta/SKILL.md":   "our beta",
		".claude/skills/beta/SKILL.md":   "user's beta",
		"marked/.tiger-cli":              skillMarkerContent,
		"marked/SKILL.md":                "marked skill",
	}
	writeTree(t, conflictHome, conflictTree)

	// forceHome holds a symlink and a directory of the user's under the names
	// of skills being installed. --force replaces both; the symlink is
	// replaced as a link, leaving its target alone.
	forceHome := home(map[string]string{
		".agents/skills/alpha":         "-> " + filepath.Join("..", "..", "elsewhere"),
		".agents/skills/beta/SKILL.md": "user's beta",
		".agents/skills/beta/stale.md": "stale",
		"elsewhere/SKILL.md":           "symlink target",
	})
	forceTree := withTrees(installedSkills(".agents/skills"), map[string]string{"elsewhere/SKILL.md": "symlink target"})

	universalDir := func(home string) string { return filepath.Join(home, ".agents", "skills") }
	claudeDir := func(home string) string { return filepath.Join(home, ".claude", "skills") }

	rateLimitReset := time.Date(2030, 1, 1, 15, 4, 0, 0, time.UTC).Unix()

	runCmdTests(t, []cmdTest{
		{
			name:    "--skills-dir with client arguments",
			args:    []string{"skills", "install", "claude-code", "--skills-dir", "/tmp/skills"},
			wantErr: "--skills-dir can't be combined with client arguments",
		},
		{
			name:    "unsupported client",
			args:    []string{"skills", "install", "claude-code", "bogus"},
			wantErr: "unsupported client: bogus. Supported clients: universal, cursor, devin, codex, gemini, gemini-cli, vscode, code, vs-code, copilot, copilot-cli, claude-code, antigravity, agy, kiro-cli",
		},
		{
			name:    "no arguments and no TTY",
			args:    []string{"skills", "install"},
			opts:    []runOption{withEnv("HOME", home(nil))},
			wantErr: "TTY not detected - specify install locations as arguments (e.g. 'tiger skills install universal')",
		},
		{
			name: "nothing selected in the picker",
			args: []string{"skills", "install"},
			opts: []runOption{
				withEnv("HOME", home(nil)),
				withIsTerminal(true),
				withRunSkillsPicker(pickerItems("Universal"), []bool{false, false, false, false}),
			},
			wantErr: "no install locations selected",
		},
		{
			name: "download fails",
			args: []string{"skills", "install", "universal"},
			opts: []runOption{
				withEnv("HOME", home(nil)),
				withSkillsServer(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}),
			},
			wantErr: "failed to download skills: unexpected status from GitHub: 500 Internal Server Error",
		},
		{
			name: "rate limited",
			args: []string{"skills", "install", "universal"},
			opts: []runOption{
				withEnv("HOME", home(nil)),
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
			args: []string{"skills", "install", "universal"},
			opts: []runOption{
				withEnv("HOME", home(nil)),
				withSkillsServer(func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte("not a tarball"))
				}),
			},
			wantErr: "failed to read skills archive: gzip: invalid header",
		},
		{
			name: "no skills in archive",
			args: []string{"skills", "install", "universal"},
			opts: []runOption{
				withEnv("HOME", home(nil)),
				withSkillsTarball(skillsTarball(t, tarEntry{name: "README.md", body: "repo readme"})),
			},
			wantErr: "no skills found",
		},
		{
			name: "existing skills not installed by Tiger CLI",
			args: []string{"skills", "install", "universal", "claude-code"},
			opts: []runOption{withEnv("HOME", conflictHome), withSkillsTarball(tarball)},
			wantErr: fmt.Sprintf("skills already exist and weren't installed by Tiger CLI: %s, %s. Use --force to replace them",
				filepath.Join(universalDir(conflictHome), "alpha"), filepath.Join(claudeDir(conflictHome), "beta")),
			checks: []checkFunc{checkTree(conflictHome, conflictTree)},
		},
		{
			name:       "installs to ~/.agents/skills",
			args:       []string{"skills", "install", "universal"},
			opts:       []runOption{withEnv("HOME", universalHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput([]string{universalDir(universalHome)}),
			checks:     []checkFunc{checkTree(universalHome, installedSkills(".agents/skills"))},
		},
		{
			// Arguments select exactly the locations named, so the universal
			// directory isn't written.
			name:       "installs to claude code's skills directory only",
			args:       []string{"skills", "install", "claude-code"},
			opts:       []runOption{withEnv("HOME", claudeHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput([]string{claudeDir(claudeHome)}),
			checks:     []checkFunc{checkTree(claudeHome, installedSkills(".claude/skills"))},
		},
		{
			name: "installs to several locations",
			args: []string{"skills", "install", "kiro-cli", "claude-code", "universal"},
			opts: []runOption{withEnv("HOME", multiHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput([]string{
				universalDir(multiHome),
				claudeDir(multiHome),
				filepath.Join(multiHome, ".kiro", "skills"),
			}),
			checks: []checkFunc{checkTree(multiHome, withTrees(
				installedSkills(".agents/skills"),
				installedSkills(".claude/skills"),
				installedSkills(".kiro/skills"),
			))},
		},
		{
			name: "respects CLAUDE_CONFIG_DIR",
			args: []string{"skills", "install", "claude-code"},
			opts: []runOption{
				withEnv("HOME", claudeConfigHome),
				withEnv("CLAUDE_CONFIG_DIR", claudeConfigDir),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput([]string{filepath.Join(claudeConfigDir, "skills")}),
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
			wantStdout: skillsInstallOutput([]string{claudeDir(claudeEmptyEnvHome)}),
			checks:     []checkFunc{checkTree(claudeEmptyEnvHome, installedSkills(".claude/skills"))},
		},
		{
			name: "relative CLAUDE_CONFIG_DIR is ignored",
			args: []string{"skills", "install", "claude-code"},
			opts: []runOption{
				withEnv("HOME", claudeRelativeEnvHome),
				withEnv("CLAUDE_CONFIG_DIR", "claude-config"),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput([]string{claudeDir(claudeRelativeEnvHome)}),
			checks:     []checkFunc{checkTree(claudeRelativeEnvHome, installedSkills(".claude/skills"))},
		},
		{
			name:       "installs to antigravity's skills directory",
			args:       []string{"skills", "install", "antigravity"},
			opts:       []runOption{withEnv("HOME", antigravityHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput([]string{filepath.Join(antigravityHome, ".gemini", "config", "skills")}),
			checks:     []checkFunc{checkTree(antigravityHome, installedSkills(".gemini/config/skills"))},
		},
		{
			// Client names that read the universal directory select it, once.
			name:       "add alias and case-insensitive client names",
			args:       []string{"skills", "add", "CURSOR", "codex"},
			opts:       []runOption{withEnv("HOME", aliasHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput([]string{universalDir(aliasHome)}),
			checks:     []checkFunc{checkTree(aliasHome, installedSkills(".agents/skills"))},
		},
		{
			// Without other installs, the picker starts with universal
			// selected.
			name: "installs to universal from the picker",
			args: []string{"skills", "install"},
			opts: []runOption{
				withEnv("HOME", universalPickerHome),
				withIsTerminal(true),
				withRunSkillsPicker(pickerItems("Universal"), []bool{true, false, false, false}),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput([]string{universalDir(universalPickerHome)}),
			checks:     []checkFunc{checkTree(universalPickerHome, installedSkills(".agents/skills"))},
		},
		{
			// An earlier install for Claude Code only is what the picker
			// starts with, so universal isn't selected.
			name: "picker starts from earlier installs",
			args: []string{"skills", "install"},
			opts: []runOption{
				withEnv("HOME", existingClaudeHome),
				withIsTerminal(true),
				withRunSkillsPicker(pickerItems("Claude Code"), []bool{false, true, false, false}),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput([]string{claudeDir(existingClaudeHome)}),
			checks:     []checkFunc{checkTree(existingClaudeHome, existingClaude)},
		},
		{
			// A location that can't be inspected starts unselected, even
			// though it holds an earlier install. The directory is made
			// unreadable by the setup hook (and restored, so t.TempDir can
			// clean up); root ignores file permissions, hence the skip.
			name: "picker leaves locations it can't inspect unselected",
			args: []string{"skills", "install"},
			opts: []runOption{
				withEnv("HOME", unreadableHome),
				withIsTerminal(true),
				withSetup(func(t *testing.T) {
					if os.Geteuid() == 0 {
						t.Skip("cannot test permission errors as root user")
					}
					dir := claudeDir(unreadableHome)
					if err := os.Chmod(dir, 0o000); err != nil {
						t.Fatalf("failed to chmod dir: %v", err)
					}
					t.Cleanup(func() { os.Chmod(dir, 0o755) })
				}),
				withRunSkillsPicker(pickerItems("Universal"), []bool{true, false, false, false}),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput([]string{universalDir(unreadableHome)}),
			checks:     []checkFunc{checkTree(filepath.Join(unreadableHome, ".agents"), installedSkills("skills"))},
		},
		{
			// The picker shows where each location actually is, including an
			// env var override.
			name: "picker shows CLAUDE_CONFIG_DIR",
			args: []string{"skills", "install"},
			opts: []runOption{
				withEnv("HOME", claudePickerEnvHome),
				withEnv("CLAUDE_CONFIG_DIR", filepath.Join(claudePickerEnvHome, "claude-config")),
				withIsTerminal(true),
				withRunSkillsPicker([]skillsPickerItem{
					{label: "Universal", dir: "~/.agents/skills", selected: true},
					{label: "Claude Code", dir: "~/claude-config/skills"},
					{label: "Google Antigravity", dir: "~/.gemini/config/skills"},
					{label: "Kiro CLI", dir: "~/.kiro/skills"},
				}, []bool{false, true, false, false}),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput([]string{filepath.Join(claudePickerEnvHome, "claude-config", "skills")}),
			checks:     []checkFunc{checkTree(claudePickerEnvHome, installedSkills("claude-config/skills"))},
		},
		{
			// The picker starts from the earlier installs; what's chosen is
			// installed, and an unchosen earlier install is left as it is.
			name: "installs to the locations chosen in the picker",
			args: []string{"skills", "install"},
			opts: []runOption{
				withEnv("HOME", pickerHome),
				withIsTerminal(true),
				withRunSkillsPicker(pickerItems("Universal", "Claude Code"), []bool{false, true, false, true}),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput([]string{claudeDir(pickerHome), filepath.Join(pickerHome, ".kiro", "skills")}),
			checks: []checkFunc{checkTree(pickerHome, withTrees(
				installedSkills(".agents/skills"),
				existingClaude,
				installedSkills(".kiro/skills"),
			))},
		},
		{
			name: "reinstall replaces our skills and removes ones no longer available",
			args: []string{"skills", "install", "claude-code"},
			opts: []runOption{withEnv("HOME", reinstallHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput([]string{claudeDir(reinstallHome)},
				fmt.Sprintf("Removed from %s (no longer available):\n  retired\n", claudeDir(reinstallHome))),
			checks: []checkFunc{
				checkTree(reinstallHome, reinstallTree),
				// Running again over a current install leaves it unchanged.
				func(t *testing.T, _ cmdResult) {
					again := runCommand(t, []string{"skills", "install", "claude-code"}, nil)
					if again.err != nil {
						t.Fatalf("second install failed: %v", again.err)
					}
					assertOutput(t, again.stdout, skillsInstallOutput([]string{claudeDir(reinstallHome)}))
				},
				checkTree(reinstallHome, reinstallTree),
			},
		},
		{
			name:       "--force replaces skills not installed by Tiger CLI",
			args:       []string{"skills", "install", "universal", "--force"},
			opts:       []runOption{withEnv("HOME", forceHome), withSkillsTarball(tarball)},
			wantStdout: skillsInstallOutput([]string{universalDir(forceHome)}),
			checks:     []checkFunc{checkTree(forceHome, forceTree)},
		},
		{
			name: "--skills-dir installs there only",
			args: []string{"skills", "install", "--skills-dir", "~/custom"},
			opts: []runOption{
				withEnv("HOME", customHome),
				withIsTerminal(true),
				withRunSkillsPicker(nil, nil),
				withSkillsTarball(tarball),
			},
			wantStdout: skillsInstallOutput([]string{filepath.Join(customHome, "custom")}),
			checks:     []checkFunc{checkTree(customHome, installedSkills("custom"))},
		},
	})
}

// TestSkillsPickerModel checks the picker's keys: toggling, moving, and that
// only enter confirms. Helper-level because the picker is a Bubble Tea model
// that needs a real TTY to run through the command; the command tests stub it
// via withRunSkillsPicker.
func TestSkillsPickerModel(t *testing.T) {
	// Ctrl+C is {Code: 'c', Mod: tea.ModCtrl}; the raw control byte {Code: 3}
	// stringifies to "\x03" and would match nothing.
	cases := []struct {
		name          string
		keys          []tea.KeyPressMsg
		wantSelected  []bool
		wantConfirmed bool
	}{
		{"enter keeps the preselection", []tea.KeyPressMsg{{Code: tea.KeyEnter}}, []bool{true, false, false}, true},
		{"space toggles the current row", []tea.KeyPressMsg{{Code: tea.KeySpace}, {Code: tea.KeyEnter}}, []bool{false, false, false}, true},
		{"down then space toggles the next row", []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeySpace}, {Code: tea.KeyEnter}}, []bool{true, true, false}, true},
		{"number keys toggle their row", []tea.KeyPressMsg{{Code: '3'}, {Code: '1'}, {Code: tea.KeyEnter}}, []bool{false, false, true}, true},
		{"number keys past the end do nothing", []tea.KeyPressMsg{{Code: '4'}, {Code: tea.KeyEnter}}, []bool{true, false, false}, true},
		{"cursor can't run past the end", []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeySpace}, {Code: tea.KeyEnter}}, []bool{true, false, true}, true},
		{"q cancels", []tea.KeyPressMsg{{Code: tea.KeySpace}, {Code: 'q'}}, []bool{false, false, false}, false},
		{"esc cancels", []tea.KeyPressMsg{{Code: tea.KeyEsc}}, []bool{true, false, false}, false},
		{"ctrl+c cancels", []tea.KeyPressMsg{{Code: 'c', Mod: tea.ModCtrl}}, []bool{true, false, false}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m tea.Model = skillsPickerModel{items: []skillsPickerItem{
				{label: "Universal", dir: "~/.agents/skills", selected: true},
				{label: "Claude Code", dir: "~/.claude/skills"},
				{label: "Kiro CLI", dir: "~/.kiro/skills"},
			}}
			for _, key := range tc.keys {
				m, _ = m.Update(key)
			}
			result := m.(skillsPickerModel)
			selected := make([]bool, len(result.items))
			for i, item := range result.items {
				selected[i] = item.selected
			}
			if diff := cmp.Diff(tc.wantSelected, selected); diff != "" {
				t.Errorf("selection mismatch (-want +got):\n%s", diff)
			}
			if result.confirmed != tc.wantConfirmed {
				t.Errorf("confirmed = %v, want %v", result.confirmed, tc.wantConfirmed)
			}
		})
	}
}
