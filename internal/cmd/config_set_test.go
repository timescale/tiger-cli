package cmd

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/config"
)

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
			output, err := executeConfigCommand(t.Context(), "config", "set", tt.key, tt.value)
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
			_, err := executeConfigCommand(t.Context(), "config", "set", tt.key, tt.value)
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
	_, err := executeConfigCommand(t.Context(), "config", "set")
	if err == nil {
		t.Error("Expected command to fail with no arguments")
	}

	// Test with one argument
	_, err = executeConfigCommand(t.Context(), "config", "set", "key")
	if err == nil {
		t.Error("Expected command to fail with only one argument")
	}

	// Test with too many arguments
	_, err = executeConfigCommand(t.Context(), "config", "set", "key", "value", "extra")
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
	if _, err := executeConfigCommand(t.Context(), "--config-dir", tmpDir, "config", "set", "service_id", "flag-set-service"); err != nil {
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

func TestConfigSet_OutputDoesPersist(t *testing.T) {
	tmpDir, _ := setupConfigTest(t)

	// Start with default config (no output setting in file)
	configFile := config.GetConfigFile(tmpDir)

	// Execute config set to explicitly set output to json
	_, err := executeConfigCommand(t.Context(), "config", "set", "output", "json")
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
	_, err = executeConfigCommand(t.Context(), "config", "set", "output", "yaml")
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
