package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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

	// Re-establish viper environment configuration after reset
	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

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

func executeServiceCommand(args ...string) (string, error, *cobra.Command) {
	// No need to reset any flags - we build fresh commands with local variables

	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot := buildRootCmd()

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err := testRoot.Execute()
	return buf.String(), err, testRoot
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
	_, err, _ := executeServiceCommand("service", "list")
	if err == nil {
		t.Fatal("Expected error when no project ID is configured")
	}

	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("Expected error about missing project ID, got: %v", err)
	}
}

func TestServiceList_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID and API URL
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
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
	_, err, _ = executeServiceCommand("service", "list")
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
	// Test service ID formatting (now uses string instead of UUID)
	testServiceID := "12345678-9abc-def0-1234-56789abcdef0"
	if derefString(&testServiceID) != testServiceID {
		t.Error("derefString should return service ID string")
	}
	if derefString(nil) != "" {
		t.Error("derefString should return empty string for nil")
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

func TestServiceCreate_ValidationErrors(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID and a mock API URL to prevent network calls
	cfg := &config.Config{
		APIURL:    "http://localhost:9999", // Use a local URL that will fail fast
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Test with no name (should auto-generate) - this should now work without error
	// Just test that it doesn't fail due to missing name
	_, err, _ = executeServiceCommand("service", "create", "--type", "postgres", "--region", "us-east-1")
	// This should fail due to network/API call, not due to missing name
	if err != nil && (strings.Contains(err.Error(), "name") && strings.Contains(err.Error(), "required")) {
		t.Errorf("Should not fail due to missing name anymore (should auto-generate), got: %v", err)
	}

	// Test with explicit empty region (should still fail validation)
	_, err, _ = executeServiceCommand("service", "create", "--name", "test", "--type", "postgres", "--region", "")
	if err == nil {
		t.Fatal("Expected error when region is empty")
	}
	if !strings.Contains(err.Error(), "region") && !strings.Contains(err.Error(), "empty") {
		t.Errorf("Expected error about empty region, got: %v", err)
	}

	// Test invalid service type - this should fail validation before making API call
	_, err, _ = executeServiceCommand("service", "create", "--type", "invalid-type", "--region", "us-east-1", "--cpu", "1000", "--memory", "4", "--replicas", "1")
	if err == nil {
		t.Fatal("Expected error when service type is invalid")
	}
	if !strings.Contains(err.Error(), "invalid service type") {
		t.Errorf("Expected error about invalid service type, got: %v", err)
	}
}

func TestServiceCreate_NoProjectID(t *testing.T) {
	setupServiceTest(t)

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Execute service create command without project ID (name will be auto-generated)
	_, err, _ := executeServiceCommand("service", "create", "--type", "postgres", "--region", "us-east-1")
	if err == nil {
		t.Fatal("Expected error when no project ID is configured")
	}

	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("Expected error about missing project ID, got: %v", err)
	}
}

func TestServiceCreate_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID and API URL
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
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

	// Execute service create command with valid parameters (name will be auto-generated)
	_, err, _ = executeServiceCommand("service", "create", "--type", "postgres", "--region", "us-east-1", "--cpu", "1000", "--memory", "4", "--replicas", "1")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestValidateAndNormalizeCPUMemory(t *testing.T) {
	// Test valid combinations when both flags are set
	testCases := []struct {
		name          string
		cpuMillis     int
		memoryGbs     float64
		cpuFlagSet    bool
		memoryFlagSet bool
		expectCPU     int
		expectMemory  float64
		expectError   bool
	}{
		{
			name:          "Valid combination both set (1 CPU, 4GB)",
			cpuMillis:     1000,
			memoryGbs:     4,
			cpuFlagSet:    true,
			memoryFlagSet: true,
			expectCPU:     1000,
			expectMemory:  4,
			expectError:   false,
		},
		{
			name:          "Valid combination both set (0.5 CPU, 2GB)",
			cpuMillis:     500,
			memoryGbs:     2,
			cpuFlagSet:    true,
			memoryFlagSet: true,
			expectCPU:     500,
			expectMemory:  2,
			expectError:   false,
		},
		{
			name:          "Invalid combination both set (1 CPU, 8GB)",
			cpuMillis:     1000,
			memoryGbs:     8,
			cpuFlagSet:    true,
			memoryFlagSet: true,
			expectError:   true,
		},
		{
			name:          "CPU only auto-configure memory (2 CPU -> 8GB)",
			cpuMillis:     2000,
			cpuFlagSet:    true,
			memoryFlagSet: false,
			expectCPU:     2000,
			expectMemory:  8,
			expectError:   false,
		},
		{
			name:          "Memory only auto-configure CPU (16GB -> 4 CPU)",
			memoryGbs:     16,
			cpuFlagSet:    false,
			memoryFlagSet: true,
			expectCPU:     4000,
			expectMemory:  16,
			expectError:   false,
		},
		{
			name:          "Invalid CPU only",
			cpuMillis:     1500,
			cpuFlagSet:    true,
			memoryFlagSet: false,
			expectError:   true,
		},
		{
			name:          "Invalid memory only",
			memoryGbs:     6,
			cpuFlagSet:    false,
			memoryFlagSet: true,
			expectError:   true,
		},
		{
			name:          "Neither flag set (use defaults)",
			cpuFlagSet:    false,
			memoryFlagSet: false,
			expectCPU:     500,
			expectMemory:  2,
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cpu, memory, err := validateAndNormalizeCPUMemory(tc.cpuMillis, tc.memoryGbs, tc.cpuFlagSet, tc.memoryFlagSet)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if cpu != tc.expectCPU {
				t.Errorf("Expected CPU %d, got %d", tc.expectCPU, cpu)
			}

			if memory != tc.expectMemory {
				t.Errorf("Expected memory %.0f, got %.1f", tc.expectMemory, memory)
			}
		})
	}
}

func TestGetAllowedCPUMemoryConfigs(t *testing.T) {
	configs := getAllowedCPUMemoryConfigs()

	// Verify we have the expected number of configurations
	expectedCount := 7
	if len(configs) != expectedCount {
		t.Errorf("Expected %d configurations, got %d", expectedCount, len(configs))
	}

	// Verify specific configurations from the spec
	expectedConfigs := []CPUMemoryConfig{
		{CPUMillis: 500, MemoryGbs: 2},
		{CPUMillis: 1000, MemoryGbs: 4},
		{CPUMillis: 2000, MemoryGbs: 8},
		{CPUMillis: 4000, MemoryGbs: 16},
		{CPUMillis: 8000, MemoryGbs: 32},
		{CPUMillis: 16000, MemoryGbs: 64},
		{CPUMillis: 32000, MemoryGbs: 128},
	}

	for i, expected := range expectedConfigs {
		if i < len(configs) {
			if configs[i].CPUMillis != expected.CPUMillis || configs[i].MemoryGbs != expected.MemoryGbs {
				t.Errorf("Config %d: expected %+v, got %+v", i, expected, configs[i])
			}
		}
	}
}

func TestFormatAllowedCombinations(t *testing.T) {
	configs := []CPUMemoryConfig{
		{CPUMillis: 500, MemoryGbs: 2},
		{CPUMillis: 1000, MemoryGbs: 4},
		{CPUMillis: 2000, MemoryGbs: 8},
	}

	result := formatAllowedCombinations(configs)
	expected := "0.5 CPU/2GB, 1 CPU/4GB, 2 CPU/8GB"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatAllowedCPUValues(t *testing.T) {
	configs := []CPUMemoryConfig{
		{CPUMillis: 500, MemoryGbs: 2},
		{CPUMillis: 1000, MemoryGbs: 4},
		{CPUMillis: 2000, MemoryGbs: 8},
	}

	result := formatAllowedCPUValues(configs)
	expected := "0.5 (500m), 1 (1000m), 2 (2000m)"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatAllowedMemoryValues(t *testing.T) {
	configs := []CPUMemoryConfig{
		{CPUMillis: 500, MemoryGbs: 2},
		{CPUMillis: 1000, MemoryGbs: 4},
		{CPUMillis: 2000, MemoryGbs: 8},
	}

	result := formatAllowedMemoryValues(configs)
	expected := "2GB, 4GB, 8GB"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
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

	// Set up config with project ID and a mock API URL to prevent network calls
	cfg := &config.Config{
		APIURL:    "http://localhost:9999", // Use a local URL that will fail fast
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Test that service name is auto-generated when not provided
	// We expect this to fail at the API call stage, not at validation
	var rootCmd *cobra.Command
	_, err, rootCmd = executeServiceCommand("service", "create", "--type", "postgres", "--region", "us-east-1")

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

func TestServiceDescribe_NoProjectID(t *testing.T) {
	setupServiceTest(t)

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Execute service describe command without project ID
	_, err, _ := executeServiceCommand("service", "describe", "svc-12345")
	if err == nil {
		t.Fatal("Expected error when no project ID is configured")
	}

	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("Expected error about missing project ID, got: %v", err)
	}
}

func TestServiceDescribe_NoServiceID(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID but no default service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Execute service describe command without service ID
	_, err, _ = executeServiceCommand("service", "describe")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestServiceDescribe_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID and service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ServiceID: "svc-12345",
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

	// Execute service describe command
	_, err, _ = executeServiceCommand("service", "describe")
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
	err := outputService(cmd, service, "json")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify JSON output
	output := buf.String()
	if !strings.Contains(output, `"service_id": "svc-12345"`) {
		t.Errorf("Expected JSON to contain service ID, got: %s", output)
	}

	// Verify that initialpassword is NOT in the output
	if strings.Contains(output, "secret-password-123") || strings.Contains(output, "initialpassword") || strings.Contains(output, "initial_password") {
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
	err := outputService(cmd, service, "yaml")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify YAML output
	output := buf.String()
	if !strings.Contains(output, "service_id: svc-12345") {
		t.Errorf("Expected YAML to contain service ID, got: %s", output)
	}

	// Verify that initialpassword is NOT in the output
	if strings.Contains(output, "secret-password-123") || strings.Contains(output, "initialpassword") {
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
	err := outputService(cmd, service, "table")
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

func TestSanitizeServiceForOutput(t *testing.T) {
	// Create a service with sensitive data
	serviceID := "svc-12345"
	serviceName := "test-service"
	initialPassword := "secret-password-123"

	service := api.Service{
		ServiceId:       &serviceID,
		Name:            &serviceName,
		InitialPassword: &initialPassword,
	}

	// Sanitize the service
	sanitized := sanitizeServiceForOutput(service)

	// Verify that sensitive fields are removed
	if _, exists := sanitized["initial_password"]; exists {
		t.Error("Expected initial_password to be removed from sanitized service")
	}
	if _, exists := sanitized["initialpassword"]; exists {
		t.Error("Expected initialpassword to be removed from sanitized service")
	}

	// Verify that other fields are preserved
	if serviceIDVal, exists := sanitized["service_id"]; !exists || serviceIDVal != serviceID {
		t.Error("Expected service_id to be preserved in sanitized service")
	}
	if nameVal, exists := sanitized["name"]; !exists || nameVal != serviceName {
		t.Error("Expected name to be preserved in sanitized service")
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
	sanitized := sanitizeServicesForOutput(services)

	// Verify that we have the same number of services
	if len(sanitized) != len(services) {
		t.Errorf("Expected %d sanitized services, got %d", len(services), len(sanitized))
	}

	// Verify that sensitive fields are removed from all services
	for i, service := range sanitized {
		if _, exists := service["initial_password"]; exists {
			t.Errorf("Expected initial_password to be removed from sanitized service %d", i)
		}
		if _, exists := service["initialpassword"]; exists {
			t.Errorf("Expected initialpassword to be removed from sanitized service %d", i)
		}

		// Verify that other fields are preserved
		if _, exists := service["service_id"]; !exists {
			t.Errorf("Expected service_id to be preserved in sanitized service %d", i)
		}
		if _, exists := service["name"]; !exists {
			t.Errorf("Expected name to be preserved in sanitized service %d", i)
		}
	}
}

func TestServiceUpdatePassword_NoProjectID(t *testing.T) {
	setupServiceTest(t)

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Execute service update-password command without project ID
	_, err, _ := executeServiceCommand("service", "update-password", "svc-12345", "--password", "new-password")
	if err == nil {
		t.Fatal("Expected error when no project ID is configured")
	}

	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("Expected error about missing project ID, got: %v", err)
	}
}

func TestServiceUpdatePassword_NoServiceID(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID but no default service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Execute service update-password command without service ID
	_, err, _ = executeServiceCommand("service", "update-password", "--password", "new-password")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestServiceUpdatePassword_NoPassword(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID and service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ServiceID: "svc-12345",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Execute service update-password command without password
	_, err, _ = executeServiceCommand("service", "update-password")
	if err == nil {
		t.Fatal("Expected error when no password is provided")
	}

	if !strings.Contains(err.Error(), "password is required") {
		t.Errorf("Expected error about missing password, got: %v", err)
	}
}

func TestServiceUpdatePassword_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID and service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ServiceID: "svc-12345",
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

	// Execute service update-password command
	_, err, _ = executeServiceCommand("service", "update-password", "--password", "new-password")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestServiceUpdatePassword_EnvironmentVariable(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID
	cfg := &config.Config{
		APIURL:    "http://localhost:9999", // Use a local URL that will fail fast
		ProjectID: "test-project-123",
		ServiceID: "test-service-456",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Set environment variable BEFORE creating command (like root test does)
	originalEnv := os.Getenv("TIGER_PASSWORD")
	os.Setenv("TIGER_PASSWORD", "env-password-123")
	defer func() {
		if originalEnv != "" {
			os.Setenv("TIGER_PASSWORD", originalEnv)
		} else {
			os.Unsetenv("TIGER_PASSWORD")
		}
	}()

	// Execute command without --password flag (should use environment variable)
	_, err, _ = executeServiceCommand("service", "update-password", "test-service-456")

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

	// Set up config with project ID to get past initial validation
	cfg := &config.Config{
		APIURL:    "http://localhost:9999", // Use local URL that will fail fast
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

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
			_, err, _ := executeServiceCommand("service", "create",
				"--name", "test-service",
				"--type", "postgres",
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
	cfg := &config.Config{
		APIURL:    "http://localhost:9999", // Non-existent server to force timeout
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetAPIKey := getAPIKeyForService
	getAPIKeyForService = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForService = originalGetAPIKey }()

	// Create API client
	client, err := api.NewTigerClient("test-api-key")
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
	err = waitForServiceReady(client, "test-project-123", "svc-12345", 100*time.Millisecond, cmd)

	// Should return an error with exit code 2
	if err == nil {
		t.Error("Expected error for timeout, but got none")
		return
	}

	// Check that it's an exitCodeError with code 2
	if exitErr, ok := err.(interface{ ExitCode() int }); ok {
		if exitErr.ExitCode() != 2 {
			t.Errorf("Expected exit code 2 for wait timeout, got %d", exitErr.ExitCode())
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
	rootCmd := buildRootCmd()

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
