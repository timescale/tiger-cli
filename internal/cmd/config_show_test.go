package cmd

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestConfigShow_TableOutput(t *testing.T) {
	tmpDir, _ := setupConfigTest(t)

	// Create config file with test data
	configContent := `api_url: https://test.api.com/v1
service_id: test-service
output: table
analytics: false
password_storage: pgpass
`
	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	output, err := executeConfigCommand(t.Context(), "config", "show")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}
	lines := strings.Split(output, "\n")

	// Check table output contains all expected key:value lines
	expectedLines := map[string]string{
		"api_url":          "https://test.api.com/v1",
		"console_url":      "https://console.cloud.tigerdata.com",
		"gateway_url":      "https://console.cloud.tigerdata.com/api",
		"docs_mcp":         "true",
		"docs_mcp_url":     "https://mcp.tigerdata.com/docs?disabled_skills=ghost-database",
		"service_id":       "test-service",
		"output":           "table",
		"analytics":        "false",
		"password_storage": "pgpass",
		"debug":            "false",
		"config_dir":       tmpDir,
		"mcp_max_rows":     strconv.Itoa(config.DefaultMCPMaxRows),
	}

	for key, expectedLine := range expectedLines {
		if !slices.ContainsFunc(lines, func(line string) bool {
			return strings.Contains(line, key) && strings.Contains(line, expectedLine)
		}) {
			t.Errorf("Output should contain line '%s':'%s', got: %s", key, expectedLine, output)
		}
	}
}

func TestConfigShow_JSONOutput(t *testing.T) {
	tmpDir, _ := setupConfigTest(t)

	// Create config file with JSON output format
	configContent := `api_url: https://json.api.com/v1
output: json
analytics: false
password_storage: keyring
`

	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	output, err := executeConfigCommand(t.Context(), "config", "show")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Parse JSON output
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	// Verify ALL JSON keys and their expected values
	expectedValues := map[string]interface{}{
		"api_url":          "https://json.api.com/v1",
		"console_url":      "https://console.cloud.tigerdata.com",
		"gateway_url":      "https://console.cloud.tigerdata.com/api",
		"docs_mcp":         true,
		"docs_mcp_url":     "https://mcp.tigerdata.com/docs?disabled_skills=ghost-database",
		"service_id":       "",
		"color":            true,
		"output":           "json",
		"analytics":        false,
		"password_storage": "keyring",
		"read_only":        false,
		"debug":            false,
		"config_dir":       tmpDir,
		"releases_url":     "https://cli.tigerdata.com",
		"version_check":    true,
		"mcp_max_rows":     float64(config.DefaultMCPMaxRows),
	}

	for key, expectedValue := range expectedValues {
		if result[key] != expectedValue {
			t.Errorf("Expected %s '%v', got %v", key, expectedValue, result[key])
		}
	}

	// Ensure no extra keys are present
	if len(result) != len(expectedValues) {
		t.Errorf("Expected %d keys in JSON output, got %d", len(expectedValues), len(result))
	}
}

func TestConfigShow_YAMLOutput(t *testing.T) {
	tmpDir, _ := setupConfigTest(t)

	// Create config file with YAML output format
	configContent := `api_url: https://yaml.api.com/v1
output: yaml
analytics: false
password_storage: keyring
`

	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	output, err := executeConfigCommand(t.Context(), "config", "show")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Parse YAML output
	var result map[string]any
	if err := yaml.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Failed to parse YAML output: %v", err)
	}

	// Verify ALL YAML keys and their expected values
	expectedValues := map[string]any{
		"api_url":          "https://yaml.api.com/v1",
		"console_url":      "https://console.cloud.tigerdata.com",
		"gateway_url":      "https://console.cloud.tigerdata.com/api",
		"docs_mcp":         true,
		"docs_mcp_url":     "https://mcp.tigerdata.com/docs?disabled_skills=ghost-database",
		"service_id":       "",
		"color":            true,
		"output":           "yaml",
		"analytics":        false,
		"password_storage": "keyring",
		"read_only":        false,
		"debug":            false,
		"config_dir":       tmpDir,
		"releases_url":     "https://cli.tigerdata.com",
		"version_check":    true,
		"mcp_max_rows":     config.DefaultMCPMaxRows,
	}

	for key, expectedValue := range expectedValues {
		if result[key] != expectedValue {
			t.Errorf("Expected %s '%v', got %v", key, expectedValue, result[key])
		}
	}

	// Ensure no extra keys are present
	if len(result) != len(expectedValues) {
		t.Errorf("Expected %d keys in YAML output, got %d", len(expectedValues), len(result))
	}
}

func TestConfigShow_OutputValueUnaffectedByCliArg(t *testing.T) {
	tmpDir, _ := setupConfigTest(t)

	// Create config file with table as default output
	configContent := `api_url: https://test.api.com/v1
project_id: test-project
output: table
analytics: true
`
	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test that -o json flag overrides config file setting for output format, but not the config value itself
	output, err := executeConfigCommand(t.Context(), "config", "show", "-o", "json")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Should be valid JSON, not table format
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Expected JSON output but got: %v\nOutput was: %s", err, output)
	}

	if result["output"] != "table" {
		t.Errorf("Expected output 'table' in JSON output, got %v", result["output"])
	}
}

func TestConfigShow_OutputValueUnaffectedByEnvVar(t *testing.T) {
	tmpDir, _ := setupConfigTest(t)

	// Create config file with table as default output
	configContent := `api_url: https://test.api.com/v1
output: table
analytics: true
`
	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test that env overrides config file setting for output format, but not the config value itself
	os.Setenv("TIGER_OUTPUT", "json")
	defer func() {
		os.Unsetenv("TIGER_OUTPUT")
	}()

	output, err := executeConfigCommand(t.Context(), "config", "show")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Should be valid JSON, not table format
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Expected JSON output but got: %v\nOutput was: %s", err, output)
	}

	if result["output"] != "table" {
		t.Errorf("Expected output 'table' in JSON output, got %v", result["output"])
	}
}

func TestConfigShow_ConfigDirFlag(t *testing.T) {
	setupConfigTest(t)

	// Create a different temporary directory for the --config-dir flag, which
	// should override the value provided via the TIGER_CONFIG_DIR env var in
	// setupConfigTest
	tmpDir, err := os.MkdirTemp("", "tiger-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	// Create a config file with test data in the specified directory
	configContent := `api_url: https://flag-test.api.com/v1
output: json
analytics: false
`
	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Execute config show with --config-dir flag
	output, err := executeConfigCommand(t.Context(), "--config-dir", tmpDir, "config", "show")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	// Parse JSON output and verify values
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if result["api_url"] != "https://flag-test.api.com/v1" {
		t.Errorf("Expected api_url 'https://flag-test.api.com/v1', got %v", result["api_url"])
	}
	if result["config_dir"] != tmpDir {
		t.Errorf("Expected config_dir '%s', got %v", tmpDir, result["config_dir"])
	}
}
