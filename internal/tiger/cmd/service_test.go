package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oapi-codegen/runtime/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/tigerdata/tiger-cli/internal/tiger/api"
	"github.com/tigerdata/tiger-cli/internal/tiger/config"
)

func setupServiceTest(t *testing.T) string {
	t.Helper()
	
	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-service-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	
	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	
	// Reset global config and viper to ensure test isolation
	config.ResetGlobalConfig()
	viper.Reset()
	
	t.Cleanup(func() {
		// Reset global config and viper first
		config.ResetGlobalConfig()
		viper.Reset()
		// Clean up environment variable BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})
	
	return tmpDir
}

func executeServiceCommand(args ...string) (string, error) {
	// Create a test root command with service subcommand
	testRoot := &cobra.Command{
		Use: "tiger",
		PersistentPreRunE: rootCmd.PersistentPreRunE,
	}
	
	// Add persistent flags and bind them
	addPersistentFlags(testRoot)
	bindFlags(testRoot)
	
	// Add the service command to our test root
	testRoot.AddCommand(serviceCmd)
	
	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)
	
	err := testRoot.Execute()
	return buf.String(), err
}

func TestServiceList_NoProjectID(t *testing.T) {
	setupServiceTest(t)
	
	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()
	
	// Execute service list command without project ID
	_, err := executeServiceCommand("service", "list")
	if err == nil {
		t.Fatal("Expected error when no project ID is configured")
	}
	
	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("Expected error about missing project ID, got: %v", err)
	}
}

func TestServiceList_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)
	
	// Set up config with project ID
	cfg := &config.Config{
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}
	
	// Mock authentication failure
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "", fmt.Errorf("not logged in")
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()
	
	// Execute service list command
	_, err = executeServiceCommand("service", "list")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}
	
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
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
	err := outputServices(cmd, services, "json")
	if err != nil {
		t.Fatalf("Failed to output JSON: %v", err)
	}
	
	// Verify JSON is valid
	var result []api.Service
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Invalid JSON output: %v", err)
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
	err := outputServices(cmd, services, "yaml")
	if err != nil {
		t.Fatalf("Failed to output YAML: %v", err)
	}
	
	// Verify YAML is valid
	var result []api.Service
	if err := yaml.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Invalid YAML output: %v", err)
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
	err := outputServices(cmd, services, "table")
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

func TestFormatHelpers(t *testing.T) {
	// Test formatUUID
	testUUID := types.UUID{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}
	if formatUUID(&testUUID) == "" {
		t.Error("formatUUID should return non-empty string for valid UUID")
	}
	if formatUUID(nil) != "" {
		t.Error("formatUUID should return empty string for nil UUID")
	}
	
	// Test derefString
	testStr := "test"
	if derefString(&testStr) != "test" {
		t.Error("derefString should return string value")
	}
	if derefString(nil) != "" {
		t.Error("derefString should return empty string for nil")
	}
	
	// Test formatDeployStatus
	status := api.DeployStatus("running")
	if formatDeployStatus(&status) != "running" {
		t.Error("formatDeployStatus should return status string")
	}
	if formatDeployStatus(nil) != "" {
		t.Error("formatDeployStatus should return empty string for nil")
	}
	
	// Test formatServiceType
	serviceType := api.ServiceType("POSTGRES")
	if formatServiceType(&serviceType) != "POSTGRES" {
		t.Error("formatServiceType should return service type string")
	}
	if formatServiceType(nil) != "" {
		t.Error("formatServiceType should return empty string for nil")
	}
	
	// Test formatTimePtr
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	if formatTimePtr(&testTime) == "" {
		t.Error("formatTimePtr should return formatted time string")
	}
	if formatTimePtr(nil) != "" {
		t.Error("formatTimePtr should return empty string for nil")
	}
}

// Helper function to create test services
func createTestServices() []api.Service {
	testUUID1 := types.UUID{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}
	testUUID2 := types.UUID{0x98, 0x76, 0x54, 0x32, 0x10, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0xfe, 0xdc, 0xba}
	
	name1 := "test-service-1"
	name2 := "test-service-2"
	region1 := "us-east-1"
	region2 := "eu-west-1"
	status1 := api.DeployStatus("running")
	status2 := api.DeployStatus("stopped")
	serviceType1 := api.ServiceType("POSTGRES")
	serviceType2 := api.ServiceType("TIMESCALEDB")
	created1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	created2 := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
	
	return []api.Service{
		{
			ServiceId:   &testUUID1,
			Name:        &name1,
			RegionCode:  &region1,
			Status:      &status1,
			ServiceType: &serviceType1,
			Created:     &created1,
		},
		{
			ServiceId:   &testUUID2,
			Name:        &name2,
			RegionCode:  &region2,
			Status:      &status2,
			ServiceType: &serviceType2,
			Created:     &created2,
		},
	}
}