package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestServiceList_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with API URL
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	mockNotLoggedIn(t)

	// Execute service list command
	_, err, _ = executeServiceCommand(t.Context(), "service", "list")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestServiceList_OutputFlagAffectsCommandOnly(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with output format explicitly set to "table"
	cfg, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":       "http://localhost:9999",
		"output":        "table",
		"version_check": false,
	})
	if err != nil {
		t.Fatalf("Failed to setup test config: %v", err)
	}
	configFile := cfg.GetConfigFile()

	// Mock authentication
	mockTestPAT(t)

	// Store original config file content
	originalConfigBytes, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read original config file: %v", err)
	}

	// Execute service list with -o json flag (will fail due to no mock API, but that's OK)
	_, _, _ = executeServiceCommand(t.Context(), "service", "list", "-o", "json")

	// Read the config file again
	newConfigBytes, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file after command: %v", err)
	}

	// Verify config file was NOT modified
	if string(originalConfigBytes) != string(newConfigBytes) {
		t.Errorf("Config file should not be modified by using -o flag.\nOriginal:\n%s\nNew:\n%s",
			string(originalConfigBytes), string(newConfigBytes))
	}
}
