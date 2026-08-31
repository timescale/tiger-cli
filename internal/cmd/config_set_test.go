package cmd

import (
	"testing"
)

func TestConfigSetCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:    "missing arguments",
			args:    []string{"config", "set"},
			wantErr: "accepts 2 arg(s), received 0",
		},
		{
			name:    "missing value",
			args:    []string{"config", "set", "service_id"},
			wantErr: "accepts 2 arg(s), received 1",
		},
		{
			name:    "too many arguments",
			args:    []string{"config", "set", "service_id", "value", "extra"},
			wantErr: "accepts 2 arg(s), received 3",
		},
		{
			name:    "unknown key",
			args:    []string{"config", "set", "unknown", "value"},
			wantErr: "failed to set config: unknown configuration key: unknown",
		},
		{
			name:    "invalid bool value",
			args:    []string{"config", "set", "analytics", "maybe"},
			wantErr: "failed to set config: invalid analytics value: maybe (must be true or false)",
		},
		{
			name:    "invalid int value",
			args:    []string{"config", "set", "mcp_max_rows", "abc"},
			wantErr: "failed to set config: invalid mcp_max_rows value: abc (must be an integer)",
		},
		{
			name:    "int value below minimum",
			args:    []string{"config", "set", "mcp_max_rows", "0"},
			wantErr: "failed to set config: mcp_max_rows must be at least 1, got 0",
		},
		{
			name:    "invalid output format",
			args:    []string{"config", "set", "output", "invalid"},
			wantErr: "failed to set config: invalid output format: invalid (must be one of: json, yaml, table)",
		},
		{
			name:    "invalid password_storage value",
			args:    []string{"config", "set", "password_storage", "secure"},
			wantErr: "failed to set config: invalid password_storage value: secure (must be one of: keyring, pgpass, none)",
		},
		{
			name:       "set api_url",
			args:       []string{"config", "set", "api_url", "https://new.api.com/v1"},
			wantStdout: "Set api_url = https://new.api.com/v1\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": "https://new.api.com/v1"})},
		},
		{
			name:       "set service_id",
			args:       []string{"config", "set", "service_id", "new-service"},
			wantStdout: "Set service_id = new-service\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"service_id": "new-service"})},
		},
		{
			name:       "set output",
			args:       []string{"config", "set", "output", "json"},
			wantStdout: "Set output = json\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"output": "json"})},
		},
		{
			name:       "set analytics false",
			args:       []string{"config", "set", "analytics", "false"},
			wantStdout: "Set analytics = false\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"analytics": false})},
		},
		{
			name:       "set mcp_max_rows",
			args:       []string{"config", "set", "mcp_max_rows", "50"},
			wantStdout: "Set mcp_max_rows = 50\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"mcp_max_rows": 50})},
		},
		{
			name:       "set password_storage pgpass",
			args:       []string{"config", "set", "password_storage", "pgpass"},
			wantStdout: "Set password_storage = pgpass\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"password_storage": "pgpass"})},
		},
		{
			name:       "set password_storage none",
			args:       []string{"config", "set", "password_storage", "none"},
			wantStdout: "Set password_storage = none\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"password_storage": "none"})},
		},
		{
			name:       "set password_storage keyring",
			args:       []string{"config", "set", "password_storage", "keyring"},
			wantStdout: "Set password_storage = keyring\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"password_storage": "keyring"})},
		},
		{
			name:       "set preserves other keys",
			args:       []string{"config", "set", "output", "json"},
			opts:       []runOption{withConfig(map[string]any{"service_id": "existing-service"})},
			wantStdout: "Set output = json\n",
			checks: []checkFunc{checkConfigFile(map[string]any{
				"service_id": "existing-service",
				"output":     "json",
			})},
		},
		{
			name:       "set overwrites existing value",
			args:       []string{"config", "set", "output", "yaml"},
			opts:       []runOption{withConfig(map[string]any{"output": "json"})},
			wantStdout: "Set output = yaml\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"output": "yaml"})},
		},
	})
}
