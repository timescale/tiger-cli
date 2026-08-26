package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

// renderTable mimics tablewriter's default box layout, so expected table
// output can be computed for values only known at runtime (the temp config
// dir, whose length sets the VALUE column width).
func renderTable(rows [][2]string) string {
	inner := [2]int{len("PROPERTY") + 2, len("VALUE") + 2}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell)+2 > inner[i] {
				inner[i] = len(cell) + 2
			}
		}
	}
	center := func(s string, w int) string {
		left := (w - len(s)) / 2
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-len(s)-left)
	}
	cell := func(s string, w int) string {
		return " " + s + strings.Repeat(" ", w-len(s)-1)
	}
	border := func(l, m, r string) string {
		return l + strings.Repeat("─", inner[0]) + m + strings.Repeat("─", inner[1]) + r + "\n"
	}
	var b strings.Builder
	b.WriteString(border("┌", "┬", "┐"))
	b.WriteString("│" + center("PROPERTY", inner[0]) + "│" + center("VALUE", inner[1]) + "│\n")
	b.WriteString(border("├", "┼", "┤"))
	for _, row := range rows {
		b.WriteString("│" + cell(row[0], inner[0]) + "│" + cell(row[1], inner[1]) + "│\n")
	}
	b.WriteString(border("└", "┴", "┘"))
	return b.String()
}

func showTableValue(v any) string {
	switch v := v.(type) {
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// showTable renders the expected full `config show` table (rows in the order
// outputTable prints them) for a default config with the given overrides.
func showTable(configDir string, overrides map[string]any) string {
	values := showValues(configDir, overrides)
	keys := []string{
		"api_url", "analytics", "config_dir", "console_url", "docs_mcp",
		"docs_mcp_url", "gateway_url", "mcp_max_rows", "color", "output",
		"password_storage", "read_only", "releases_url", "service_id",
		"version_check",
	}
	rows := make([][2]string, len(keys))
	for i, key := range keys {
		rows[i] = [2]string{key, showTableValue(values[key])}
	}
	return renderTable(rows)
}

// showYAML renders the expected `config show` YAML output (keys sorted
// alphabetically) for a default config with the given overrides.
func showYAML(configDir string, overrides map[string]any) string {
	values := showValues(configDir, overrides)
	keys := []string{
		"analytics", "api_url", "color", "config_dir", "console_url",
		"docs_mcp", "docs_mcp_url", "gateway_url", "mcp_max_rows", "output",
		"password_storage", "read_only", "releases_url", "service_id",
		"version_check",
	}
	var b strings.Builder
	for _, key := range keys {
		value := fmt.Sprintf("%v", values[key])
		if value == "" {
			value = `""`
		}
		fmt.Fprintf(&b, "%s: %s\n", key, value)
	}
	return b.String()
}

func TestConfigShowCmd(t *testing.T) {
	dir := t.TempDir()

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
			opts:       []runOption{withConfigDir(dir), withConfig(map[string]any{})},
			wantStdout: showTable(dir, nil),
		},
		{
			name: "table output with configured values",
			args: []string{"config", "show"},
			opts: []runOption{withConfigDir(dir), withConfig(map[string]any{
				"api_url":          "https://test.api.com/v1",
				"service_id":       "test-service",
				"analytics":        false,
				"password_storage": "pgpass",
			})},
			wantStdout: showTable(dir, map[string]any{
				"api_url":          "https://test.api.com/v1",
				"service_id":       "test-service",
				"analytics":        false,
				"password_storage": "pgpass",
			}),
		},
		{
			name: "json output from config file",
			args: []string{"config", "show"},
			opts: []runOption{withConfigDir(dir), withConfig(map[string]any{
				"output":    "json",
				"api_url":   "https://json.api.com/v1",
				"analytics": false,
			})},
			wantStdout: showJSON(t, dir, map[string]any{
				"output":    "json",
				"api_url":   "https://json.api.com/v1",
				"analytics": false,
			}),
		},
		{
			name:       "output flag changes format but not reported value",
			args:       []string{"config", "show", "-o", "json"},
			opts:       []runOption{withConfigDir(dir), withConfig(map[string]any{})},
			wantStdout: showJSON(t, dir, nil),
		},
		{
			name: "output env var changes format but not reported value",
			args: []string{"config", "show"},
			opts: []runOption{
				withConfigDir(dir),
				withConfig(map[string]any{"output": "table"}),
				withEnv("TIGER_OUTPUT", "json"),
			},
			wantStdout: showJSON(t, dir, map[string]any{"output": "table"}),
		},
		{
			name: "yaml output from config file",
			args: []string{"config", "show"},
			opts: []runOption{withConfigDir(dir), withConfig(map[string]any{
				"output":  "yaml",
				"api_url": "https://yaml.api.com/v1",
			})},
			wantStdout: showYAML(dir, map[string]any{
				"output":  "yaml",
				"api_url": "https://yaml.api.com/v1",
			}),
		},
		{
			name:       "yaml output via flag",
			args:       []string{"config", "show", "-o", "yaml"},
			opts:       []runOption{withConfigDir(dir), withConfig(map[string]any{})},
			wantStdout: showYAML(dir, nil),
		},
		{
			name: "no-defaults shows only configured values",
			args: []string{"config", "show", "--no-defaults"},
			opts: []runOption{withConfigDir(dir), withConfig(map[string]any{
				"service_id": "test-service",
				"analytics":  false,
			})},
			wantStdout: renderTable([][2]string{
				{"analytics", "false"},
				{"config_dir", dir},
				{"service_id", "test-service"},
			}),
		},
		{
			name: "with-env applies env overrides",
			args: []string{"config", "show", "--with-env"},
			opts: []runOption{
				withConfigDir(dir),
				withConfig(map[string]any{}),
				withEnv("TIGER_SERVICE_ID", "env-service"),
			},
			wantStdout: showTable(dir, map[string]any{"service_id": "env-service"}),
		},
		{
			name: "env override ignored without with-env",
			args: []string{"config", "show"},
			opts: []runOption{
				withConfigDir(dir),
				withConfig(map[string]any{}),
				withEnv("TIGER_SERVICE_ID", "env-service"),
			},
			wantStdout: showTable(dir, nil),
		},
		{
			name: "config-dir flag overrides TIGER_CONFIG_DIR env var",
			args: []string{"config", "show"},
			opts: []runOption{
				withConfigDir(dir),
				withConfig(map[string]any{"api_url": "https://flag-test.api.com/v1"}),
				withEnv("TIGER_CONFIG_DIR", envDir),
			},
			wantStdout: showTable(dir, map[string]any{"api_url": "https://flag-test.api.com/v1"}),
		},
		{
			// Configs written by older CLI versions used a
			// version_check_interval duration; 0 (checks disabled) must carry
			// over to version_check=false rather than the default true.
			name:       "legacy version_check_interval 0 disables version_check",
			args:       []string{"config", "show"},
			opts:       []runOption{withConfigDir(dir), withConfig(map[string]any{"version_check_interval": 0})},
			wantStdout: showTable(dir, map[string]any{"version_check": false}),
		},
		{
			name:       "list alias",
			args:       []string{"config", "list"},
			opts:       []runOption{withConfigDir(dir), withConfig(map[string]any{})},
			wantStdout: showTable(dir, nil),
		},
		{
			name:       "ls alias",
			args:       []string{"config", "ls"},
			opts:       []runOption{withConfigDir(dir), withConfig(map[string]any{})},
			wantStdout: showTable(dir, nil),
		},
	}

	runCmdTests(t, tests)
}
