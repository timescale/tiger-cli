package cmd

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/config"
)

// Full `config list` output for an all-defaults config, per format, without
// and with --all. Cases whose output differs use their own literals.
const (
	configListDefaultsTable = `┌──────────────────┬─────────┐
│     PROPERTY     │  VALUE  │
├──────────────────┼─────────┤
│ analytics        │ true    │
│ color            │ true    │
│ docs_mcp         │ true    │
│ mcp_max_rows     │ 100     │
│ output           │ table   │
│ password_storage │ keyring │
│ read_only        │ off     │
│ service_id       │         │
│ version_check    │ true    │
└──────────────────┴─────────┘
`

	configListDefaultsJSON = `{
  "analytics": true,
  "color": true,
  "docs_mcp": true,
  "mcp_max_rows": 100,
  "output": "table",
  "password_storage": "keyring",
  "read_only": "off",
  "service_id": "",
  "version_check": true
}
`

	configListDefaultsYAML = `analytics: true
color: true
docs_mcp: true
mcp_max_rows: 100
output: table
password_storage: keyring
read_only: "off"
service_id: ""
version_check: true
`

	configListAllDefaultsTable = `┌──────────────────┬───────────────────────────────────────────────────────────────┐
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
│ read_only        │ off                                                           │
│ releases_url     │ https://cli.tigerdata.com                                     │
│ service_id       │                                                               │
│ version_check    │ true                                                          │
└──────────────────┴───────────────────────────────────────────────────────────────┘
`
)

func TestConfigListCmd(t *testing.T) {
	// A second config dir, pointed at by TIGER_CONFIG_DIR in one case to prove
	// the --config-dir flag takes precedence over the env var.
	envDir := t.TempDir()
	writeConfigFile(t, envDir, map[string]any{"service_id": "env-dir-service"})

	runCmdTests(t, []cmdTest{
		{
			name:    "unexpected argument",
			args:    []string{"config", "list", "extra"},
			wantErr: `unknown command "extra" for "tiger config list"`,
		},
		{
			name:       "table output defaults",
			args:       []string{"config", "list"},
			wantStdout: configListDefaultsTable,
		},
		{
			name: "table output with configured values",
			args: []string{"config", "list"},
			opts: []runOption{withConfig(map[string]any{
				"service_id":       "test-service",
				"analytics":        false,
				"password_storage": "pgpass",
			})},
			wantStdout: `┌──────────────────┬──────────────┐
│     PROPERTY     │    VALUE     │
├──────────────────┼──────────────┤
│ analytics        │ false        │
│ color            │ true         │
│ docs_mcp         │ true         │
│ mcp_max_rows     │ 100          │
│ output           │ table        │
│ password_storage │ pgpass       │
│ read_only        │ off          │
│ service_id       │ test-service │
│ version_check    │ true         │
└──────────────────┴──────────────┘
`,
		},
		{
			name: "json output from config file",
			args: []string{"config", "list"},
			opts: []runOption{withConfig(map[string]any{
				"output":    "json",
				"analytics": false,
			})},
			wantStdout: `{
  "analytics": false,
  "color": true,
  "docs_mcp": true,
  "mcp_max_rows": 100,
  "output": "json",
  "password_storage": "keyring",
  "read_only": "off",
  "service_id": "",
  "version_check": true
}
`,
		},
		{
			name:       "output flag changes format but not reported value",
			args:       []string{"config", "list", "-o", "json"},
			wantStdout: configListDefaultsJSON,
		},
		{
			name: "output env var changes format but not reported value",
			args: []string{"config", "list"},
			opts: []runOption{
				withConfig(map[string]any{"output": "table"}),
				withEnv("TIGER_OUTPUT", "json"),
			},
			wantStdout: configListDefaultsJSON,
		},
		{
			name: "yaml output from config file",
			args: []string{"config", "list"},
			opts: []runOption{withConfig(map[string]any{
				"output":     "yaml",
				"service_id": "yaml-service",
			})},
			wantStdout: `analytics: true
color: true
docs_mcp: true
mcp_max_rows: 100
output: yaml
password_storage: keyring
read_only: "off"
service_id: yaml-service
version_check: true
`,
		},
		{
			name:       "yaml output via flag",
			args:       []string{"config", "list", "-o", "yaml"},
			wantStdout: configListDefaultsYAML,
		},
		{
			name: "no-defaults shows only configured values",
			args: []string{"config", "list", "--no-defaults"},
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
			args: []string{"config", "list", "--with-env"},
			opts: []runOption{withEnv("TIGER_SERVICE_ID", "env-service")},
			wantStdout: `┌──────────────────┬─────────────┐
│     PROPERTY     │    VALUE    │
├──────────────────┼─────────────┤
│ analytics        │ true        │
│ color            │ true        │
│ docs_mcp         │ true        │
│ mcp_max_rows     │ 100         │
│ output           │ table       │
│ password_storage │ keyring     │
│ read_only        │ off         │
│ service_id       │ env-service │
│ version_check    │ true        │
└──────────────────┴─────────────┘
`,
		},
		{
			name:       "env override ignored without with-env",
			args:       []string{"config", "list"},
			opts:       []runOption{withEnv("TIGER_SERVICE_ID", "env-service")},
			wantStdout: configListDefaultsTable,
		},
		{
			name: "private keys hidden even when set in config file",
			args: []string{"config", "list", "--no-defaults"},
			opts: []runOption{withConfig(map[string]any{
				"api_url":      "https://test.api.com/v1",
				"console_url":  "https://test.console.com",
				"docs_mcp_url": "https://test.docs.com/mcp",
				"gateway_url":  "https://test.api.com/gw",
				"releases_url": "https://test.releases.com",
				"service_id":   "test-service",
			})},
			wantStdout: `┌────────────┬──────────────┐
│  PROPERTY  │    VALUE     │
├────────────┼──────────────┤
│ service_id │ test-service │
└────────────┴──────────────┘
`,
		},
		{
			name:       "private keys hidden even when set by env var",
			args:       []string{"config", "list", "--with-env"},
			opts:       []runOption{withEnv("TIGER_API_URL", "https://env.api.com/v1")},
			wantStdout: configListDefaultsTable,
		},
		{
			name:       "all includes private defaults",
			args:       []string{"config", "list", "--all"},
			wantStdout: configListAllDefaultsTable,
		},
		{
			name:       "all shorthand",
			args:       []string{"config", "list", "-a"},
			wantStdout: configListAllDefaultsTable,
		},
		{
			name: "all includes private values from config file",
			args: []string{"config", "list", "--all", "-o", "json"},
			opts: []runOption{withConfig(map[string]any{
				"api_url":      "https://test.api.com/v1",
				"console_url":  "https://test.console.com",
				"docs_mcp_url": "https://test.docs.com/mcp",
				"gateway_url":  "https://test.api.com/gw",
				"releases_url": "https://test.releases.com",
			})},
			wantStdout: `{
  "analytics": true,
  "api_url": "https://test.api.com/v1",
  "color": true,
  "console_url": "https://test.console.com",
  "docs_mcp": true,
  "docs_mcp_url": "https://test.docs.com/mcp",
  "gateway_url": "https://test.api.com/gw",
  "mcp_max_rows": 100,
  "output": "table",
  "password_storage": "keyring",
  "read_only": "off",
  "releases_url": "https://test.releases.com",
  "service_id": "",
  "version_check": true
}
`,
		},
		{
			name: "all with no-defaults shows only configured values",
			args: []string{"config", "list", "--all", "--no-defaults"},
			opts: []runOption{withConfig(map[string]any{
				"api_url":    "https://test.api.com/v1",
				"service_id": "test-service",
			})},
			wantStdout: `┌────────────┬─────────────────────────┐
│  PROPERTY  │          VALUE          │
├────────────┼─────────────────────────┤
│ api_url    │ https://test.api.com/v1 │
│ service_id │ test-service            │
└────────────┴─────────────────────────┘
`,
		},
		{
			name: "all with with-env applies private env overrides",
			args: []string{"config", "list", "--all", "--with-env", "-o", "yaml"},
			opts: []runOption{withEnv("TIGER_API_URL", "https://env.api.com/v1")},
			wantStdout: `analytics: true
api_url: https://env.api.com/v1
color: true
console_url: https://console.cloud.tigerdata.com
docs_mcp: true
docs_mcp_url: https://mcp.tigerdata.com/docs?disabled_skills=ghost-database
gateway_url: https://console.cloud.tigerdata.com/api
mcp_max_rows: 100
output: table
password_storage: keyring
read_only: "off"
releases_url: https://cli.tigerdata.com
service_id: ""
version_check: true
`,
		},
		{
			name: "config-dir flag overrides TIGER_CONFIG_DIR env var",
			args: []string{"config", "list", "--no-defaults"},
			opts: []runOption{
				withConfig(map[string]any{"service_id": "flag-dir-service"}),
				withEnv("TIGER_CONFIG_DIR", envDir),
			},
			wantStdout: `┌────────────┬──────────────────┐
│  PROPERTY  │      VALUE       │
├────────────┼──────────────────┤
│ service_id │ flag-dir-service │
└────────────┴──────────────────┘
`,
		},
		{
			// Configs written by older CLI versions used a
			// version_check_interval duration; 0 (checks disabled) must carry
			// over to version_check=false rather than the default true.
			name: "legacy version_check_interval 0 disables version_check",
			args: []string{"config", "list"},
			opts: []runOption{withConfig(map[string]any{"version_check_interval": 0})},
			wantStdout: `┌──────────────────┬─────────┐
│     PROPERTY     │  VALUE  │
├──────────────────┼─────────┤
│ analytics        │ true    │
│ color            │ true    │
│ docs_mcp         │ true    │
│ mcp_max_rows     │ 100     │
│ output           │ table   │
│ password_storage │ keyring │
│ read_only        │ off     │
│ service_id       │         │
│ version_check    │ false   │
└──────────────────┴─────────┘
`,
		},
		{
			name:       "show alias",
			args:       []string{"config", "show"},
			wantStdout: configListDefaultsTable,
		},
		{
			name:       "ls alias",
			args:       []string{"config", "ls"},
			wantStdout: configListDefaultsTable,
		},
	})
}

// Every config key must be visible in every `config list` format: public keys
// by default, and every key with --all. The table is rendered by a
// hand-written if-chain in outputTable and the other two by ConfigOutput's
// json/yaml tags, so a newly added key can silently fail to show up in one of
// them. The literal expectations above would still pass — they only assert the
// keys that were there when they were written — so assert the full key set
// against the registry instead.
func TestConfigListCoversEveryKey(t *testing.T) {
	for _, variant := range []struct {
		name  string
		flags []string
		want  []string
	}{
		{name: "public", want: config.PublicConfigOptions()},
		{name: "all", flags: []string{"--all"}, want: config.ValidConfigOptions()},
	} {
		t.Run(variant.name, func(t *testing.T) {
			args := append([]string{"config", "list"}, variant.flags...)

			assertKeys := func(t *testing.T, got []string) {
				t.Helper()
				slices.Sort(got)
				if !slices.Equal(variant.want, got) {
					t.Errorf("`config list` keys = %v, want %v", got, variant.want)
				}
			}

			t.Run("table", func(t *testing.T) {
				result := runCommand(t, args, nil)
				var got []string
				for line := range strings.SplitSeq(result.stdout, "\n") {
					cells := strings.Split(line, "\u2502")
					// A data row is "│ key │ value │": empty, key, value, empty.
					if len(cells) != 4 {
						continue
					}
					if key := strings.TrimSpace(cells[1]); key != "" && key != "PROPERTY" {
						got = append(got, key)
					}
				}
				assertKeys(t, got)
			})

			t.Run("json", func(t *testing.T) {
				result := runCommand(t, append(args, "-o", "json"), nil)
				var values map[string]any
				if err := json.Unmarshal([]byte(result.stdout), &values); err != nil {
					t.Fatalf("failed to parse json output: %v", err)
				}
				assertKeys(t, slices.Collect(maps.Keys(values)))
			})

			t.Run("yaml", func(t *testing.T) {
				result := runCommand(t, append(args, "-o", "yaml"), nil)
				var values map[string]any
				if err := yaml.Unmarshal([]byte(result.stdout), &values); err != nil {
					t.Fatalf("failed to parse yaml output: %v", err)
				}
				assertKeys(t, slices.Collect(maps.Keys(values)))
			})
		})
	}
}
