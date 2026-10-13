package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timescale/tiger-cli/internal/util"
)

// stubTigerExecutablePath pins the executable path the install command writes
// into client configs, since os.Executable returns the unpredictable test
// binary path.
func stubTigerExecutablePath(t *testing.T, path string) {
	t.Helper()
	original := tigerExecutablePathFunc
	tigerExecutablePathFunc = func() (string, error) { return path, nil }
	t.Cleanup(func() { tigerExecutablePathFunc = original })
}

// withMCPHome isolates client config lookups to home: it sets HOME, clears
// the env vars that relocate client configs, and pins the platform to Linux
// so the expected paths don't depend on where the tests run.
func withMCPHome(home string) runOption {
	return withSetup(func(t *testing.T) {
		t.Setenv("HOME", home)
		for _, v := range []string{"APPDATA", "CLAUDE_CONFIG_DIR", "CODEX_HOME", "COPILOT_HOME", "GEMINI_CLI_HOME", "KIRO_HOME", "XDG_CONFIG_HOME"} {
			t.Setenv(v, "")
		}
		original := goos
		goos = "linux"
		t.Cleanup(func() { goos = original })
	})
}

// withGOOS makes the client config paths those of the given platform. It
// goes after withMCPHome, which pins Linux.
func withGOOS(platform string) runOption {
	return withSetup(func(t *testing.T) {
		original := goos
		goos = platform
		t.Cleanup(func() { goos = original })
	})
}

// installedFor is one line of the install command's list of clients.
type installedFor struct {
	client string
	path   string
}

// mcpInstalledList is the list of clients the install command prints.
func mcpInstalledList(installed ...installedFor) string {
	width := 0
	for _, in := range installed {
		width = max(width, len(in.client))
	}
	var b strings.Builder
	b.WriteString("Installed Tiger MCP server for:\n")
	for _, in := range installed {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, in.client, in.path)
	}
	return b.String()
}

// mcpInstallOutput is the exact stdout of a successful install.
func mcpInstallOutput(installed ...installedFor) string {
	return mcpInstalledList(installed...) + `
Restart your coding agents to load the new configuration.

Try asking your AI assistant:
  "List my Tiger Cloud services"
  "Create a new database service called my-app-db"
  "Show me Tiger Cloud documentation about hypertables"
  "What are the best practices for PostgreSQL indexing?"
  "Help me optimize my PostgreSQL queries"
  "Help me create a new database schema for my application"
  "Help me figure out which tables should be hypertables"
`
}

// mcpPickerItems returns the items the picker offers with the default
// (Linux) config paths, selecting the clients named in selected.
func mcpPickerItems(selected ...string) []pickerItem {
	items := []pickerItem{
		{label: "Claude Code", detail: "~/.claude.json"},
		{label: "Cursor", detail: "~/.cursor/mcp.json"},
		{label: "Devin", detail: "~/.config/devin/mcp_config.json"},
		{label: "Codex", detail: "~/.codex/config.toml"},
		{label: "Gemini CLI", detail: "~/.gemini/settings.json"},
		{label: "VS Code", detail: "~/.config/Code/User/mcp.json"},
		{label: "Google Antigravity", detail: "~/.gemini/config/mcp_config.json"},
		{label: "Kiro CLI", detail: "~/.kiro/settings/mcp.json"},
		{label: "GitHub Copilot CLI", detail: "~/.copilot/mcp-config.json"},
	}
	for i := range items {
		items[i].selected = slices.Contains(selected, items[i].label)
	}
	return items
}

// tigerEntryLines are the command and args of the Tiger entry an install
// writes to a JSON config (with the executable stubbed to "tiger"), at the
// indentation of a fresh file.
const tigerEntryLines = `      "command": "tiger",
      "args": [
        "mcp",
        "start"
      ]`

// freshJSONConfig is the JSON config an install creates where there was none,
// holding only the Tiger entry, whose lines are given.
func freshJSONConfig(serversKey, entryLines string) string {
	return fmt.Sprintf("{\n  %q: {\n    \"tiger\": {\n%s\n    }\n  }\n}\n", serversKey, entryLines)
}

// freshConfigs are the config files, relative to the home directory, that
// installing for each client creates on Linux where there were none.
var freshConfigs = map[MCPClient]struct {
	path    string
	content string
}{
	ClaudeCode:  {".claude.json", freshJSONConfig("mcpServers", "      \"type\": \"stdio\",\n"+tigerEntryLines)},
	Cursor:      {".cursor/mcp.json", freshJSONConfig("mcpServers", tigerEntryLines)},
	Devin:       {".config/devin/mcp_config.json", freshJSONConfig("mcpServers", "      \"transport\": \"stdio\",\n"+tigerEntryLines)},
	Codex:       {".codex/config.toml", "[mcp_servers.tiger]\ncommand = 'tiger'\nargs = ['mcp', 'start']\n"},
	Gemini:      {".gemini/settings.json", freshJSONConfig("mcpServers", tigerEntryLines)},
	VSCode:      {".config/Code/User/mcp.json", freshJSONConfig("servers", tigerEntryLines)},
	Antigravity: {".gemini/config/mcp_config.json", freshJSONConfig("mcpServers", tigerEntryLines)},
	KiroCLI:     {".kiro/settings/mcp.json", freshJSONConfig("mcpServers", tigerEntryLines)},
	Copilot:     {".copilot/mcp-config.json", freshJSONConfig("mcpServers", "      \"type\": \"local\",\n"+tigerEntryLines+",\n      \"tools\": [\n        \"*\"\n      ]")},
}

// freshTree returns the tree (see readTree) that installing for clients
// creates in an empty directory, with each file's path relative to it
// rewritten by rebase if set.
func freshTree(rebase func(string) string, clients ...MCPClient) map[string]string {
	tree := map[string]string{}
	for _, c := range clients {
		f := freshConfigs[c]
		p := f.path
		if rebase != nil {
			p = rebase(p)
		}
		tree[p] = f.content
	}
	return tree
}

// checkFile asserts the exact content of the file at path.
func checkFile(path, want string) checkFunc {
	return func(t *testing.T, _ cmdResult) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}
		if diff := cmp.Diff(want, string(data)); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", path, diff)
		}
	}
}

// checkMode asserts the permissions of the file at path.
func checkMode(path string, want os.FileMode) checkFunc {
	return func(t *testing.T, _ cmdResult) {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("failed to stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
}

// checkBackups asserts the contents of the backups made of the config file at
// path, and that each has the given permissions.
func checkBackups(path string, mode os.FileMode, want ...string) checkFunc {
	return func(t *testing.T, _ cmdResult) {
		t.Helper()
		matches, err := filepath.Glob(path + ".backup.*")
		if err != nil {
			t.Fatalf("failed to glob backups: %v", err)
		}
		var got []string
		for _, m := range matches {
			data, err := os.ReadFile(m)
			if err != nil {
				t.Fatalf("failed to read backup: %v", err)
			}
			got = append(got, string(data))
			checkMode(m, mode)(t, cmdResult{})
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("backups of %s mismatch (-want +got):\n%s", path, diff)
		}
	}
}

// withChmod sets the permissions of path for the case, restoring them (so
// t.TempDir can clean up) when it ends. Root ignores file permissions, so the
// case is skipped when run as root.
func withChmod(path string, mode os.FileMode) runOption {
	return withSetup(func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("cannot test permission errors as root user")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("failed to stat %s: %v", path, err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatalf("failed to chmod %s: %v", path, err)
		}
		t.Cleanup(func() { os.Chmod(path, info.Mode().Perm()) })
	})
}

func TestMCPInstallCmd(t *testing.T) {
	stubTigerExecutablePath(t, "tiger")

	// home creates a home directory seeded with tree (see writeTree). Its
	// path has symlinks resolved (on macOS, the temporary directory is under
	// one), like the paths of the config files an install writes through.
	home := func(tree map[string]string) string {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		writeTree(t, dir, tree)
		return dir
	}

	// pickerTree is a previous install for Claude Code (outdated) and Codex,
	// plus an entry in a config file that doesn't parse (so it doesn't count)
	// and a Cursor config holding only another server.
	pickerTree := map[string]string{
		".claude.json":          `{"mcpServers": {"tiger": {"command": "/old/tiger", "args": ["mcp", "start"]}}}`,
		".codex/config.toml":    "[mcp_servers.tiger]\ncommand = \"/old/tiger\"\nargs = [\"mcp\", \"start\"]\n",
		".gemini/settings.json": `{"mcpServers": {"tiger": {}`,
		".cursor/mcp.json":      `{"mcpServers": {"other": {"command": "other"}}}`,
	}
	pickerHome := home(pickerTree)

	unparseableTree := map[string]string{".codex/config.toml": "model = "}
	unparseableHome := home(unparseableTree)
	invalidJSONHome := home(map[string]string{".cursor/mcp.json": "{invalid json"})
	notObjectHome := home(map[string]string{".cursor/mcp.json": "[]"})
	serversNotObjectHome := home(map[string]string{".cursor/mcp.json": `{"mcpServers": []}`})
	unreadableHome := home(map[string]string{".cursor/mcp.json": "{}"})
	unwritableHome := home(map[string]string{".codex/config.toml": ""})
	freshHome := home(nil)

	// geminiConfig has comments and four-space indentation, which an install
	// keeps, adding the entry in the same style.
	geminiConfig := `{
    // Theme
    "theme": "dark",
    "mcpServers": {
        "other": {
            "command": "other"
        }
    }
}
`
	geminiHome := home(map[string]string{".gemini/settings.json": geminiConfig})

	tabsConfig := "{\n\t\"theme\": \"dark\"\n}\n"
	tabsHome := home(map[string]string{".cursor/mcp.json": tabsConfig})

	// claudeConfig has an outdated entry, written on one line by hand.
	claudeConfig := `{
  "numStartups": 3,
  "mcpServers": {
    "tiger": {"type": "stdio", "command": "/old/tiger", "args": ["mcp", "start"], "env": {}} // mine
  },
  "projects": {}
}
`
	claudeHome := home(map[string]string{".claude.json": claudeConfig})

	// upToDateTree holds the current entries, laid out differently from how
	// an install would write them.
	upToDateTree := map[string]string{
		".cursor/mcp.json":   `{"mcpServers":{"tiger":{"command":"tiger","args":["mcp","start"]}}}`,
		".codex/config.toml": "[mcp_servers]\ntiger = { command = \"tiger\", args = [\"mcp\", \"start\"] }\n",
	}
	upToDateHome := home(upToDateTree)

	codexConfig := `# Codex settings
model = "gpt-5"

[mcp_servers.other]
command = "other" # trailing

[mcp_servers.tiger]
command = "/old/tiger"
args = ["mcp", "start"]

[mcp_servers.tiger.env]
A = "b"

[profiles.fast]
model = "gpt-5-mini"
`
	codexHome := home(map[string]string{".codex/config.toml": codexConfig})

	envHome := home(nil)
	relativeEnvHome := home(nil)
	macHome := home(nil)
	windowsHome := home(nil)
	noAppDataHome := home(nil)
	symlinkHome := home(map[string]string{
		".cursor/mcp.json":     "-> ../dotfiles/cursor.json",
		"dotfiles/cursor.json": "{}",
	})
	backupHome := home(map[string]string{".cursor/mcp.json": "{}"})
	noBackupHome := home(map[string]string{".cursor/mcp.json": "{}"})
	customHome := home(nil)
	aliasHome := home(nil)

	runCmdTests(t, []cmdTest{
		{
			name:    "--config-path without a client",
			args:    []string{"mcp", "install", "--config-path", "/tmp/mcp.json"},
			wantErr: "--config-path requires exactly one client argument",
		},
		{
			name:    "--config-path with several clients",
			args:    []string{"mcp", "install", "cursor", "codex", "--config-path", "/tmp/mcp.json"},
			wantErr: "--config-path requires exactly one client argument",
		},
		{
			name:    "unsupported client",
			args:    []string{"mcp", "install", "cursor", "bogus"},
			wantErr: "unsupported client: bogus. Supported clients: claude-code, cursor, devin, codex, gemini, gemini-cli, vscode, code, vs-code, antigravity, agy, kiro-cli, copilot, copilot-cli",
		},
		{
			name:    "no arguments and no TTY",
			args:    []string{"mcp", "install"},
			opts:    []runOption{withMCPHome(home(nil))},
			wantErr: "TTY not detected - specify clients as arguments (e.g. 'tiger mcp install claude-code')",
		},
		{
			name: "nothing selected in the picker",
			args: []string{"mcp", "install"},
			opts: []runOption{
				withMCPHome(home(nil)),
				withIsTerminal(true),
				withRunPicker(mcpPickerItems(), make([]bool, 9)),
			},
			wantErr: "no clients selected",
		},
		{
			// The picker starts from the clients the server is installed for.
			// What's chosen is installed, and an unchosen earlier install is
			// left as it is.
			name: "installs for the clients chosen in the picker",
			args: []string{"mcp", "install"},
			opts: []runOption{
				withMCPHome(pickerHome),
				withIsTerminal(true),
				withRunPicker(mcpPickerItems("Claude Code", "Codex"), []bool{true, true, false, false, false, false, false, false, false}),
			},
			wantStdout: mcpInstallOutput(
				installedFor{"Claude Code", filepath.Join(pickerHome, ".claude.json")},
				installedFor{"Cursor", filepath.Join(pickerHome, ".cursor", "mcp.json")},
			),
			checks: []checkFunc{
				checkFile(filepath.Join(pickerHome, ".claude.json"), `{"mcpServers": {"tiger": {
      "type": "stdio",
      "command": "tiger",
      "args": [
        "mcp",
        "start"
      ]
    }}}`),
				checkFile(filepath.Join(pickerHome, ".cursor", "mcp.json"), `{"mcpServers": {"other": {"command": "other"},
    "tiger": {
      "command": "tiger",
      "args": [
        "mcp",
        "start"
      ]
    }}}`),
				checkFile(filepath.Join(pickerHome, ".codex", "config.toml"), pickerTree[".codex/config.toml"]),
				checkFile(filepath.Join(pickerHome, ".gemini", "settings.json"), pickerTree[".gemini/settings.json"]),
			},
		},
		{
			// Every config file is read before any is written, so Claude
			// Code's isn't created.
			name:    "unparseable config stops every install",
			args:    []string{"mcp", "install", "claude-code", "codex"},
			opts:    []runOption{withMCPHome(unparseableHome)},
			wantErr: fmt.Sprintf("failed to install for Codex: failed to parse %s: toml: expected value, not end of input", filepath.Join(unparseableHome, ".codex", "config.toml")),
			checks:  []checkFunc{checkTree(unparseableHome, unparseableTree)},
		},
		{
			name:    "invalid JSON config",
			args:    []string{"mcp", "install", "cursor"},
			opts:    []runOption{withMCPHome(invalidJSONHome)},
			wantErr: fmt.Sprintf("failed to install for Cursor: failed to parse %s: hujson: line 1, column 2: invalid literal: invalid", filepath.Join(invalidJSONHome, ".cursor", "mcp.json")),
		},
		{
			name:    "config that isn't a JSON object",
			args:    []string{"mcp", "install", "cursor"},
			opts:    []runOption{withMCPHome(notObjectHome)},
			wantErr: fmt.Sprintf("failed to install for Cursor: failed to parse %s: not a JSON object", filepath.Join(notObjectHome, ".cursor", "mcp.json")),
		},
		{
			name:    "servers that aren't a JSON object",
			args:    []string{"mcp", "install", "cursor"},
			opts:    []runOption{withMCPHome(serversNotObjectHome)},
			wantErr: fmt.Sprintf("failed to install for Cursor: failed to parse %s: mcpServers is not an object", filepath.Join(serversNotObjectHome, ".cursor", "mcp.json")),
		},
		{
			name: "unreadable config",
			args: []string{"mcp", "install", "cursor"},
			opts: []runOption{
				withMCPHome(unreadableHome),
				withChmod(filepath.Join(unreadableHome, ".cursor", "mcp.json"), 0o000),
			},
			wantErr: fmt.Sprintf("failed to install for Cursor: failed to read configuration file: open %s: permission denied", filepath.Join(unreadableHome, ".cursor", "mcp.json")),
		},
		{
			// A failure partway through reports the clients already installed.
			// The temporary file's name is random, hence the regexp.
			name: "write failure",
			args: []string{"mcp", "install", "claude-code", "codex", "--no-backup"},
			opts: []runOption{
				withMCPHome(unwritableHome),
				withChmod(filepath.Join(unwritableHome, ".codex"), 0o555),
			},
			wantStdout: mcpInstalledList(installedFor{"Claude Code", filepath.Join(unwritableHome, ".claude.json")}),
			wantErr: matchRegexp(`^failed to install for Codex: failed to write configuration file: open ` +
				regexp.QuoteMeta(filepath.Join(unwritableHome, ".codex", ".config.toml.tiger-")) + `\d+: permission denied$`),
			checks: []checkFunc{checkFile(filepath.Join(unwritableHome, ".claude.json"), freshConfigs[ClaudeCode].content)},
		},
		{
			name: "installs for every client",
			args: []string{"mcp", "install", "claude-code", "cursor", "devin", "codex", "gemini", "vscode", "antigravity", "kiro-cli", "copilot"},
			opts: []runOption{withMCPHome(freshHome)},
			wantStdout: mcpInstallOutput(
				installedFor{"Claude Code", filepath.Join(freshHome, ".claude.json")},
				installedFor{"Cursor", filepath.Join(freshHome, ".cursor", "mcp.json")},
				installedFor{"Devin", filepath.Join(freshHome, ".config", "devin", "mcp_config.json")},
				installedFor{"Codex", filepath.Join(freshHome, ".codex", "config.toml")},
				installedFor{"Gemini CLI", filepath.Join(freshHome, ".gemini", "settings.json")},
				installedFor{"VS Code", filepath.Join(freshHome, ".config", "Code", "User", "mcp.json")},
				installedFor{"Google Antigravity", filepath.Join(freshHome, ".gemini", "config", "mcp_config.json")},
				installedFor{"Kiro CLI", filepath.Join(freshHome, ".kiro", "settings", "mcp.json")},
				installedFor{"GitHub Copilot CLI", filepath.Join(freshHome, ".copilot", "mcp-config.json")},
			),
			checks: []checkFunc{
				checkTree(freshHome, freshTree(nil, ClaudeCode, Cursor, Devin, Codex, Gemini, VSCode, Antigravity, KiroCLI, Copilot)),
				checkMode(filepath.Join(freshHome, ".claude.json"), 0o600),
			},
		},
		{
			name:       "keeps the rest of a JSON config as it is",
			args:       []string{"mcp", "install", "gemini"},
			opts:       []runOption{withMCPHome(geminiHome)},
			wantStdout: mcpInstallOutput(installedFor{"Gemini CLI", filepath.Join(geminiHome, ".gemini", "settings.json")}),
			checks: []checkFunc{
				checkFile(filepath.Join(geminiHome, ".gemini", "settings.json"), `{
    // Theme
    "theme": "dark",
    "mcpServers": {
        "other": {
            "command": "other"
        },
        "tiger": {
            "command": "tiger",
            "args": [
                "mcp",
                "start"
            ]
        }
    }
}
`),
				checkBackups(filepath.Join(geminiHome, ".gemini", "settings.json"), 0o644, geminiConfig),
			},
		},
		{
			name:       "adds the servers to a JSON config without any",
			args:       []string{"mcp", "install", "cursor", "--no-backup"},
			opts:       []runOption{withMCPHome(tabsHome)},
			wantStdout: mcpInstallOutput(installedFor{"Cursor", filepath.Join(tabsHome, ".cursor", "mcp.json")}),
			checks: []checkFunc{checkFile(filepath.Join(tabsHome, ".cursor", "mcp.json"), `{
	"theme": "dark",
	"mcpServers": {
		"tiger": {
			"command": "tiger",
			"args": [
				"mcp",
				"start"
			]
		}
	}
}
`)},
		},
		{
			// The whole entry is replaced, dropping fields the install doesn't
			// write, but a comment beside it stays.
			name:       "replaces an outdated JSON entry",
			args:       []string{"mcp", "install", "claude-code", "--no-backup"},
			opts:       []runOption{withMCPHome(claudeHome)},
			wantStdout: mcpInstallOutput(installedFor{"Claude Code", filepath.Join(claudeHome, ".claude.json")}),
			checks: []checkFunc{checkFile(filepath.Join(claudeHome, ".claude.json"), `{
  "numStartups": 3,
  "mcpServers": {
    "tiger": {
      "type": "stdio",
      "command": "tiger",
      "args": [
        "mcp",
        "start"
      ]
    } // mine
  },
  "projects": {}
}
`)},
		},
		{
			name:       "replaces an outdated TOML entry",
			args:       []string{"mcp", "install", "codex", "--no-backup"},
			opts:       []runOption{withMCPHome(codexHome)},
			wantStdout: mcpInstallOutput(installedFor{"Codex", filepath.Join(codexHome, ".codex", "config.toml")}),
			checks: []checkFunc{checkFile(filepath.Join(codexHome, ".codex", "config.toml"), `# Codex settings
model = "gpt-5"

[mcp_servers.other]
command = "other" # trailing

[mcp_servers.tiger]
command = 'tiger'
args = ['mcp', 'start']

[profiles.fast]
model = "gpt-5-mini"
`)},
		},
		{
			// Config files already holding the current entries aren't
			// rewritten or backed up, however they're laid out.
			name: "leaves up-to-date configs alone",
			args: []string{"mcp", "install", "cursor", "codex"},
			opts: []runOption{withMCPHome(upToDateHome)},
			wantStdout: mcpInstallOutput(
				installedFor{"Cursor", filepath.Join(upToDateHome, ".cursor", "mcp.json")},
				installedFor{"Codex", filepath.Join(upToDateHome, ".codex", "config.toml")},
			),
			checks: []checkFunc{checkTree(upToDateHome, upToDateTree)},
		},
		{
			name: "respects env vars that relocate configs",
			args: []string{"mcp", "install", "claude-code", "devin", "codex", "gemini", "vscode", "kiro-cli", "copilot"},
			opts: []runOption{
				withMCPHome(envHome),
				withEnv("CLAUDE_CONFIG_DIR", filepath.Join(envHome, "claude")),
				withEnv("CODEX_HOME", filepath.Join(envHome, "codex")),
				withEnv("COPILOT_HOME", filepath.Join(envHome, "copilot")),
				withEnv("GEMINI_CLI_HOME", filepath.Join(envHome, "gemini")),
				withEnv("KIRO_HOME", filepath.Join(envHome, "kiro")),
				withEnv("XDG_CONFIG_HOME", filepath.Join(envHome, "xdg")),
			},
			wantStdout: mcpInstallOutput(
				installedFor{"Claude Code", filepath.Join(envHome, "claude", ".claude.json")},
				installedFor{"Devin", filepath.Join(envHome, "xdg", "devin", "mcp_config.json")},
				installedFor{"Codex", filepath.Join(envHome, "codex", "config.toml")},
				installedFor{"Gemini CLI", filepath.Join(envHome, "gemini", ".gemini", "settings.json")},
				installedFor{"VS Code", filepath.Join(envHome, "xdg", "Code", "User", "mcp.json")},
				installedFor{"Kiro CLI", filepath.Join(envHome, "kiro", "settings", "mcp.json")},
				installedFor{"GitHub Copilot CLI", filepath.Join(envHome, "copilot", "mcp-config.json")},
			),
			checks: []checkFunc{checkTree(envHome, freshTree(func(p string) string {
				for from, to := range map[string]string{".config/": "xdg/", ".codex/": "codex/", ".copilot/": "copilot/", ".kiro/": "kiro/", ".gemini/": "gemini/.gemini/"} {
					if rest, ok := strings.CutPrefix(p, from); ok {
						return to + rest
					}
				}
				return "claude/" + p
			}, ClaudeCode, Devin, Codex, Gemini, VSCode, KiroCLI, Copilot))},
		},
		{
			name: "relative env var is ignored",
			args: []string{"mcp", "install", "codex"},
			opts: []runOption{
				withMCPHome(relativeEnvHome),
				withEnv("CODEX_HOME", "codex"),
			},
			wantStdout: mcpInstallOutput(installedFor{"Codex", filepath.Join(relativeEnvHome, ".codex", "config.toml")}),
			checks:     []checkFunc{checkTree(relativeEnvHome, freshTree(nil, Codex))},
		},
		{
			name: "macOS config paths",
			args: []string{"mcp", "install", "devin", "vscode"},
			opts: []runOption{withMCPHome(macHome), withGOOS("darwin")},
			wantStdout: mcpInstallOutput(
				installedFor{"Devin", filepath.Join(macHome, ".config", "devin", "mcp_config.json")},
				installedFor{"VS Code", filepath.Join(macHome, "Library", "Application Support", "Code", "User", "mcp.json")},
			),
			checks: []checkFunc{checkTree(macHome, map[string]string{
				".config/devin/mcp_config.json":                  freshConfigs[Devin].content,
				"Library/Application Support/Code/User/mcp.json": freshConfigs[VSCode].content,
			})},
		},
		{
			name: "Windows config paths",
			args: []string{"mcp", "install", "claude-code", "devin", "vscode"},
			opts: []runOption{
				withMCPHome(windowsHome),
				withGOOS("windows"),
				withEnv("APPDATA", filepath.Join(windowsHome, "AppData", "Roaming")),
			},
			wantStdout: mcpInstallOutput(
				installedFor{"Claude Code", filepath.Join(windowsHome, ".claude.json")},
				installedFor{"Devin", filepath.Join(windowsHome, "AppData", "Roaming", "devin", "mcp_config.json")},
				installedFor{"VS Code", filepath.Join(windowsHome, "AppData", "Roaming", "Code", "User", "mcp.json")},
			),
			checks: []checkFunc{checkTree(windowsHome, map[string]string{
				".claude.json":                          freshConfigs[ClaudeCode].content,
				"AppData/Roaming/devin/mcp_config.json": freshConfigs[Devin].content,
				"AppData/Roaming/Code/User/mcp.json":    freshConfigs[VSCode].content,
			})},
		},
		{
			name:    "Windows without APPDATA",
			args:    []string{"mcp", "install", "devin"},
			opts:    []runOption{withMCPHome(noAppDataHome), withGOOS("windows")},
			wantErr: "failed to install for Devin: failed to determine configuration file: ${APPDATA}/devin/mcp_config.json references an unset environment variable",
		},
		{
			name:       "writes through a symlinked config",
			args:       []string{"mcp", "install", "cursor", "--no-backup"},
			opts:       []runOption{withMCPHome(symlinkHome)},
			wantStdout: mcpInstallOutput(installedFor{"Cursor", filepath.Join(symlinkHome, ".cursor", "mcp.json")}),
			checks: []checkFunc{checkTree(symlinkHome, map[string]string{
				".cursor/mcp.json":     "-> ../dotfiles/cursor.json",
				"dotfiles/cursor.json": "{\n  \"mcpServers\": {\n    \"tiger\": {\n" + tigerEntryLines + "\n    }\n  }\n}",
			})},
		},
		{
			name: "backs up the config, keeping its permissions",
			args: []string{"mcp", "install", "cursor"},
			opts: []runOption{
				withMCPHome(backupHome),
				withChmod(filepath.Join(backupHome, ".cursor", "mcp.json"), 0o640),
			},
			wantStdout: mcpInstallOutput(installedFor{"Cursor", filepath.Join(backupHome, ".cursor", "mcp.json")}),
			checks: []checkFunc{
				checkMode(filepath.Join(backupHome, ".cursor", "mcp.json"), 0o640),
				checkBackups(filepath.Join(backupHome, ".cursor", "mcp.json"), 0o640, "{}"),
			},
		},
		{
			name:       "--no-backup",
			args:       []string{"mcp", "install", "cursor", "--no-backup"},
			opts:       []runOption{withMCPHome(noBackupHome)},
			wantStdout: mcpInstallOutput(installedFor{"Cursor", filepath.Join(noBackupHome, ".cursor", "mcp.json")}),
			checks:     []checkFunc{checkBackups(filepath.Join(noBackupHome, ".cursor", "mcp.json"), 0o644)},
		},
		{
			// The file keeps the client's format whatever it's named.
			name:       "--config-path",
			args:       []string{"mcp", "install", "codex", "--config-path", "~/custom/codex.json"},
			opts:       []runOption{withMCPHome(customHome)},
			wantStdout: mcpInstallOutput(installedFor{"Codex", filepath.Join(customHome, "custom", "codex.json")}),
			checks:     []checkFunc{checkTree(customHome, map[string]string{"custom/codex.json": freshConfigs[Codex].content})},
		},
		{
			// Clients are installed once each, in the supported clients
			// order.
			name: "add alias and case-insensitive, repeated client names",
			args: []string{"mcp", "add", "CURSOR", "claude-code", "cursor"},
			opts: []runOption{withMCPHome(aliasHome)},
			wantStdout: mcpInstallOutput(
				installedFor{"Claude Code", filepath.Join(aliasHome, ".claude.json")},
				installedFor{"Cursor", filepath.Join(aliasHome, ".cursor", "mcp.json")},
			),
			checks: []checkFunc{checkTree(aliasHome, freshTree(nil, ClaudeCode, Cursor))},
		},
	})
}

// TestInstallMCPForClient covers what pkg/mcpinstall adds over the command,
// which always passes every option: validating them, and installing under a
// name and command of the caller's.
func TestInstallMCPForClient(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "mcp.json")
	valid := InstallOptions{
		ClientName:       "cursor",
		ServerName:       "other",
		Command:          "/usr/bin/other",
		Args:             []string{"serve"},
		CustomConfigPath: configPath,
	}
	cases := []struct {
		name    string
		modify  func(*InstallOptions)
		wantErr string
	}{
		{"missing client", func(o *InstallOptions) { o.ClientName = "" }, "missing required option: ClientName"},
		{"missing server name", func(o *InstallOptions) { o.ServerName = "" }, "missing required option: ServerName"},
		{"missing command", func(o *InstallOptions) { o.Command = "" }, "missing required option: Command"},
		{"missing args", func(o *InstallOptions) { o.Args = nil }, "missing required option: Args"},
		{"unsupported client", func(o *InstallOptions) { o.ClientName = "bogus" }, "unsupported client: bogus. Supported clients: claude-code, cursor, devin, codex, gemini, gemini-cli, vscode, code, vs-code, antigravity, agy, kiro-cli, copilot, copilot-cli"},
		{"installs", func(*InstallOptions) {}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := valid
			tc.modify(&opts)
			err := InstallMCPForClient(opts)
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			assertOutput(t, gotErr, tc.wantErr)
		})
	}
	checkFile(configPath, `{
  "mcpServers": {
    "other": {
      "command": "/usr/bin/other",
      "args": [
        "serve"
      ]
    }
  }
}
`)(t, cmdResult{})
}

// TestExpandPath covers the path expansion the install command relies on for
// --config-path and the built-in client config locations.
func TestExpandPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	t.Setenv("TEST_EXPAND_PATH_VAR", "/test/env/path")
	t.Setenv("TEST_EXPAND_PATH_SUBDIR", "Documents")

	tests := []struct {
		name string
		path string
		want string
	}{
		{"tilde", "~/config.json", filepath.Join(homeDir, "config.json")},
		{"tilde with subdirectory", "~/.config/tiger/config.json", filepath.Join(homeDir, ".config/tiger/config.json")},
		{"absolute path unchanged", "/absolute/path/config.json", "/absolute/path/config.json"},
		{"relative path unchanged", "relative/path/config.json", "relative/path/config.json"},
		{"env var", "$TEST_EXPAND_PATH_VAR/config.json", "/test/env/path/config.json"},
		{"env var with braces", "${TEST_EXPAND_PATH_VAR}/config.json", "/test/env/path/config.json"},
		{"tilde and env var", "~/$TEST_EXPAND_PATH_SUBDIR/config.json", filepath.Join(homeDir, "Documents", "config.json")},
		{"undefined env var becomes empty", "$UNDEFINED_ENV_VAR/config.json", "/config.json"},
		{"tilde not at beginning unchanged", "/some/path/~/config.json", "/some/path/~/config.json"},
		{"bare tilde", "~", homeDir},
		{"tilde with trailing slash", "~/", homeDir},
		{"empty path", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertOutput(t, util.ExpandPath(tt.path), tt.want)
		})
	}
}
