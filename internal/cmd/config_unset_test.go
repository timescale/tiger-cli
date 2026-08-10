package cmd

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestConfigUnset_ValidKeys(t *testing.T) {
	_, _ = setupConfigTest(t)

	// First set some values
	cfg, err := config.Load(nil)
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
			output, err := executeConfigCommand(t.Context(), "config", "unset", tt.key)
			if err != nil {
				t.Fatalf("Command failed: %v", err)
			}

			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("Expected output to contain '%s', got '%s'", tt.expectedOutput, strings.TrimSpace(output))
			}

			// Verify the value was actually unset
			cfg, err := config.Load(nil)
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

	_, err := executeConfigCommand(t.Context(), "config", "unset", "unknown_key")
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
	_, err := executeConfigCommand(t.Context(), "config", "unset")
	if err == nil {
		t.Error("Expected command to fail with no arguments")
	}

	// Test with too many arguments
	_, err = executeConfigCommand(t.Context(), "config", "unset", "key", "extra")
	if err == nil {
		t.Error("Expected command to fail with too many arguments")
	}
}
