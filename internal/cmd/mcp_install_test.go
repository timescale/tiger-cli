package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// installSuccessOutput is the exact stdout `tiger mcp install` prints after a
// successful file-based installation.
func installSuccessOutput(clientName, configPath string) string {
	return fmt.Sprintf(`✅ Successfully installed Tiger MCP server configuration for %s
📁 Configuration file: %s

💡 Next steps:
   1. Restart %s to load the new configuration
   2. The Tiger MCP server will be available as 'tiger'

🤖 Try asking your AI assistant:

   📊 List and manage your Tiger Cloud services:
   • "List my Tiger Cloud services"
   • "Show me details for service xyz-123"
   • "Create a new database service called my-app-db"
   • "Update the password for my database service"
   • "What Tiger Cloud services do I have access to?"

   📚 Ask questions from the PostgreSQL and Tiger Cloud documentation:
   • "Show me Tiger Cloud documentation about hypertables?"
   • "What are the best practices for PostgreSQL indexing?"
   • "What is the command for renaming a table?"
   • "Help me optimize my PostgreSQL queries"

   📋 Make use of our optimized AI guides for common workflows:
   • "Help me create a new database schema for my application"
   • "Help me set up hypertables for the device_readings table"
   • "Help me figure out which tables should be hypertables"
   • "What's the best way to structure time-series data?"
`, clientName, configPath, clientName)
}

// tigerServerEntry is the MCP server entry the install command writes (with
// the executable path stubbed to "tiger").
func tigerServerEntry() map[string]any {
	return map[string]any{
		"command": "tiger",
		"args":    []any{"mcp", "start"},
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("file %s is not valid JSON: %v", path, err)
	}
	return parsed
}

func assertJSONFile(t *testing.T, path string, want map[string]any) {
	t.Helper()
	if diff := cmp.Diff(want, readJSONFile(t, path)); diff != "" {
		t.Errorf("config file mismatch (-want +got):\n%s", diff)
	}
}

// backupFiles returns the backup files created alongside configPath.
func backupFiles(t *testing.T, configPath string) []string {
	t.Helper()
	matches, err := filepath.Glob(configPath + ".backup.*")
	if err != nil {
		t.Fatalf("failed to glob backup files: %v", err)
	}
	return matches
}

func TestMCPInstallCmd(t *testing.T) {
	stubTigerExecutablePath(t, "tiger")

	dir := t.TempDir()
	home := t.TempDir()

	// path returns a per-case config file path; seed pre-creates it.
	path := func(caseName string) string {
		return filepath.Join(dir, caseName, "mcp.json")
	}
	seed := func(caseName, content string, mode os.FileMode) string {
		t.Helper()
		p := path(caseName)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("failed to create config dir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatalf("failed to seed config file: %v", err)
		}
		return p
	}

	// Stub `claude` on PATH so the CLI-based install path runs end-to-end
	// without the real client; the script records its argv for the check.
	stubBin := t.TempDir()
	argvFile := filepath.Join(stubBin, "argv")
	stubScript := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\n", argvFile)
	if err := os.WriteFile(filepath.Join(stubBin, "claude"), []byte(stubScript), 0755); err != nil {
		t.Fatalf("failed to write claude stub: %v", err)
	}
	cliHome := t.TempDir()

	mergePath := seed("merge", `{"mcpServers": {"server1": {"command": "cmd1", "args": ["arg1"]}, "server2": {"command": "cmd2", "args": ["arg2", "arg3"]}}}`, 0644)
	otherPath := seed("other", `{"other": "config"}`, 0644)
	idempotentPath := seed("idempotent", `{"mcpServers": {"existing": {"command": "existing", "args": ["arg1"]}, "tiger": {"command": "/old/path/to/tiger", "args": ["old", "args"]}}}`, 0644)
	emptyPath := seed("empty", "", 0644)
	backupInitial := `{"mcpServers": {"existing": {"command": "test", "args": ["arg1"]}}}`
	backupPath := seed("backup", backupInitial, 0644)
	permsPath := seed("perms", `{"test": "data"}`, 0600)
	badPath := seed("bad", `{invalid json`, 0644)

	// For "backup fails when config is unreadable": the directory is made
	// unreadable by that case's setup hook, so the paths are just declared here.
	lockedDir := filepath.Join(dir, "locked")
	lockedConfigPath := filepath.Join(lockedDir, "config.json")

	runCmdTests(t, []cmdTest{
		{
			name:    "too many arguments",
			args:    []string{"mcp", "install", "cursor", "windsurf"},
			wantErr: "accepts at most 1 arg(s), received 2",
		},
		{
			name:    "no client and no TTY",
			args:    []string{"mcp", "install"},
			wantErr: "TTY not detected - specify a client as an argument (e.g. 'tiger mcp install claude-code')",
		},
		{
			name:    "unsupported client",
			args:    []string{"mcp", "install", "bogus"},
			wantErr: "unsupported client: bogus. Supported clients: claude-code, cursor, windsurf, codex, gemini, gemini-cli, vscode, code, vs-code, antigravity, agy, kiro-cli, copilot, copilot-cli",
		},
		{
			name:    "invalid existing config",
			args:    []string{"mcp", "install", "cursor", "--no-backup", "--config-path", badPath},
			wantErr: "failed to add MCP server configuration: failed to parse existing config: hujson: line 1, column 2: invalid literal: invalid",
		},
		{
			// Backups are enabled (the default) but nothing exists to back up.
			name:       "creates config file and directories",
			args:       []string{"mcp", "install", "cursor", "--config-path", path("fresh")},
			wantStdout: installSuccessOutput("cursor", path("fresh")),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				assertJSONFile(t, path("fresh"), map[string]any{
					"mcpServers": map[string]any{"tiger": tigerServerEntry()},
				})
				info, err := os.Stat(path("fresh"))
				if err != nil {
					t.Fatalf("failed to stat config file: %v", err)
				}
				if got := info.Mode().Perm(); got != 0600 {
					t.Errorf("new config file mode = %o, want 0600", got)
				}
				if backups := backupFiles(t, path("fresh")); len(backups) != 0 {
					t.Errorf("expected no backup files, got %v", backups)
				}
			}},
		},
		{
			name:       "merges with existing servers",
			args:       []string{"mcp", "install", "cursor", "--no-backup", "--config-path", mergePath},
			wantStdout: installSuccessOutput("cursor", mergePath),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				assertJSONFile(t, mergePath, map[string]any{
					"mcpServers": map[string]any{
						"server1": map[string]any{"command": "cmd1", "args": []any{"arg1"}},
						"server2": map[string]any{"command": "cmd2", "args": []any{"arg2", "arg3"}},
						"tiger":   tigerServerEntry(),
					},
				})
				// --no-backup skips the backup of the existing file.
				if backups := backupFiles(t, mergePath); len(backups) != 0 {
					t.Errorf("expected no backup files with --no-backup, got %v", backups)
				}
			}},
		},
		{
			name:       "preserves unrelated config keys",
			args:       []string{"mcp", "install", "cursor", "--no-backup", "--config-path", otherPath},
			wantStdout: installSuccessOutput("cursor", otherPath),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				assertJSONFile(t, otherPath, map[string]any{
					"other":      "config",
					"mcpServers": map[string]any{"tiger": tigerServerEntry()},
				})
			}},
		},
		{
			name:       "updates existing tiger entry idempotently",
			args:       []string{"mcp", "install", "cursor", "--no-backup", "--config-path", idempotentPath},
			wantStdout: installSuccessOutput("cursor", idempotentPath),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				want := map[string]any{
					"mcpServers": map[string]any{
						"existing": map[string]any{"command": "existing", "args": []any{"arg1"}},
						"tiger":    tigerServerEntry(),
					},
				}
				assertJSONFile(t, idempotentPath, want)

				// A second install must leave the config unchanged.
				again := runCommand(t, []string{"mcp", "install", "cursor", "--no-backup", "--config-path", idempotentPath}, nil)
				if again.err != nil {
					t.Fatalf("second install failed: %v", again.err)
				}
				assertJSONFile(t, idempotentPath, want)
			}},
		},
		{
			name:       "handles empty config file and backs it up",
			args:       []string{"mcp", "install", "cursor", "--config-path", emptyPath},
			wantStdout: installSuccessOutput("cursor", emptyPath),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				assertJSONFile(t, emptyPath, map[string]any{
					"mcpServers": map[string]any{"tiger": tigerServerEntry()},
				})
				backups := backupFiles(t, emptyPath)
				if len(backups) != 1 {
					t.Fatalf("expected 1 backup file, got %v", backups)
				}
				content, err := os.ReadFile(backups[0])
				if err != nil {
					t.Fatalf("failed to read backup: %v", err)
				}
				if len(content) != 0 {
					t.Errorf("backup of empty file should be empty, got %q", content)
				}
			}},
		},
		{
			name:       "creates backup by default",
			args:       []string{"mcp", "install", "cursor", "--config-path", backupPath},
			wantStdout: installSuccessOutput("cursor", backupPath),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				backups := backupFiles(t, backupPath)
				if len(backups) != 1 {
					t.Fatalf("expected 1 backup file, got %v", backups)
				}
				content, err := os.ReadFile(backups[0])
				if err != nil {
					t.Fatalf("failed to read backup: %v", err)
				}
				assertOutput(t, string(content), backupInitial)
				assertJSONFile(t, backupPath, map[string]any{
					"mcpServers": map[string]any{
						"existing": map[string]any{"command": "test", "args": []any{"arg1"}},
						"tiger":    tigerServerEntry(),
					},
				})

				// A second install creates a second, distinct backup.
				again := runCommand(t, []string{"mcp", "install", "cursor", "--config-path", backupPath}, nil)
				if again.err != nil {
					t.Fatalf("second install failed: %v", again.err)
				}
				if backups := backupFiles(t, backupPath); len(backups) != 2 {
					t.Errorf("expected 2 distinct backup files, got %v", backups)
				}
			}},
		},
		{
			name:       "backup preserves file permissions",
			args:       []string{"mcp", "install", "cursor", "--config-path", permsPath},
			wantStdout: installSuccessOutput("cursor", permsPath),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				backups := backupFiles(t, permsPath)
				if len(backups) != 1 {
					t.Fatalf("expected 1 backup file, got %v", backups)
				}
				for _, p := range append(backups, permsPath) {
					info, err := os.Stat(p)
					if err != nil {
						t.Fatalf("failed to stat %s: %v", p, err)
					}
					if got := info.Mode().Perm(); got != 0600 {
						t.Errorf("%s mode = %o, want 0600", p, got)
					}
				}
			}},
		},
		{
			// claude-code installs via the client's own CLI rather than JSON
			// patching; the stub on PATH records the exact command run.
			name: "cli-based client runs the client's install command",
			args: []string{"mcp", "install", "claude-code"},
			opts: []runOption{
				withEnv("PATH", stubBin),
				withEnv("HOME", cliHome),
			},
			wantStdout: installSuccessOutput("claude-code", filepath.Join(cliHome, ".claude.json")),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				argv, err := os.ReadFile(argvFile)
				if err != nil {
					t.Fatalf("claude stub was not invoked: %v", err)
				}
				assertOutput(t, string(argv), "mcp\nadd\n-s\nuser\ntiger\ntiger\nmcp\nstart\n")
			}},
		},
		{
			name:       "default config path under HOME",
			args:       []string{"mcp", "install", "cursor", "--no-backup"},
			opts:       []runOption{withEnv("HOME", home)},
			wantStdout: installSuccessOutput("cursor", filepath.Join(home, ".cursor", "mcp.json")),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				assertJSONFile(t, filepath.Join(home, ".cursor", "mcp.json"), map[string]any{
					"mcpServers": map[string]any{"tiger": tigerServerEntry()},
				})
			}},
		},
		{
			name:       "client name is case-insensitive",
			args:       []string{"mcp", "install", "CURSOR", "--no-backup", "--config-path", path("upper")},
			wantStdout: installSuccessOutput("CURSOR", path("upper")),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				assertJSONFile(t, path("upper"), map[string]any{
					"mcpServers": map[string]any{"tiger": tigerServerEntry()},
				})
			}},
		},
		{
			name:       "add alias",
			args:       []string{"mcp", "add", "cursor", "--no-backup", "--config-path", path("alias")},
			wantStdout: installSuccessOutput("cursor", path("alias")),
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				assertJSONFile(t, path("alias"), map[string]any{
					"mcpServers": map[string]any{"tiger": tigerServerEntry()},
				})
			}},
		},
		{
			// The backup read must fail, so the config sits in a directory the
			// setup hook makes unreadable (and restores, so t.TempDir can
			// clean up). Root ignores file permissions, hence the skip.
			name: "backup fails when config is unreadable",
			args: []string{"mcp", "install", "cursor", "--config-path", lockedConfigPath},
			opts: []runOption{withSetup(func(t *testing.T) {
				if os.Geteuid() == 0 {
					t.Skip("cannot test permission errors as root user")
				}
				if err := os.MkdirAll(lockedDir, 0755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				if err := os.WriteFile(lockedConfigPath, []byte(`{"test": "data"}`), 0644); err != nil {
					t.Fatalf("failed to seed config file: %v", err)
				}
				if err := os.Chmod(lockedDir, 0444); err != nil {
					t.Fatalf("failed to chmod dir: %v", err)
				}
				t.Cleanup(func() { os.Chmod(lockedDir, 0755) })
			})},
			wantErr: fmt.Sprintf("failed to create backup: failed to read original config file: open %s: permission denied", lockedConfigPath),
		},
	})
}

// TestFindClientConfig covers the client-name lookup table: behavior that the
// command-level cases only exercise for cursor.
func TestFindClientConfig(t *testing.T) {
	mappings := []struct {
		clientName   string
		expectedType MCPClient
		expectedName string
	}{
		{"claude-code", ClaudeCode, "Claude Code"},
		{"CLAUDE-CODE", ClaudeCode, "Claude Code"},
		{"cursor", Cursor, "Cursor"},
		{"CURSOR", Cursor, "Cursor"},
		{"windsurf", Windsurf, "Windsurf"},
		{"WindSurf", Windsurf, "Windsurf"},
		{"codex", Codex, "Codex"},
		{"CODEX", Codex, "Codex"},
	}
	for _, m := range mappings {
		t.Run(m.clientName, func(t *testing.T) {
			cfg, err := findClientConfig(m.clientName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.ClientType != m.expectedType {
				t.Errorf("ClientType = %q, want %q", cfg.ClientType, m.expectedType)
			}
			if cfg.Name != m.expectedName {
				t.Errorf("Name = %q, want %q", cfg.Name, m.expectedName)
			}
		})
	}

	t.Run("every client is installable", func(t *testing.T) {
		for _, cfg := range supportedClients {
			found, err := findClientConfig(cfg.EditorNames[0])
			if err != nil {
				t.Fatalf("findClientConfig(%q) failed: %v", cfg.EditorNames[0], err)
			}
			if found.Name == "" {
				t.Errorf("%s: Name should not be empty", cfg.ClientType)
			}
			// Every client needs an install mechanism: JSON patching (path
			// prefix) or a CLI install command.
			if found.MCPServersPathPrefix == "" && found.buildInstallCommand == nil {
				t.Errorf("%s: needs MCPServersPathPrefix or buildInstallCommand", cfg.ClientType)
			}
			// CLI-only clients (no config paths) must have an install command.
			if len(found.ConfigPaths) == 0 && found.buildInstallCommand == nil {
				t.Errorf("%s: CLI-only clients must have buildInstallCommand", cfg.ClientType)
			}
		}
	})
}

// TestFindClientConfigFile covers config file discovery, including each
// client's per-client fallback path.
func TestFindClientConfigFile(t *testing.T) {
	t.Run("errors when no config paths provided", func(t *testing.T) {
		for _, paths := range [][]string{{}, nil} {
			if _, err := findClientConfigFile(paths); err == nil {
				t.Errorf("findClientConfigFile(%v) should error", paths)
			} else {
				assertOutput(t, err.Error(), "no config paths provided")
			}
		}
	})

	t.Run("finds existing config file", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := findClientConfigFile([]string{configPath})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertOutput(t, got, configPath)
	})

	t.Run("falls back to first path when none exist", func(t *testing.T) {
		dir := t.TempDir()
		fallback := filepath.Join(dir, "fallback.json")
		got, err := findClientConfigFile([]string{fallback, filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertOutput(t, got, fallback)
	})

	t.Run("prefers first existing file", func(t *testing.T) {
		dir := t.TempDir()
		first := filepath.Join(dir, "first.json")
		second := filepath.Join(dir, "second.json")
		for _, p := range []string{first, second} {
			if err := os.WriteFile(p, []byte(`{}`), 0644); err != nil {
				t.Fatal(err)
			}
		}
		got, err := findClientConfigFile([]string{first, second})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertOutput(t, got, first)
	})

	t.Run("expands environment variables", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "config.json")
		if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("FINDCONFIGFILE_TEST_DIR", dir)
		got, err := findClientConfigFile([]string{"$FINDCONFIGFILE_TEST_DIR/config.json"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertOutput(t, got, configPath)
	})

	t.Run("per-client fallback paths", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		for _, cfg := range supportedClients {
			if len(cfg.ConfigPaths) == 0 {
				continue
			}
			got, err := findClientConfigFile(cfg.ConfigPaths)
			if err != nil {
				t.Errorf("%s: unexpected error: %v", cfg.Name, err)
				continue
			}
			if want := util.ExpandPath(cfg.ConfigPaths[0]); got != want {
				t.Errorf("%s: fallback = %q, want %q", cfg.Name, got, want)
			}
		}
	})

}

// TestAddMCPServerViaCLI covers the CLI-based install path, which the command
// tests can't reach without real client binaries on PATH.
func TestAddMCPServerViaCLI(t *testing.T) {
	t.Run("errors when no install command configured", func(t *testing.T) {
		cfg := &clientConfig{ClientType: "test-client", Name: "Test Client"}
		err := addMCPServerViaCLI(cfg, "tiger", "/path/to/tiger", []string{"mcp", "start"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, err.Error(), "no install command configured for client Test Client")
	})

	t.Run("errors when command execution fails", func(t *testing.T) {
		cfg := &clientConfig{
			ClientType: "test-client",
			Name:       "Test Client",
			buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
				return []string{"nonexistent-command-12345", "arg1", "arg2"}, nil
			},
		}
		err := addMCPServerViaCLI(cfg, "tiger", "/path/to/tiger", []string{"mcp", "start"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to run Test Client installation command") {
			t.Errorf("error should mention installation command failure, got: %v", err)
		}
	})

	// The install commands themselves would exec the real client binaries, so
	// pin each CLI-based client's exact argv here instead. The command-level
	// "cli-based client" case executes claude-code's end-to-end via a stub.
	t.Run("builds the expected command per client", func(t *testing.T) {
		want := map[MCPClient][]string{
			ClaudeCode: {"claude", "mcp", "add", "-s", "user", "tiger", "/path/to/tiger", "mcp", "start"},
			Codex:      {"codex", "mcp", "add", "tiger", "/path/to/tiger", "mcp", "start"},
			Gemini:     {"gemini", "mcp", "add", "-s", "user", "tiger", "/path/to/tiger", "mcp", "start"},
			VSCode:     {"code", "--add-mcp", `{"args":["mcp","start"],"command":"/path/to/tiger","name":"tiger"}`},
			KiroCLI:    {"kiro-cli", "mcp", "add", "--name", "tiger", "--command", "/path/to/tiger", "--args", "mcp,start"},
			Copilot:    {"copilot", "mcp", "add", "tiger", "--", "/path/to/tiger", "mcp", "start"},
		}
		for _, cfg := range supportedClients {
			if cfg.buildInstallCommand == nil {
				continue
			}
			wantCmd, ok := want[cfg.ClientType]
			if !ok {
				t.Errorf("%s: CLI-based client missing from expected command table", cfg.ClientType)
				continue
			}
			got, err := cfg.BuildInstallCommand("tiger", "/path/to/tiger", []string{"mcp", "start"})
			if err != nil {
				t.Errorf("%s: unexpected error: %v", cfg.ClientType, err)
				continue
			}
			if diff := cmp.Diff(wantCmd, got); diff != "" {
				t.Errorf("%s: install command mismatch (-want +got):\n%s", cfg.ClientType, diff)
			}
		}
	})
}

// TestAddMCPServerViaJSON covers the path-prefix parameterization, which the
// command tests can't reach: every supported JSON client uses /mcpServers.
func TestAddMCPServerViaJSON(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := addMCPServerViaJSON(configPath, "/servers", "tiger", "tiger", []string{"mcp", "start"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONFile(t, configPath, map[string]any{
		"servers": map[string]any{"tiger": tigerServerEntry()},
	})
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
