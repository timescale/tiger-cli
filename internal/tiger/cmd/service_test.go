package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
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

	// Disable analytics for service tests to avoid tracking test events
	os.Setenv("TIGER_ANALYTICS", "false")

	// Reset global config and viper to ensure test isolation
	config.ResetGlobalConfig()

	// Re-establish viper environment configuration after reset
	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

	t.Cleanup(func() {
		// Reset global config and viper first
		config.ResetGlobalConfig()
		// Clean up environment variables BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_ANALYTICS")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func executeServiceCommand(ctx context.Context, args ...string) (string, error, *cobra.Command) {
	// No need to reset any flags - we build fresh commands with local variables

	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot, err := buildRootCmd(ctx)
	if err != nil {
		return "", err, nil
	}

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err = testRoot.Execute()
	return buf.String(), err, testRoot
}

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
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service list command
	_, err, _ = executeServiceCommand(t.Context(), "service", "list")
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

func TestServiceFork_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service fork command with required timing flag
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "--now")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestServiceFork_NoSourceService(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config without service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service fork command without providing service ID but with timing flag
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "--now")
	if err == nil {
		t.Fatal("Expected error when no service ID provided")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected service ID required error, got: %v", err)
	}
}

func TestServiceFork_NoTimingFlag(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service fork command without any timing flag
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "source-service-123")
	if err == nil {
		t.Fatal("Expected error when no timing flag provided")
	}

	if !strings.Contains(err.Error(), "must specify --now, --last-snapshot or --to-timestamp") {
		t.Errorf("Expected timing flag required error, got: %v", err)
	}
}

func TestServiceFork_MultipleTiming(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service fork command with multiple timing flags
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "source-service-123", "--now", "--last-snapshot")
	if err == nil {
		t.Fatal("Expected error when multiple timing flags provided")
	}

	if !strings.Contains(err.Error(), "can only specify one of --now, --last-snapshot or --to-timestamp") {
		t.Errorf("Expected multiple timing flags error, got: %v", err)
	}
}

func TestServiceFork_InvalidTimestamp(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service fork command with invalid timestamp
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "source-service-123", "--to-timestamp", "invalid-timestamp")
	if err == nil {
		t.Fatal("Expected error when invalid timestamp provided")
	}

	if !strings.Contains(err.Error(), "invalid time format") {
		t.Errorf("Expected invalid timestamp error, got: %v", err)
	}
}

func TestServiceFork_CPUMemoryValidation(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "source-service-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication success
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Test with invalid CPU/memory combination (this would fail at API call stage)
	// Since we don't want to make real API calls, we expect the command to fail during validation
	_, err, _ = executeServiceCommand(t.Context(), "service", "fork", "source-service-123", "--now", "--cpu", "999", "--memory", "1")

	// This test is mainly to ensure the flags are parsed correctly
	// The actual validation happens later in the process when we have source service details
	// So we expect either a validation error or an API connection error
	if err == nil {
		t.Fatal("Expected some error due to invalid CPU/memory or API connection")
	}

	// We're mainly testing that the flags are accepted and parsed
	// The detailed validation logic is tested in integration tests
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

func TestServiceCreate_ValidationErrors(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with a mock API URL to prevent network calls
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "http://localhost:9999", // Use a local URL that will fail fast
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Test with no name (should auto-generate) - this should now work without error
	// Just test that it doesn't fail due to missing name
	_, err, _ = executeServiceCommand(t.Context(), "service", "create", "--addons", "none", "--region", "us-east-1")
	// This should fail due to network/API call, not due to missing name
	if err != nil && (strings.Contains(err.Error(), "name") && strings.Contains(err.Error(), "required")) {
		t.Errorf("Should not fail due to missing name anymore (should auto-generate), got: %v", err)
	}

	// Test invalid addon - this should fail validation before making API call
	_, err, _ = executeServiceCommand(t.Context(), "service", "create", "--addons", "invalid-addon", "--region", "us-east-1", "--cpu", "1000", "--memory", "4", "--replicas", "1")
	if err == nil {
		t.Fatal("Expected error when addon is invalid")
	}
	if !strings.Contains(err.Error(), "invalid add-on") {
		t.Errorf("Expected error about invalid add-on, got: %v", err)
	}
}

func TestServiceCreate_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with API URL
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service create command with valid parameters (name will be auto-generated)
	_, err, _ = executeServiceCommand(t.Context(), "service", "create", "--addons", "none", "--region", "us-east-1", "--cpu", "1000", "--memory", "4", "--replicas", "1")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

// Helper function to create test services
func createTestServices() []api.Service {
	testServiceID1 := "12345678-9abc-def0-1234-56789abcdef0"
	testServiceID2 := "98765432-10fe-dcba-9876-543210fedcba"

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
			ServiceId:   &testServiceID1,
			Name:        &name1,
			RegionCode:  &region1,
			Status:      &status1,
			ServiceType: &serviceType1,
			Created:     &created1,
		},
		{
			ServiceId:   &testServiceID2,
			Name:        &name2,
			RegionCode:  &region2,
			Status:      &status2,
			ServiceType: &serviceType2,
			Created:     &created2,
		},
	}
}

func TestAutoGeneratedServiceName(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with a mock API URL to prevent network calls
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "http://localhost:9999", // Use a local URL that will fail fast
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Test that service name is auto-generated when not provided
	// We expect this to fail at the API call stage, not at validation
	var rootCmd *cobra.Command
	_, err, rootCmd = executeServiceCommand(t.Context(), "service", "create", "--addons", "none", "--region", "us-east-1")

	// The command should not fail due to missing service name
	if err != nil && strings.Contains(err.Error(), "service name is required") {
		t.Error("Service name should be auto-generated, not required")
	}

	// Navigate to the create command that was actually used
	if rootCmd == nil {
		t.Fatal("rootCmd should not be nil")
	}

	// Find service command
	serviceCmd, _, err := rootCmd.Find([]string{"service"})
	if err != nil {
		t.Fatalf("Failed to find service command: %v", err)
	}

	// Find create subcommand
	createCmd, _, err := serviceCmd.Find([]string{"create"})
	if err != nil {
		t.Fatalf("Failed to find create command: %v", err)
	}

	nameFlag := createCmd.Flags().Lookup("name")
	if nameFlag == nil {
		t.Fatal("name flag should exist on create command")
	}

	serviceName := nameFlag.Value.String()
	if serviceName == "" {
		t.Error("Service name should have been auto-generated")
	}

	// Check pattern (should start with "db-" followed by numbers)
	if !strings.HasPrefix(serviceName, "db-") {
		t.Errorf("Auto-generated name should start with 'db-', got: %s", serviceName)
	}
}

func TestServiceGet_NoServiceID(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID but no default service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"project_id": "test-project-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service get command without service ID
	_, err, _ = executeServiceCommand(t.Context(), "service", "get")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestServiceGet_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service get command
	_, err, _ = executeServiceCommand(t.Context(), "service", "get")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestOutputService_JSON(t *testing.T) {
	// Create a test service object
	serviceID := "svc-12345"
	serviceName := "test-service"
	serviceType := api.TIMESCALEDB
	regionCode := "us-east-1"
	status := api.READY
	created := time.Now()
	initialPassword := "secret-password-123"

	service := api.Service{
		ServiceId:       &serviceID,
		Name:            &serviceName,
		ServiceType:     &serviceType,
		RegionCode:      &regionCode,
		Status:          &status,
		Created:         &created,
		InitialPassword: &initialPassword,
	}

	// Create a test command
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	// Test JSON output
	err := outputService(cmd, service, "json", false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify JSON output
	output := buf.String()
	if !strings.Contains(output, `"service_id": "svc-12345"`) {
		t.Errorf("Expected JSON to contain service ID, got: %s", output)
	}

	// Verify that initialpassword is NOT in the output
	if strings.Contains(output, "secret-password-123") || strings.Contains(output, "initialpassword") || strings.Contains(output, "initial_password") || strings.Contains(output, "password") {
		t.Errorf("JSON output should not contain initialpassword field, got: %s", output)
	}

	// Verify it's valid JSON
	var jsonResult api.Service
	err = json.Unmarshal([]byte(output), &jsonResult)
	if err != nil {
		t.Errorf("Output should be valid JSON: %v", err)
	}

	// Verify that the unmarshaled result has no initial password
	// Since we're now using maps for sanitized output, we need to parse it differently
	var jsonMap map[string]interface{}
	err2 := json.Unmarshal([]byte(output), &jsonMap)
	if err2 != nil {
		t.Errorf("Output should be valid JSON map: %v", err2)
	}

	// Check that initialpassword fields are not present in the map
	if _, exists := jsonMap["initial_password"]; exists {
		t.Error("JSON should not contain initial_password field")
	}
	if _, exists := jsonMap["initialpassword"]; exists {
		t.Error("JSON should not contain initialpassword field")
	}
	if _, exists := jsonMap["password"]; exists {
		t.Error("JSON should not contain password field")
	}
}

func TestOutputService_YAML(t *testing.T) {
	// Create a test service object
	serviceID := "svc-12345"
	serviceName := "test-service"
	serviceType := api.TIMESCALEDB
	regionCode := "us-east-1"
	status := api.READY
	created := time.Now()
	initialPassword := "secret-password-123"

	service := api.Service{
		ServiceId:       &serviceID,
		Name:            &serviceName,
		ServiceType:     &serviceType,
		RegionCode:      &regionCode,
		Status:          &status,
		Created:         &created,
		InitialPassword: &initialPassword,
	}

	// Create a test command
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	// Test YAML output
	err := outputService(cmd, service, "yaml", false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify YAML output
	output := buf.String()
	if !strings.Contains(output, "service_id: svc-12345") {
		t.Errorf("Expected YAML to contain service ID, got: %s", output)
	}

	// Verify that initialpassword is NOT in the output
	if strings.Contains(output, "secret-password-123") || strings.Contains(output, "initialpassword") || strings.Contains(output, "password") {
		t.Errorf("YAML output should not contain initialpassword field, got: %s", output)
	}

	// Verify it's valid YAML
	var yamlResult api.Service
	err = yaml.Unmarshal([]byte(output), &yamlResult)
	if err != nil {
		t.Errorf("Output should be valid YAML: %v", err)
	}

	// Verify that the unmarshaled result has no initial password
	// Since we're now using maps for sanitized output, we need to parse it differently
	var yamlMap map[string]interface{}
	err2 := yaml.Unmarshal([]byte(output), &yamlMap)
	if err2 != nil {
		t.Errorf("Output should be valid YAML map: %v", err2)
	}

	// Check that initialpassword fields are not present in the map
	if _, exists := yamlMap["initial_password"]; exists {
		t.Error("YAML should not contain initial_password field")
	}
	if _, exists := yamlMap["initialpassword"]; exists {
		t.Error("YAML should not contain initialpassword field")
	}
	if _, exists := yamlMap["password"]; exists {
		t.Error("YAML should not contain password field")
	}
}

func TestOutputService_Table(t *testing.T) {
	// Create a test service object with resource information
	serviceID := "svc-12345"
	serviceName := "test-service"
	serviceType := api.TIMESCALEDB
	regionCode := "us-east-1"
	status := api.READY
	created := time.Now()
	cpuMillis := 2000
	memoryGbs := 8
	replicaCount := 2
	host := "test.tigerdata.com"
	port := 5432
	initialPassword := "secret-password-123"

	service := api.Service{
		ServiceId:       &serviceID,
		Name:            &serviceName,
		ServiceType:     &serviceType,
		RegionCode:      &regionCode,
		Status:          &status,
		Created:         &created,
		InitialPassword: &initialPassword,
		Resources: &[]struct {
			Id   *string `json:"id,omitempty"`
			Spec *struct {
				CpuMillis  *int    `json:"cpu_millis,omitempty"`
				MemoryGbs  *int    `json:"memory_gbs,omitempty"`
				VolumeType *string `json:"volume_type,omitempty"`
			} `json:"spec,omitempty"`
		}{
			{
				Spec: &struct {
					CpuMillis  *int    `json:"cpu_millis,omitempty"`
					MemoryGbs  *int    `json:"memory_gbs,omitempty"`
					VolumeType *string `json:"volume_type,omitempty"`
				}{
					CpuMillis: &cpuMillis,
					MemoryGbs: &memoryGbs,
				},
			},
		},
		HaReplicas: &api.HAReplica{
			ReplicaCount: &replicaCount,
		},
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Create a test command
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	// Test table output
	err := outputService(cmd, service, "table", false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify table output contains expected information
	output := buf.String()
	expectedContents := []string{
		"svc-12345",
		"test-service",
		"READY",
		"TIMESCALEDB",
		"us-east-1",
		"2 cores (2000m)",
		"8 GB",
		"2",
		"test.tigerdata.com:5432",
	}

	for _, content := range expectedContents {
		if !strings.Contains(output, content) {
			t.Errorf("Expected table to contain %q, got: %s", content, output)
		}
	}

	// Verify that initialpassword is NOT in the table output
	if strings.Contains(output, "secret-password-123") || strings.Contains(output, "password") {
		t.Errorf("Table output should not contain password information, got: %s", output)
	}
}

func TestPrepareServiceForOutput_WithoutPassword(t *testing.T) {
	// Create a service with sensitive data
	serviceID := "svc-12345"
	serviceName := "test-service"
	initialPassword := "secret-password-123"

	service := api.Service{
		ServiceId:       &serviceID,
		Name:            &serviceName,
		InitialPassword: &initialPassword,
	}

	// Mock a cobra command for testing
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Prepare service for output without password
	outputSvc := prepareServiceForOutput(service, false, cmd.ErrOrStderr())

	// Verify that password is removed
	if outputSvc.InitialPassword != nil {
		t.Error("Expected InitialPassword to be nil when withPassword=false")
	}
	if outputSvc.Password != "" {
		t.Error("Expected Password to be empty when withPassword=false")
	}

	// Verify that other fields are preserved
	if outputSvc.ServiceId == nil || *outputSvc.ServiceId != serviceID {
		t.Error("Expected service_id to be preserved")
	}
	if outputSvc.Name == nil || *outputSvc.Name != serviceName {
		t.Error("Expected name to be preserved")
	}
}

func TestPrepareServiceForOutput_WithPassword(t *testing.T) {
	// Create a service with sensitive data
	serviceID := "svc-12345"
	serviceName := "test-service"
	initialPassword := "secret-password-123"
	serviceHost := "test.tigerdata.com"
	servicePort := 5432

	service := api.Service{
		ServiceId:       &serviceID,
		Name:            &serviceName,
		InitialPassword: &initialPassword,
		Endpoint: &api.Endpoint{
			Host: &serviceHost,
			Port: &servicePort,
		},
	}

	// Mock a cobra command for testing
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Prepare service for output with password
	outputSvc := prepareServiceForOutput(service, true, cmd.ErrOrStderr())

	// Verify that password is preserved
	if outputSvc.InitialPassword != nil {
		t.Error("Expected InitialPassword to be nil when withPassword=true")
	}
	if outputSvc.Password == "" || outputSvc.Password != initialPassword {
		t.Error("Expected Password to be preserved when withPassword=true")
	}

	// Verify that other fields are preserved
	if outputSvc.ServiceId == nil || *outputSvc.ServiceId != serviceID {
		t.Error("Expected service_id to be preserved")
	}
	if outputSvc.Name == nil || *outputSvc.Name != serviceName {
		t.Error("Expected name to be preserved")
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
	sanitized := prepareServicesForOutput(services, nil)

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

func TestServiceUpdatePassword_NoServiceID(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID but no default service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"project_id": "test-project-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service update-password command without service ID
	_, err, _ = executeServiceCommand(t.Context(), "service", "update-password", "--new-password", "new-password")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestServiceUpdatePassword_NoPassword(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service update-password command without password
	_, err, _ = executeServiceCommand(t.Context(), "service", "update-password")
	if err == nil {
		t.Fatal("Expected error when no password is provided")
	}

	if !strings.Contains(err.Error(), "password is required") {
		t.Errorf("Expected error about missing password, got: %v", err)
	}
}

func TestServiceUpdatePassword_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service update-password command
	_, err, _ = executeServiceCommand(t.Context(), "service", "update-password", "--new-password", "new-password")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestServiceUpdatePassword_EnvironmentVariable(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999", // Use a local URL that will fail fast
		"service_id": "test-service-456",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Set environment variable BEFORE creating command (like root test does)
	originalEnv := os.Getenv("TIGER_NEW_PASSWORD")
	os.Setenv("TIGER_NEW_PASSWORD", "env-password-123")
	defer func() {
		if originalEnv != "" {
			os.Setenv("TIGER_NEW_PASSWORD", originalEnv)
		} else {
			os.Unsetenv("TIGER_NEW_PASSWORD")
		}
	}()

	// Execute command without --password flag (should use environment variable)
	_, err, _ = executeServiceCommand(t.Context(), "service", "update-password", "test-service-456")

	// Should fail with network error (not password missing error) since we have password from env
	if err == nil {
		t.Fatal("Expected network error since we're using a mock URL")
	}

	// Should not be a password validation error - if it gets to network call, env var worked
	if strings.Contains(err.Error(), "password is required") {
		t.Errorf("Environment variable was not picked up, got password required error: %v", err)
	}

	// Should be network/API error showing the password was found
	if !strings.Contains(err.Error(), "API request failed") && !strings.Contains(err.Error(), "failed to update service password") {
		t.Errorf("Expected network/API error indicating password was found, got: %v", err)
	}
}

func TestServiceCreate_WaitTimeoutParsing(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config to get past initial validation
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "http://localhost:9999", // Use local URL that will fail fast
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	testCases := []struct {
		name          string
		waitTimeout   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid duration - minutes",
			waitTimeout: "30m",
			expectError: false,
		},
		{
			name:        "Valid duration - hours and minutes",
			waitTimeout: "1h30m",
			expectError: false,
		},
		{
			name:        "Valid duration - seconds",
			waitTimeout: "90s",
			expectError: false,
		},
		{
			name:        "Valid duration - hours",
			waitTimeout: "2h",
			expectError: false,
		},
		{
			name:          "Invalid duration format",
			waitTimeout:   "invalid",
			expectError:   true,
			errorContains: "invalid duration", // Cobra's parsing error
		},
		{
			name:          "Negative duration",
			waitTimeout:   "-30m",
			expectError:   true,
			errorContains: "wait timeout must be positive",
		},
		{
			name:          "Zero duration",
			waitTimeout:   "0s",
			expectError:   true,
			errorContains: "wait timeout must be positive",
		},
		{
			name:          "Empty duration",
			waitTimeout:   "",
			expectError:   true,
			errorContains: "invalid duration", // Cobra's parsing error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Execute service create with specific wait-timeout
			_, err, _ := executeServiceCommand(t.Context(), "service", "create",
				"--name", "test-service",
				"--addons", "none",
				"--region", "us-east-1",
				"--wait-timeout", tc.waitTimeout,
				"--no-wait") // Use no-wait to avoid actual API calls

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for wait-timeout '%s', but got none", tc.waitTimeout)
					return
				}
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tc.errorContains, err)
				}
			} else {
				// For valid durations, we expect authentication error since we're using mock API
				// The duration parsing should succeed and we should get to the API call stage
				if err != nil && strings.Contains(err.Error(), "invalid duration") {
					t.Errorf("Unexpected duration parsing error for '%s': %v", tc.waitTimeout, err)
				}
			}
		})
	}
}

func TestWaitForServiceReady_Timeout(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config
	cfg, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "http://localhost:9999", // Non-existent server to force timeout
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Create API client
	client, err := api.NewTigerClient(cfg, "test-api-key")
	if err != nil {
		t.Fatalf("Failed to create API client: %v", err)
	}

	// Create a test command
	cmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	// Test waitForServiceReady with very short timeout to trigger timeout quickly
	_, err = waitForServiceReady(t.Context(), client, "test-project-123", "svc-12345", 100*time.Millisecond, nil, cmd.ErrOrStderr())

	// Should return an error with ExitTimeout
	if err == nil {
		t.Error("Expected error for timeout, but got none")
		return
	}

	// Check that it's an exitCodeError with ExitTimeout
	if exitErr, ok := err.(interface{ ExitCode() int }); ok {
		if exitErr.ExitCode() != ExitTimeout {
			t.Errorf("Expected exit code %d for wait timeout, got %d", ExitTimeout, exitErr.ExitCode())
		}
	} else {
		t.Error("Expected exitCodeError for wait timeout")
	}

	// Check error message mentions timeout and continuing provisioning
	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "wait timeout reached") {
		t.Errorf("Expected error message to mention timeout, got: %v", errorMsg)
	}
	if !strings.Contains(errorMsg, "service may still be provisioning") {
		t.Errorf("Expected error message to mention service may still be provisioning, got: %v", errorMsg)
	}
}

func TestServiceCommandAliases(t *testing.T) {
	// Build a fresh root command to test aliases
	rootCmd, err := buildRootCmd(t.Context())
	if err != nil {
		t.Fatalf("Failed to build root command: %v", err)
	}

	// Test that 'service' command exists
	serviceCmd, _, err := rootCmd.Find([]string{"service"})
	if err != nil {
		t.Fatalf("Failed to find 'service' command: %v", err)
	}
	if serviceCmd.Use != "service" {
		t.Errorf("Expected service command Use to be 'service', got: %s", serviceCmd.Use)
	}

	// Test that 'services' alias works
	servicesCmd, _, err := rootCmd.Find([]string{"services"})
	if err != nil {
		t.Fatalf("Failed to find 'services' alias: %v", err)
	}
	if servicesCmd != serviceCmd {
		t.Errorf("Expected 'services' alias to resolve to same command as 'service'")
	}

	// Test that 'svc' alias works
	svcCmd, _, err := rootCmd.Find([]string{"svc"})
	if err != nil {
		t.Fatalf("Failed to find 'svc' alias: %v", err)
	}
	if svcCmd != serviceCmd {
		t.Errorf("Expected 'svc' alias to resolve to same command as 'service'")
	}

	// Verify aliases are properly set in the command definition
	expectedAliases := []string{"services", "svc"}
	if len(serviceCmd.Aliases) != len(expectedAliases) {
		t.Errorf("Expected %d aliases, got %d", len(expectedAliases), len(serviceCmd.Aliases))
	}
	for i, expected := range expectedAliases {
		if i >= len(serviceCmd.Aliases) || serviceCmd.Aliases[i] != expected {
			t.Errorf("Expected alias %d to be '%s', got '%s'", i, expected, serviceCmd.Aliases[i])
		}
	}
}

func TestServiceDelete_NoServiceID(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Execute service delete command without service ID
	_, err, _ = executeServiceCommand(t.Context(), "service", "delete")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestServiceDelete_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service delete command
	_, err, _ = executeServiceCommand(t.Context(), "service", "delete", "svc-12345", "--confirm")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestServiceDelete_WithConfirmFlag(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "http://localhost:9999", // Non-existent server for testing
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service delete command with --confirm flag
	// This should fail due to network error (which is expected in tests)
	_, err, _ = executeServiceCommand(t.Context(), "service", "delete", "svc-12345", "--confirm")
	if err == nil {
		t.Fatal("Expected error due to network failure, but got none")
	}

	// Should fail with network error, not confirmation error
	if strings.Contains(err.Error(), "confirmation") {
		t.Errorf("Should not prompt for confirmation with --confirm flag, got: %v", err)
	}
}

func TestServiceDelete_ConfirmationPrompt(t *testing.T) {
	// This test verifies that without --confirm flag, the command would prompt for confirmation
	// Since we can't easily test interactive input, we test that it tries to prompt

	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service delete command without --confirm flag
	// This should try to read from stdin for confirmation, which will fail in test environment
	output, err, _ := executeServiceCommand(t.Context(), "service", "delete", "svc-12345")

	// Should either fail due to stdin read error or show cancellation message
	// The exact behavior depends on the test environment
	if err == nil && !strings.Contains(output, "Delete operation cancelled") {
		t.Error("Expected either error or cancellation message when no confirmation provided")
	}
}

func TestServiceDelete_HelpOutput(t *testing.T) {
	// Test that the help output contains expected information
	output, err, _ := executeServiceCommand(t.Context(), "service", "delete", "--help")
	if err != nil {
		t.Fatalf("Help command should not fail: %v", err)
	}

	expectedStrings := []string{
		"Delete a database service permanently",
		"irreversible",
		"--confirm",
		"--no-wait",
		"--wait-timeout",
		"tiger service delete svc-12345",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected help output to contain '%s', but it didn't. Output: %s", expected, output)
		}
	}
}

func TestServiceDelete_FlagsValidation(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"project_id": "test-project-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Test various flag combinations
	testCases := []struct {
		name string
		args []string
	}{
		{"with confirm flag", []string{"service", "delete", "svc-12345", "--confirm"}},
		{"with no-wait flag", []string{"service", "delete", "svc-12345", "--confirm", "--no-wait"}},
		{"with wait-timeout", []string{"service", "delete", "svc-12345", "--confirm", "--wait-timeout", "15m"}},
		{"with all flags", []string{"service", "delete", "svc-12345", "--confirm", "--no-wait", "--wait-timeout", "10m"}},
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// All these should fail due to network (which is expected)
			// but they should NOT fail due to flag parsing errors
			_, err, _ := executeServiceCommand(t.Context(), tc.args...)

			// Should fail with network error, not flag parsing error
			if err != nil && strings.Contains(err.Error(), "flag") {
				t.Errorf("Should not have flag parsing error, got: %v", err)
			}
		})
	}
}

func TestServiceCreate_NoSetDefaultFlag(t *testing.T) {
	// Test that the --no-set-default flag is recognized and doesn't cause parsing errors
	output, err, _ := executeServiceCommand(t.Context(), "service", "create", "--help")
	if err != nil {
		t.Fatalf("Help command should not fail: %v", err)
	}

	// Verify the flag appears in help output
	if !strings.Contains(output, "--no-set-default") {
		t.Error("Expected --no-set-default flag to appear in help output")
	}

	// Verify the flag description
	if !strings.Contains(output, "Don't set this service as the default service") {
		t.Error("Expected --no-set-default flag description to appear in help output")
	}

	// Verify the help text mentions the default behavior
	if !strings.Contains(output, "newly created service will be set as your default service") {
		t.Error("Expected help text to mention default service behavior")
	}
}

func TestOutputService_FreeTier(t *testing.T) {
	// Create a test free tier service object with null CPU and memory
	serviceID := "svc-free-123"
	serviceName := "free-tier-service"
	serviceType := api.TIMESCALEDB
	regionCode := "us-east-1"
	status := api.READY
	created := time.Now()
	replicaCount := 0
	host := "free.tigerdata.com"
	port := 5432

	service := api.Service{
		ServiceId:   &serviceID,
		Name:        &serviceName,
		ServiceType: &serviceType,
		RegionCode:  &regionCode,
		Status:      &status,
		Created:     &created,
		Resources: &[]struct {
			Id   *string `json:"id,omitempty"`
			Spec *struct {
				CpuMillis  *int    `json:"cpu_millis,omitempty"`
				MemoryGbs  *int    `json:"memory_gbs,omitempty"`
				VolumeType *string `json:"volume_type,omitempty"`
			} `json:"spec,omitempty"`
		}{
			{
				Spec: &struct {
					CpuMillis  *int    `json:"cpu_millis,omitempty"`
					MemoryGbs  *int    `json:"memory_gbs,omitempty"`
					VolumeType *string `json:"volume_type,omitempty"`
				}{
					// CPU and Memory are nil for free tier services
					CpuMillis: nil,
					MemoryGbs: nil,
				},
			},
		},
		HaReplicas: &api.HAReplica{
			ReplicaCount: &replicaCount,
		},
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Create a test command
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	// Test table output
	err := outputService(cmd, service, "table", false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify table output contains free tier indicators
	output := buf.String()
	expectedContents := []string{
		"svc-free-123",
		"free-tier-service",
		"READY",
		"TIMESCALEDB",
		"us-east-1",
		"shared", // CPU should show as "shared" for free tier
		"shared", // Memory should show as "shared" for free tier
		"0",      // Replicas
		"free.tigerdata.com:5432",
	}

	for _, content := range expectedContents {
		if !strings.Contains(output, content) {
			t.Errorf("Expected table to contain %q, got: %s", content, output)
		}
	}
}

func parseConfigFile(t *testing.T, configFile string) map[string]interface{} {
	t.Helper()

	// Read the config file directly
	configBytes, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}
	var configMap map[string]interface{}
	if err := yaml.Unmarshal(configBytes, &configMap); err != nil {
		t.Fatalf("Failed to parse config YAML: %v", err)
	}
	return configMap
}

func TestServiceCreate_OutputFlagDoesNotPersist(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with default output format (table)
	cfg, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "http://localhost:9999",
		"output":  "table", // Explicitly set default
	})
	if err != nil {
		t.Fatalf("Failed to setup test config: %v", err)
	}
	configFile := cfg.GetConfigFile()

	// Validate starting config file
	configMap := parseConfigFile(t, configFile)
	if outputVal, exists := configMap["output"]; !exists || outputVal != "table" {
		t.Fatalf("Expected initial output in config file to be 'table', got: %v", outputVal)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

	// Execute service create with -o json flag
	// This will fail due to network error (localhost:9999 doesn't exist), but that's OK
	// We just want to verify that the config file doesn't get the output flag written to it
	_, _, _ = executeServiceCommand(t.Context(), "service", "create", "-o", "json", "--cpu", "shared", "--memory", "shared")

	configMap = parseConfigFile(t, configFile)
	if outputVal, exists := configMap["output"]; !exists || outputVal != "table" {
		t.Fatalf("Expected output in config file to be 'table', got: %v", outputVal)
	}
}

func TestServiceList_OutputFlagAffectsCommandOnly(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with output format explicitly set to "table"
	cfg, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":                "http://localhost:9999",
		"output":                 "table",
		"version_check_interval": 0,
	})
	if err != nil {
		t.Fatalf("Failed to setup test config: %v", err)
	}
	configFile := cfg.GetConfigFile()

	// Mock authentication
	originalGetCredentials := getCredentialsForService
	getCredentialsForService = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForService = originalGetCredentials }()

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
