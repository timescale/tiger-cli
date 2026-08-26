package cmd

import (
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

// Full `config show` output for an all-defaults config, per format. Cases
// whose output differs use their own literals.
const (
	configShowDefaultsTable = `┌──────────────────┬───────────────────────────────────────────────────────────────┐
│     PROPERTY     │                             VALUE                             │
├──────────────────┼───────────────────────────────────────────────────────────────┤
│ analytics        │ true                                                          │
│ api_url          │ https://console.cloud.tigerdata.com/public/api/v1             │
│ color            │ true                                                          │
│ console_url      │ https://console.cloud.tigerdata.com                           │
│ docs_mcp         │ true                                                          │
│ docs_mcp_url     │ https://mcp.tigerdata.com/docs?disabled_skills=ghost-database │
│ gateway_url      │ https://console.cloud.tigerdata.com/api                       │
│ mcp_max_rows     │ 100                                                           │
│ output           │ table                                                         │
│ password_storage │ keyring                                                       │
│ read_only        │ false                                                         │
│ releases_url     │ https://cli.tigerdata.com                                     │
│ service_id       │                                                               │
│ version_check    │ true                                                          │
└──────────────────┴───────────────────────────────────────────────────────────────┘
`

	configShowDefaultsJSON = `{
  "analytics": true,
  "api_url": "https://console.cloud.tigerdata.com/public/api/v1",
  "color": true,
  "console_url": "https://console.cloud.tigerdata.com",
  "docs_mcp": true,
  "docs_mcp_url": "https://mcp.tigerdata.com/docs?disabled_skills=ghost-database",
  "gateway_url": "https://console.cloud.tigerdata.com/api",
  "mcp_max_rows": 100,
  "output": "table",
  "password_storage": "keyring",
  "read_only": false,
  "releases_url": "https://cli.tigerdata.com",
  "service_id": "",
  "version_check": true
}
`

	configShowDefaultsYAML = `analytics: true
api_url: https://console.cloud.tigerdata.com/public/api/v1
color: true
console_url: https://console.cloud.tigerdata.com
docs_mcp: true
docs_mcp_url: https://mcp.tigerdata.com/docs?disabled_skills=ghost-database
gateway_url: https://console.cloud.tigerdata.com/api
mcp_max_rows: 100
output: table
password_storage: keyring
read_only: false
releases_url: https://cli.tigerdata.com
service_id: ""
version_check: true
`
)

func TestConfigShowCmd(t *testing.T) {
	// A second config dir, pointed at by TIGER_CONFIG_DIR in one case to prove
	// the --config-dir flag takes precedence over the env var.
	envDir := t.TempDir()
	if _, err := config.UseTestConfig(envDir, map[string]any{"api_url": "https://env.api.com/v1"}); err != nil {
		t.Fatalf("failed to seed env config dir: %v", err)
	}

	tests := []cmdTest{
		{
			name:    "unexpected argument",
			args:    []string{"config", "show", "extra"},
			wantErr: `unknown command "extra" for "tiger config show"`,
		},
		{
			name:       "table output defaults",
			args:       []string{"config", "show"},
			wantStdout: configShowDefaultsTable,
		},
		{
			name: "table output with configured values",
			args: []string{"config", "show"},
			opts: []runOption{withConfig(map[string]any{
				"api_url":          "https://test.api.com/v1",
				"service_id":       "test-service",
				"analytics":        false,
				"password_storage": "pgpass",
			})},
			wantStdout: `┌──────────────────┬───────────────────────────────────────────────────────────────┐
│     PROPERTY     │                             VALUE                             │
├──────────────────┼───────────────────────────────────────────────────────────────┤
│ analytics        │ false                                                         │
│ api_url          │ https://test.api.com/v1                                       │
│ color            │ true                                                          │
│ console_url      │ https://console.cloud.tigerdata.com                           │
│ docs_mcp         │ true                                                          │
│ docs_mcp_url     │ https://mcp.tigerdata.com/docs?disabled_skills=ghost-database │
│ gateway_url      │ https://console.cloud.tigerdata.com/api                       │
│ mcp_max_rows     │ 100                                                           │
│ output           │ table                                                         │
│ password_storage │ pgpass                                                        │
│ read_only        │ false                                                         │
│ releases_url     │ https://cli.tigerdata.com                                     │
│ service_id       │ test-service                                                  │
│ version_check    │ true                                                          │
└──────────────────┴───────────────────────────────────────────────────────────────┘
`,
		},
		{
			name: "json output from config file",
			args: []string{"config", "show"},
			opts: []runOption{withConfig(map[string]any{
				"output":    "json",
				"api_url":   "https://json.api.com/v1",
				"analytics": false,
			})},
			wantStdout: `{
  "analytics": false,
  "api_url": "https://json.api.com/v1",
  "color": true,
  "console_url": "https://console.cloud.tigerdata.com",
  "docs_mcp": true,
  "docs_mcp_url": "https://mcp.tigerdata.com/docs?disabled_skills=ghost-database",
  "gateway_url": "https://console.cloud.tigerdata.com/api",
  "mcp_max_rows": 100,
  "output": "json",
  "password_storage": "keyring",
  "read_only": false,
  "releases_url": "https://cli.tigerdata.com",
  "service_id": "",
  "version_check": true
}
`,
		},
		{
			name:       "output flag changes format but not reported value",
			args:       []string{"config", "show", "-o", "json"},
			wantStdout: configShowDefaultsJSON,
		},
		{
			name: "output env var changes format but not reported value",
			args: []string{"config", "show"},
			opts: []runOption{
				withConfig(map[string]any{"output": "table"}),
				withEnv("TIGER_OUTPUT", "json"),
			},
			wantStdout: configShowDefaultsJSON,
		},
		{
			name: "yaml output from config file",
			args: []string{"config", "show"},
			opts: []runOption{withConfig(map[string]any{
				"output":  "yaml",
				"api_url": "https://yaml.api.com/v1",
			})},
			wantStdout: `analytics: true
api_url: https://yaml.api.com/v1
color: true
console_url: https://console.cloud.tigerdata.com
docs_mcp: true
docs_mcp_url: https://mcp.tigerdata.com/docs?disabled_skills=ghost-database
gateway_url: https://console.cloud.tigerdata.com/api
mcp_max_rows: 100
output: yaml
password_storage: keyring
read_only: false
releases_url: https://cli.tigerdata.com
service_id: ""
version_check: true
`,
		},
		{
			name:       "yaml output via flag",
			args:       []string{"config", "show", "-o", "yaml"},
			wantStdout: configShowDefaultsYAML,
		},
		{
			name: "no-defaults shows only configured values",
			args: []string{"config", "show", "--no-defaults"},
			opts: []runOption{withConfig(map[string]any{
				"service_id": "test-service",
				"analytics":  false,
			})},
			wantStdout: `┌────────────┬──────────────┐
│  PROPERTY  │    VALUE     │
├────────────┼──────────────┤
│ analytics  │ false        │
│ service_id │ test-service │
└────────────┴──────────────┘
`,
		},
		{
			name: "with-env applies env overrides",
			args: []string{"config", "show", "--with-env"},
			opts: []runOption{withEnv("TIGER_SERVICE_ID", "env-service")},
			wantStdout: `┌──────────────────┬───────────────────────────────────────────────────────────────┐
│     PROPERTY     │                             VALUE                             │
├──────────────────┼───────────────────────────────────────────────────────────────┤
│ analytics        │ true                                                          │
│ api_url          │ https://console.cloud.tigerdata.com/public/api/v1             │
│ color            │ true                                                          │
│ console_url      │ https://console.cloud.tigerdata.com                           │
│ docs_mcp         │ true                                                          │
│ docs_mcp_url     │ https://mcp.tigerdata.com/docs?disabled_skills=ghost-database │
│ gateway_url      │ https://console.cloud.tigerdata.com/api                       │
│ mcp_max_rows     │ 100                                                           │
│ output           │ table                                                         │
│ password_storage │ keyring                                                       │
│ read_only        │ false                                                         │
│ releases_url     │ https://cli.tigerdata.com                                     │
│ service_id       │ env-service                                                   │
│ version_check    │ true                                                          │
└──────────────────┴───────────────────────────────────────────────────────────────┘
`,
		},
		{
			name:       "env override ignored without with-env",
			args:       []string{"config", "show"},
			opts:       []runOption{withEnv("TIGER_SERVICE_ID", "env-service")},
			wantStdout: configShowDefaultsTable,
		},
		{
			name: "config-dir flag overrides TIGER_CONFIG_DIR env var",
			args: []string{"config", "show"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": "https://flag-test.api.com/v1"}),
				withEnv("TIGER_CONFIG_DIR", envDir),
			},
			wantStdout: `┌──────────────────┬───────────────────────────────────────────────────────────────┐
│     PROPERTY     │                             VALUE                             │
├──────────────────┼───────────────────────────────────────────────────────────────┤
│ analytics        │ true                                                          │
│ api_url          │ https://flag-test.api.com/v1                                  │
│ color            │ true                                                          │
│ console_url      │ https://console.cloud.tigerdata.com                           │
│ docs_mcp         │ true                                                          │
│ docs_mcp_url     │ https://mcp.tigerdata.com/docs?disabled_skills=ghost-database │
│ gateway_url      │ https://console.cloud.tigerdata.com/api                       │
│ mcp_max_rows     │ 100                                                           │
│ output           │ table                                                         │
│ password_storage │ keyring                                                       │
│ read_only        │ false                                                         │
│ releases_url     │ https://cli.tigerdata.com                                     │
│ service_id       │                                                               │
│ version_check    │ true                                                          │
└──────────────────┴───────────────────────────────────────────────────────────────┘
`,
		},
		{
			// Configs written by older CLI versions used a
			// version_check_interval duration; 0 (checks disabled) must carry
			// over to version_check=false rather than the default true.
			name: "legacy version_check_interval 0 disables version_check",
			args: []string{"config", "show"},
			opts: []runOption{withConfig(map[string]any{"version_check_interval": 0})},
			wantStdout: `┌──────────────────┬───────────────────────────────────────────────────────────────┐
│     PROPERTY     │                             VALUE                             │
├──────────────────┼───────────────────────────────────────────────────────────────┤
│ analytics        │ true                                                          │
│ api_url          │ https://console.cloud.tigerdata.com/public/api/v1             │
│ color            │ true                                                          │
│ console_url      │ https://console.cloud.tigerdata.com                           │
│ docs_mcp         │ true                                                          │
│ docs_mcp_url     │ https://mcp.tigerdata.com/docs?disabled_skills=ghost-database │
│ gateway_url      │ https://console.cloud.tigerdata.com/api                       │
│ mcp_max_rows     │ 100                                                           │
│ output           │ table                                                         │
│ password_storage │ keyring                                                       │
│ read_only        │ false                                                         │
│ releases_url     │ https://cli.tigerdata.com                                     │
│ service_id       │                                                               │
│ version_check    │ false                                                         │
└──────────────────┴───────────────────────────────────────────────────────────────┘
`,
		},
		{
			name:       "list alias",
			args:       []string{"config", "list"},
			wantStdout: configShowDefaultsTable,
		},
		{
			name:       "ls alias",
			args:       []string{"config", "ls"},
			wantStdout: configShowDefaultsTable,
		},
	}

	runCmdTests(t, tests)
}
