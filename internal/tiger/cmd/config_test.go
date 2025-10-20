package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func setupConfigTest(t *testing.T) (string, func()) {
	t.Helper()

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set environment variable to use test directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	config.UseTestConfig(tmpDir, map[string]any{})

	// Clean up function
	cleanup := func() {
		os.RemoveAll(tmpDir)
		os.Unsetenv("TIGER_CONFIG_DIR")

		// Reset global config in the config package
		// This is important for test isolation
		// We need to clear the singleton
		config.ResetGlobalConfig()
	}

	t.Cleanup(cleanup)

	return tmpDir, cleanup
}

func executeConfigCommand(args ...string) (string, error) {
	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot := buildRootCmd()

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err := testRoot.Execute()
	return buf.String(), err
}

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

	output, err := executeConfigCommand("config", "show")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}
	lines := strings.Split(output, "\n")

	// Check table output contains all expected key:value lines
	expectedLines := map[string]string{
		"api_url":          "https://test.api.com/v1",
		"console_url":      "https://console.cloud.timescale.com",
		"gateway_url":      "https://console.cloud.timescale.com/api",
		"docs_mcp":         "true",
		"docs_mcp_url":     "https://mcp.tigerdata.com/docs",
		"service_id":       "test-service",
		"output":           "table",
		"analytics":        "false",
		"password_storage": "pgpass",
		"debug":            "false",
		"config_dir":       tmpDir,
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

	now := time.Now()

	// Create config file with JSON output format
	configContent := `api_url: https://json.api.com/v1
output: json
analytics: true
password_storage: none
version_check_interval: 1h
version_check_last_time: ` + now.Format(time.RFC3339) + "\n"

	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	output, err := executeConfigCommand("config", "show")
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
		"api_url":                 "https://json.api.com/v1",
		"console_url":             "https://console.cloud.timescale.com",
		"gateway_url":             "https://console.cloud.timescale.com/api",
		"docs_mcp":                true,
		"docs_mcp_url":            "https://mcp.tigerdata.com/docs",
		"service_id":              "",
		"color":                   true,
		"output":                  "json",
		"analytics":               true,
		"password_storage":        "none",
		"debug":                   false,
		"config_dir":              tmpDir,
		"releases_url":            "https://cli.tigerdata.com",
		"version_check_interval":  float64(3600000000000), // JSON unmarshals time.Duration as nanoseconds (1 hour = 3600000000000ns)
		"version_check_last_time": now.Format(time.RFC3339),
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

	now := time.Now()

	// Create config file with YAML output format
	configContent := `api_url: https://yaml.api.com/v1
output: yaml
analytics: false
password_storage: keyring
version_check_last_time: ` + now.Format(time.RFC3339) + "\n"

	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	output, err := executeConfigCommand("config", "show")
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
		"api_url":                 "https://yaml.api.com/v1",
		"console_url":             "https://console.cloud.timescale.com",
		"gateway_url":             "https://console.cloud.timescale.com/api",
		"docs_mcp":                true,
		"docs_mcp_url":            "https://mcp.tigerdata.com/docs",
		"service_id":              "",
		"color":                   true,
		"output":                  "yaml",
		"analytics":               false,
		"password_storage":        "keyring",
		"debug":                   false,
		"config_dir":              tmpDir,
		"releases_url":            "https://cli.tigerdata.com",
		"version_check_interval":  "24h0m0s", // YAML serializes time.Duration as string
		"version_check_last_time": now,
	}

	for key, expectedValue := range expectedValues {
		switch expectedValue.(type) {
		case time.Time:
			// YAML unmarshals time.Time as time.Time type, so we need to compare differently
			if expectedValue.(time.Time).Format(time.RFC3339) != result[key].(time.Time).Format(time.RFC3339) {
				t.Errorf("foo Expected %s '%v', got %v", key, expectedValue, result[key])
			}
		default:
			// Other types can be compared directly
			if result[key] != expectedValue {
				t.Errorf("Expected %s '%v', got %v", key, expectedValue, result[key])
			}
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
	output, err := executeConfigCommand("config", "show", "-o", "json")
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

	output, err := executeConfigCommand("config", "show")
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
	output, err := executeConfigCommand("--config-dir", tmpDir, "config", "show")
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

func TestConfigSet_ValidValues(t *testing.T) {
	_, _ = setupConfigTest(t)

	tests := []struct {
		key            string
		value          string
		expectedOutput string
	}{
		{"api_url", "https://new.api.com/v1", "Set api_url = https://new.api.com/v1"},
		{"service_id", "new-service", "Set service_id = new-service"},
		{"output", "json", "Set output = json"},
		{"analytics", "false", "Set analytics = false"},
		{"password_storage", "pgpass", "Set password_storage = pgpass"},
		{"password_storage", "none", "Set password_storage = none"},
		{"password_storage", "keyring", "Set password_storage = keyring"},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			output, err := executeConfigCommand("config", "set", tt.key, tt.value)
			if err != nil {
				t.Fatalf("Command failed: %v", err)
			}

			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("Expected output to contain '%s', got '%s'", tt.expectedOutput, strings.TrimSpace(output))
			}

			// Verify the value was actually set
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Check the value was set correctly
			switch tt.key {
			case "api_url":
				if cfg.APIURL != tt.value {
					t.Errorf("Expected APIURL %s, got %s", tt.value, cfg.APIURL)
				}
			case "service_id":
				if cfg.ServiceID != tt.value {
					t.Errorf("Expected ServiceID %s, got %s", tt.value, cfg.ServiceID)
				}
			case "output":
				if cfg.Output != tt.value {
					t.Errorf("Expected Output %s, got %s", tt.value, cfg.Output)
				}
			case "analytics":
				expected := tt.value == "true"
				if cfg.Analytics != expected {
					t.Errorf("Expected Analytics %t, got %t", expected, cfg.Analytics)
				}
			case "password_storage":
				if cfg.PasswordStorage != tt.value {
					t.Errorf("Expected PasswordStorage %s, got %s", tt.value, cfg.PasswordStorage)
				}
			default:
				t.Fatalf("Unhandled test case for key: %s", tt.key)
			}
		})
	}
}

func TestConfigSet_InvalidValues(t *testing.T) {
	_, _ = setupConfigTest(t)

	tests := []struct {
		key   string
		value string
		error string
	}{
		{"output", "invalid", "invalid output format"},
		{"analytics", "maybe", "invalid analytics value"},
		{"password_storage", "invalid", "invalid password_storage value"},
		{"password_storage", "secure", "invalid password_storage value"},
		{"unknown", "value", "unknown configuration key"},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			_, err := executeConfigCommand("config", "set", tt.key, tt.value)
			if err == nil {
				t.Error("Expected command to fail, but it succeeded")
			}

			if !strings.Contains(err.Error(), tt.error) {
				t.Errorf("Expected error to contain '%s', got '%s'", tt.error, err.Error())
			}
		})
	}
}

func TestConfigSet_WrongArgs(t *testing.T) {
	_, _ = setupConfigTest(t)

	// Test with no arguments
	_, err := executeConfigCommand("config", "set")
	if err == nil {
		t.Error("Expected command to fail with no arguments")
	}

	// Test with one argument
	_, err = executeConfigCommand("config", "set", "key")
	if err == nil {
		t.Error("Expected command to fail with only one argument")
	}

	// Test with too many arguments
	_, err = executeConfigCommand("config", "set", "key", "value", "extra")
	if err == nil {
		t.Error("Expected command to fail with too many arguments")
	}
}

func TestConfigSet_ConfigDirFlag(t *testing.T) {
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

	// Execute config set with --config-dir flag
	if _, err := executeConfigCommand("--config-dir", tmpDir, "config", "set", "service_id", "flag-set-service"); err != nil {
		t.Fatalf("Config set command failed: %v", err)
	}

	// Verify the config file was created in the specified directory
	configFile := config.GetConfigFile(tmpDir)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Fatalf("Config file should exist at %s", configFile)
	}

	// Read the config file and verify the value was saved
	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	if !strings.Contains(string(content), "service_id: flag-set-service") {
		t.Errorf("Config file should contain 'service_id: flag-set-service', got: %s", string(content))
	}
}

func TestConfigUnset_ValidKeys(t *testing.T) {
	_, _ = setupConfigTest(t)

	// First set some values
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	cfg.Set("service_id", "test-service")
	cfg.Set("output", "json")
	cfg.Set("password_storage", "pgpass")

	tests := []struct {
		key            string
		expectedOutput string
	}{
		{"service_id", "Unset service_id"},
		{"output", "Unset output"},
		{"password_storage", "Unset password_storage"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			output, err := executeConfigCommand("config", "unset", tt.key)
			if err != nil {
				t.Fatalf("Command failed: %v", err)
			}

			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("Expected output to contain '%s', got '%s'", tt.expectedOutput, strings.TrimSpace(output))
			}

			// Verify the value was actually unset
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Check the value was unset correctly
			switch tt.key {
			case "service_id":
				if cfg.ServiceID != "" {
					t.Errorf("Expected empty ServiceID, got %s", cfg.ServiceID)
				}
			case "output":
				if cfg.Output != config.DefaultOutput {
					t.Errorf("Expected default Output %s, got %s", config.DefaultOutput, cfg.Output)
				}
			case "password_storage":
				if cfg.PasswordStorage != config.DefaultPasswordStorage {
					t.Errorf("Expected default PasswordStorage %s, got %s", config.DefaultPasswordStorage, cfg.PasswordStorage)
				}
			default:
				t.Fatalf("Unhandled test case for key: %s", tt.key)
			}
		})
	}
}

func TestConfigUnset_InvalidKey(t *testing.T) {
	_, _ = setupConfigTest(t)

	_, err := executeConfigCommand("config", "unset", "unknown_key")
	if err == nil {
		t.Error("Expected command to fail with unknown key")
	}

	if !strings.Contains(err.Error(), "unknown configuration key") {
		t.Errorf("Expected error about unknown key, got: %s", err.Error())
	}
}

func TestConfigUnset_WrongArgs(t *testing.T) {
	_, _ = setupConfigTest(t)

	// Test with no arguments
	_, err := executeConfigCommand("config", "unset")
	if err == nil {
		t.Error("Expected command to fail with no arguments")
	}

	// Test with too many arguments
	_, err = executeConfigCommand("config", "unset", "key", "extra")
	if err == nil {
		t.Error("Expected command to fail with too many arguments")
	}
}

func TestConfigReset(t *testing.T) {
	_, _ = setupConfigTest(t)

	// First set some custom values
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	cfg.Set("service_id", "custom-service")
	cfg.Set("output", "json")
	cfg.Set("analytics", "false")

	// Execute reset command
	output, err := executeConfigCommand("config", "reset")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	if !strings.Contains(output, "Configuration reset to defaults") {
		t.Errorf("Expected output to contain reset message, got '%s'", strings.TrimSpace(output))
	}

	cfg, err = config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify all values were reset to defaults
	if cfg.APIURL != config.DefaultAPIURL {
		t.Errorf("Expected default APIURL %s, got %s", config.DefaultAPIURL, cfg.APIURL)
	}
	if cfg.ServiceID != "" {
		t.Errorf("Expected empty ServiceID, got %s", cfg.ServiceID)
	}
	if cfg.Output != config.DefaultOutput {
		t.Errorf("Expected default Output %s, got %s", config.DefaultOutput, cfg.Output)
	}
	if cfg.Analytics != config.DefaultAnalytics {
		t.Errorf("Expected default Analytics %t, got %t", config.DefaultAnalytics, cfg.Analytics)
	}
}

func TestConfigCommands_Integration(t *testing.T) {
	_, _ = setupConfigTest(t)

	// Test full workflow: set -> show -> unset -> reset

	// 1. Set some values
	_, err := executeConfigCommand("config", "set", "service_id", "integration-test")
	if err != nil {
		t.Fatalf("Failed to set service_id: %v", err)
	}

	_, err = executeConfigCommand("config", "set", "output", "json")
	if err != nil {
		t.Fatalf("Failed to set output: %v", err)
	}

	// 2. Show config in JSON format (should use the output format we just set)
	showOutput, err := executeConfigCommand("config", "show")
	if err != nil {
		t.Fatalf("Failed to show config: %v", err)
	}

	// Should be JSON output
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(showOutput), &result); err != nil {
		t.Fatalf("Expected JSON output, got: %s", showOutput)
	}

	if result["service_id"] != "integration-test" {
		t.Errorf("Expected service_id 'integration-test', got %v", result["service_id"])
	}

	// 3. Unset service_id
	_, err = executeConfigCommand("config", "unset", "service_id")
	if err != nil {
		t.Fatalf("Failed to unset service_id: %v", err)
	}

	// 4. Verify service_id was unset
	showOutput, err = executeConfigCommand("config", "show")
	if err != nil {
		t.Fatalf("Failed to show config after unset: %v", err)
	}

	result = make(map[string]any)
	json.Unmarshal([]byte(showOutput), &result)
	if result["service_id"] != "" {
		t.Errorf("Expected empty service_id after unset, got %v", result["service_id"])
	}

	// 5. Reset all config
	_, err = executeConfigCommand("config", "reset")
	if err != nil {
		t.Fatalf("Failed to reset config: %v", err)
	}

	// 6. Verify everything is back to defaults
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config after reset: %v", err)
	}

	if cfg.Output != config.DefaultOutput {
		t.Errorf("Expected output reset to default %s, got %s", config.DefaultOutput, cfg.Output)
	}
}

func TestConfigSet_OutputDoesPersist(t *testing.T) {
	tmpDir, _ := setupConfigTest(t)

	// Start with default config (no output setting in file)
	configFile := config.GetConfigFile(tmpDir)

	// Execute config set to explicitly set output to json
	_, err := executeConfigCommand("config", "set", "output", "json")
	if err != nil {
		t.Fatalf("Failed to set output to json: %v", err)
	}

	// Read the config file directly
	configBytes, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// Parse the YAML to check
	var configMap map[string]interface{}
	if err := yaml.Unmarshal(configBytes, &configMap); err != nil {
		t.Fatalf("Failed to parse config YAML: %v", err)
	}

	if outputVal, exists := configMap["output"]; !exists || outputVal != "json" {
		t.Errorf("Expected output in config file to be 'json', got: %v (exists: %v)", outputVal, exists)
	}

	// Also verify by loading config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Output != "json" {
		t.Errorf("Expected loaded config output to be 'json', got: %s", cfg.Output)
	}

	// Now test that setting it to a different value updates the file
	_, err = executeConfigCommand("config", "set", "output", "yaml")
	if err != nil {
		t.Fatalf("Failed to set output to yaml: %v", err)
	}

	// Read the config file again
	configBytes, err = os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file after second set: %v", err)
	}

	configContent := string(configBytes)

	// Verify that output was updated in the config file
	if !strings.Contains(configContent, "output: yaml") {
		t.Errorf("Config file should contain 'output: yaml' after update. Config content:\n%s", configContent)
	}

	// Should NOT contain the old value
	if strings.Contains(configContent, "output: json") {
		t.Errorf("Config file should NOT contain old 'output: json' value. Config content:\n%s", configContent)
	}
}
