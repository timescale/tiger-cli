package cmd

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/config"
)

// showValues returns the values `config show` reports for a default config in
// configDir, with overrides replacing individual keys.
func showValues(configDir string, overrides map[string]any) map[string]any {
	values := map[string]any{
		"api_url":          "https://console.cloud.tigerdata.com/public/api/v1",
		"analytics":        true,
		"color":            true,
		"config_dir":       configDir,
		"console_url":      "https://console.cloud.tigerdata.com",
		"docs_mcp":         true,
		"docs_mcp_url":     "https://mcp.tigerdata.com/docs?disabled_skills=ghost-database",
		"gateway_url":      "https://console.cloud.tigerdata.com/api",
		"mcp_max_rows":     100,
		"output":           "table",
		"password_storage": "keyring",
		"read_only":        false,
		"releases_url":     "https://cli.tigerdata.com",
		"service_id":       "",
		"version_check":    true,
	}
	maps.Copy(values, overrides)
	return values
}

// showJSON renders the expected `config show` JSON output (keys in struct
// field order) for a default config in configDir with the given overrides.
func showJSON(t *testing.T, configDir string, overrides map[string]any) string {
	t.Helper()
	values := showValues(configDir, overrides)
	keys := []string{
		"api_url", "analytics", "color", "config_dir", "console_url",
		"docs_mcp", "docs_mcp_url", "gateway_url", "mcp_max_rows", "output",
		"password_storage", "read_only", "releases_url", "service_id",
		"version_check",
	}
	var b strings.Builder
	b.WriteString("{\n")
	for i, key := range keys {
		value, err := json.Marshal(values[key])
		if err != nil {
			t.Fatalf("failed to marshal %s: %v", key, err)
		}
		fmt.Fprintf(&b, "  %q: %s", key, value)
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// readConfigFile parses the config file persisted in configDir.
func readConfigFile(t *testing.T, configDir string) map[string]any {
	t.Helper()
	contents, err := os.ReadFile(config.GetConfigFile(configDir))
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	var values map[string]any
	if err := yaml.Unmarshal(contents, &values); err != nil {
		t.Fatalf("failed to parse config file: %v", err)
	}
	return values
}

// checkConfigFile returns a check func asserting that the persisted config
// file contains exactly the given keys and values.
func checkConfigFile(want map[string]any) func(t *testing.T, result cmdResult) {
	return func(t *testing.T, result cmdResult) {
		t.Helper()
		got := readConfigFile(t, result.configDir)
		if got == nil {
			got = map[string]any{}
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("config file mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestConfigCmd(t *testing.T) {
	t.Run("cfg alias", func(t *testing.T) {
		result := runCommand(t, []string{"cfg", "set", "service_id", "alias-service"}, nil)
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "Set service_id = alias-service\n")
		checkConfigFile(map[string]any{"service_id": "alias-service"})(t, result)
	})

	t.Run("set show unset reset workflow", func(t *testing.T) {
		result := runCommand(t, []string{"config", "set", "service_id", "integration-test"}, nil)
		if result.err != nil {
			t.Fatalf("config set service_id failed: %v", result.err)
		}
		dir := result.configDir

		result = runCommand(t, []string{"config", "set", "output", "json"}, nil, withConfigDir(dir))
		if result.err != nil {
			t.Fatalf("config set output failed: %v", result.err)
		}

		// The configured output format applies to `config show` itself.
		result = runCommand(t, []string{"config", "show"}, nil, withConfigDir(dir))
		if result.err != nil {
			t.Fatalf("config show failed: %v", result.err)
		}
		assertOutput(t, result.stdout, showJSON(t, dir, map[string]any{
			"output":     "json",
			"service_id": "integration-test",
		}))

		result = runCommand(t, []string{"config", "unset", "service_id"}, nil, withConfigDir(dir))
		if result.err != nil {
			t.Fatalf("config unset failed: %v", result.err)
		}

		result = runCommand(t, []string{"config", "show"}, nil, withConfigDir(dir))
		if result.err != nil {
			t.Fatalf("config show after unset failed: %v", result.err)
		}
		assertOutput(t, result.stdout, showJSON(t, dir, map[string]any{
			"output": "json",
		}))

		result = runCommand(t, []string{"config", "reset"}, nil, withConfigDir(dir))
		if result.err != nil {
			t.Fatalf("config reset failed: %v", result.err)
		}
		checkConfigFile(map[string]any{})(t, result)

		result = runCommand(t, []string{"config", "show", "-o", "json"}, nil, withConfigDir(dir))
		if result.err != nil {
			t.Fatalf("config show after reset failed: %v", result.err)
		}
		assertOutput(t, result.stdout, showJSON(t, dir, nil))
	})
}
