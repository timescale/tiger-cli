package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/api"
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

func TestOutputServices_JSON(t *testing.T) {
	setupServiceTest(t)

	// Create test services
	services := createTestServices()

	// Create test command
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	// Test JSON output
	err := outputServices(cmd, testConfig(t), services, "json")
	if err != nil {
		t.Fatalf("Failed to output JSON: %v", err)
	}

	// Verify JSON is valid
	var result []api.Service
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON Output: %v", err)
	}

	if len(result) != len(services) {
		t.Errorf("Expected %d services in JSON, got %d", len(services), len(result))
	}
}

func TestOutputServices_YAML(t *testing.T) {
	setupServiceTest(t)

	// Create test services
	services := createTestServices()

	// Create test command
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	// Test YAML output
	err := outputServices(cmd, testConfig(t), services, "yaml")
	if err != nil {
		t.Fatalf("Failed to output YAML: %v", err)
	}

	// Verify YAML is valid
	var result []api.Service
	if err := yaml.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Invalid YAML Output: %v", err)
	}

	if len(result) != len(services) {
		t.Errorf("Expected %d services in YAML, got %d", len(services), len(result))
	}
}

func TestOutputServices_Table(t *testing.T) {
	setupServiceTest(t)

	// Create test services
	services := createTestServices()

	// Create test command
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	// Test table output
	err := outputServices(cmd, testConfig(t), services, "table")
	if err != nil {
		t.Fatalf("Failed to output table: %v", err)
	}

	output := buf.String()

	// Verify table contains headers
	if !strings.Contains(output, "SERVICE ID") {
		t.Error("Table output should contain SERVICE ID header")
	}
	if !strings.Contains(output, "NAME") {
		t.Error("Table output should contain NAME header")
	}
	if !strings.Contains(output, "STATUS") {
		t.Error("Table output should contain STATUS header")
	}

	// Verify table contains service data
	if !strings.Contains(output, "test-service-1") {
		t.Error("Table output should contain test service name")
	}
}

func TestSanitizeServicesForOutput(t *testing.T) {
	// Create services with sensitive data
	serviceID1 := "svc-12345"
	serviceName1 := "test-service-1"
	initialPassword1 := "secret-password-123"

	serviceID2 := "svc-67890"
	serviceName2 := "test-service-2"
	initialPassword2 := "another-secret-456"

	services := []api.Service{
		{
			ServiceId:       &serviceID1,
			Name:            &serviceName1,
			InitialPassword: &initialPassword1,
		},
		{
			ServiceId:       &serviceID2,
			Name:            &serviceName2,
			InitialPassword: &initialPassword2,
		},
	}

	// Sanitize the services
	sanitized := prepareServicesForOutput(nil, testConfig(t), services)

	// Verify that we have the same number of services
	if len(sanitized) != len(services) {
		t.Errorf("Expected %d sanitized services, got %d", len(services), len(sanitized))
	}

	// Verify that sensitive fields are removed from all services
	for i, service := range sanitized {
		if service.InitialPassword != nil {
			t.Errorf("Expected InitialPassword to be nil in sanitized service %d", i)
		}
		if service.Password != "" {
			t.Errorf("Expected Password to be empty in sanitized service %d", i)
		}

		// Verify that other fields are preserved
		if service.ServiceId == nil {
			t.Errorf("Expected ServiceId to be preserved in sanitized service %d", i)
		}
		if service.Name == nil {
			t.Errorf("Expected Name to be preserved in sanitized service %d", i)
		}
	}
}

func TestFormatTimePtr(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	if formatTimePtr(&testTime) == "" {
		t.Error("formatTimePtr should return formatted time string")
	}
	if formatTimePtr(nil) != "" {
		t.Error("formatTimePtr should return empty string for nil")
	}
}
