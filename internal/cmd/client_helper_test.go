package cmd

import "testing"

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
		{"devin", Devin, "Devin"},
		{"DEVIN", Devin, "Devin"},
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
			if found.MCPServersPathPrefix == "" && found.buildMCPInstallCommand == nil {
				t.Errorf("%s: needs MCPServersPathPrefix or buildMCPInstallCommand", cfg.ClientType)
			}
			// CLI-only clients (no config paths) must have an install command.
			if len(found.MCPConfigPaths) == 0 && found.buildMCPInstallCommand == nil {
				t.Errorf("%s: CLI-only clients must have buildMCPInstallCommand", cfg.ClientType)
			}
			if found.SkillsDir == "" {
				t.Errorf("%s: needs SkillsDir", cfg.ClientType)
			}
		}
	})
}
