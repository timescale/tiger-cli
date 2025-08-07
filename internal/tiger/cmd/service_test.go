package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	// Reset global flag variables to ensure clean state for each test
	createServiceName = ""
	createServiceType = "timescaledb"
	createRegionCode = "us-east-1"
	createCpuMillis = 500
	createMemoryGbs = 2.0
	createReplicaCount = 1
	createNoWait = false
	createTimeoutMinutes = 30
	
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
	_, err = executeServiceCommand("service", "create", "--type", "postgres", "--region", "us-east-1")
	// This should fail due to network/API call, not due to missing name
	if err != nil && (strings.Contains(err.Error(), "name") && strings.Contains(err.Error(), "required")) {
		t.Errorf("Should not fail due to missing name anymore (should auto-generate), got: %v", err)
	}
	
	// Test with explicit empty region (should still fail validation)
	_, err = executeServiceCommand("service", "create", "--name", "test", "--type", "postgres", "--region", "")
	if err == nil {
		t.Fatal("Expected error when region is empty")
	}
	if !strings.Contains(err.Error(), "region") && !strings.Contains(err.Error(), "empty") {
		t.Errorf("Expected error about empty region, got: %v", err)
	}
	
	// Test invalid service type - this should fail validation before making API call
	_, err = executeServiceCommand("service", "create", "--type", "invalid-type", "--region", "us-east-1", "--cpu", "1000", "--memory", "4", "--replicas", "1")
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
	_, err := executeServiceCommand("service", "create", "--type", "postgres", "--region", "us-east-1")
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
	_, err = executeServiceCommand("service", "create", "--type", "postgres", "--region", "us-east-1", "--cpu", "1000", "--memory", "4", "--replicas", "1")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}
	
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestSavePgPassEntry(t *testing.T) {
	tmpDir := setupServiceTest(t)
	
	// Create a temporary home directory for testing
	testHomeDir := filepath.Join(tmpDir, "home")
	err := os.MkdirAll(testHomeDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test home directory: %v", err)
	}
	
	// Set HOME environment variable for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", testHomeDir)
	defer func() { 
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	
	// Create test service data
	host := "test-host.tigerdata.com"
	port := 5432
	password := "test-password-123"
	serviceID := "12345678-9abc-def0-1234-56789abcdef0"
	
	service := api.Service{
		ServiceId: &serviceID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
		InitialPassword: &password,
	}
	
	// Test saving entry
	err = savePgPassEntry(service, password)
	if err != nil {
		t.Fatalf("Failed to save pgpass entry: %v", err)
	}
	
	// Verify the .pgpass file was created
	pgpassPath := filepath.Join(testHomeDir, ".pgpass")
	if _, err := os.Stat(pgpassPath); os.IsNotExist(err) {
		t.Fatal("Expected .pgpass file to be created")
	}
	
	// Check file permissions
	fileInfo, err := os.Stat(pgpassPath)
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}
	expectedPerms := os.FileMode(0600)
	if fileInfo.Mode().Perm() != expectedPerms {
		t.Errorf("Expected file permissions %v, got %v", expectedPerms, fileInfo.Mode().Perm())
	}
	
	// Read and verify file contents
	content, err := os.ReadFile(pgpassPath)
	if err != nil {
		t.Fatalf("Failed to read .pgpass file: %v", err)
	}
	
	expectedEntry := "test-host.tigerdata.com:5432:tsdb:tsdbadmin:test-password-123\n"
	if string(content) != expectedEntry {
		t.Errorf("Expected pgpass entry %q, got %q", expectedEntry, string(content))
	}
}

func TestSavePgPassEntry_DefaultPort(t *testing.T) {
	tmpDir := setupServiceTest(t)
	
	// Create a temporary home directory for testing
	testHomeDir := filepath.Join(tmpDir, "home")
	err := os.MkdirAll(testHomeDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test home directory: %v", err)
	}
	
	// Set HOME environment variable for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", testHomeDir)
	defer func() { 
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	
	// Create test service data without explicit port (should use default 5432)
	host := "test-host.tigerdata.com"
	password := "test-password-123"
	serviceID := "12345678-9abc-def0-1234-56789abcdef0"
	
	service := api.Service{
		ServiceId: &serviceID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: nil, // No port specified - should default to 5432
		},
		InitialPassword: &password,
	}
	
	// Test saving entry
	err = savePgPassEntry(service, password)
	if err != nil {
		t.Fatalf("Failed to save pgpass entry: %v", err)
	}
	
	// Read and verify file contents use default port
	pgpassPath := filepath.Join(testHomeDir, ".pgpass")
	content, err := os.ReadFile(pgpassPath)
	if err != nil {
		t.Fatalf("Failed to read .pgpass file: %v", err)
	}
	
	expectedEntry := "test-host.tigerdata.com:5432:tsdb:tsdbadmin:test-password-123\n"
	if string(content) != expectedEntry {
		t.Errorf("Expected pgpass entry %q, got %q", expectedEntry, string(content))
	}
}

func TestSavePgPassEntry_AppendToExisting(t *testing.T) {
	tmpDir := setupServiceTest(t)
	
	// Create a temporary home directory for testing
	testHomeDir := filepath.Join(tmpDir, "home")
	err := os.MkdirAll(testHomeDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test home directory: %v", err)
	}
	
	// Set HOME environment variable for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", testHomeDir)
	defer func() { 
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	
	// Create existing .pgpass file with some content
	pgpassPath := filepath.Join(testHomeDir, ".pgpass")
	existingContent := "existing-host:5432:existing-db:existing-user:existing-pass\n"
	err = os.WriteFile(pgpassPath, []byte(existingContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create existing .pgpass file: %v", err)
	}
	
	// Create test service data
	host := "new-host.tigerdata.com"
	port := 5432
	password := "new-password-123"
	serviceID := "12345678-9abc-def0-1234-56789abcdef0"
	
	service := api.Service{
		ServiceId: &serviceID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
		InitialPassword: &password,
	}
	
	// Test saving entry
	err = savePgPassEntry(service, password)
	if err != nil {
		t.Fatalf("Failed to save pgpass entry: %v", err)
	}
	
	// Read and verify file contents include both entries
	content, err := os.ReadFile(pgpassPath)
	if err != nil {
		t.Fatalf("Failed to read .pgpass file: %v", err)
	}
	
	expectedContent := existingContent + "new-host.tigerdata.com:5432:tsdb:tsdbadmin:new-password-123\n"
	if string(content) != expectedContent {
		t.Errorf("Expected pgpass content %q, got %q", expectedContent, string(content))
	}
}

func TestPgpassEntryExists(t *testing.T) {
	tmpDir := setupServiceTest(t)
	
	// Create a temporary home directory for testing
	testHomeDir := filepath.Join(tmpDir, "home")
	err := os.MkdirAll(testHomeDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test home directory: %v", err)
	}
	
	pgpassPath := filepath.Join(testHomeDir, ".pgpass")
	
	// Test with non-existent file
	exists, err := pgpassEntryExists(pgpassPath, "test-host", "5432", "tsdbadmin")
	if err != nil {
		t.Fatalf("Unexpected error checking non-existent file: %v", err)
	}
	if exists {
		t.Error("Expected entry to not exist in non-existent file")
	}
	
	// Create .pgpass file with test entries
	content := `host1:5432:db1:user1:pass1
test-host.tigerdata.com:5432:tsdb:tsdbadmin:existing-pass
host2:3306:db2:user2:pass2
test-host.tigerdata.com:5433:tsdb:tsdbadmin:different-port
`
	err = os.WriteFile(pgpassPath, []byte(content), 0600)
	if err != nil {
		t.Fatalf("Failed to create test .pgpass file: %v", err)
	}
	
	// Test existing entry
	exists, err = pgpassEntryExists(pgpassPath, "test-host.tigerdata.com", "5432", "tsdbadmin")
	if err != nil {
		t.Fatalf("Unexpected error checking existing entry: %v", err)
	}
	if !exists {
		t.Error("Expected entry to exist")
	}
	
	// Test non-existing entry (different host)
	exists, err = pgpassEntryExists(pgpassPath, "different-host", "5432", "tsdbadmin")
	if err != nil {
		t.Fatalf("Unexpected error checking non-existing entry: %v", err)
	}
	if exists {
		t.Error("Expected entry to not exist for different host")
	}
	
	// Test non-existing entry (different port)
	exists, err = pgpassEntryExists(pgpassPath, "test-host.tigerdata.com", "5431", "tsdbadmin")
	if err != nil {
		t.Fatalf("Unexpected error checking non-existing entry: %v", err)
	}
	if exists {
		t.Error("Expected entry to not exist for different port")
	}
	
	// Test non-existing entry (different user)
	exists, err = pgpassEntryExists(pgpassPath, "test-host.tigerdata.com", "5432", "different-user")
	if err != nil {
		t.Fatalf("Unexpected error checking non-existing entry: %v", err)
	}
	if exists {
		t.Error("Expected entry to not exist for different user")
	}
}

func TestSavePgPassEntry_SkipDuplicate(t *testing.T) {
	tmpDir := setupServiceTest(t)
	
	// Create a temporary home directory for testing
	testHomeDir := filepath.Join(tmpDir, "home")
	err := os.MkdirAll(testHomeDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test home directory: %v", err)
	}
	
	// Set HOME environment variable for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", testHomeDir)
	defer func() { 
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	
	// Create existing .pgpass file with the entry we're about to add
	pgpassPath := filepath.Join(testHomeDir, ".pgpass")
	existingContent := "test-host.tigerdata.com:5432:tsdb:tsdbadmin:existing-pass\n"
	err = os.WriteFile(pgpassPath, []byte(existingContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create existing .pgpass file: %v", err)
	}
	
	// Create test service data with same host/port/user
	host := "test-host.tigerdata.com"
	port := 5432
	password := "new-password-123"
	serviceID := "12345678-9abc-def0-1234-56789abcdef0"
	
	service := api.Service{
		ServiceId: &serviceID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
		InitialPassword: &password,
	}
	
	// Test saving entry (should not add duplicate)
	err = savePgPassEntry(service, password)
	if err != nil {
		t.Fatalf("Failed to save pgpass entry: %v", err)
	}
	
	// Read and verify file contents are unchanged (no duplicate)
	content, err := os.ReadFile(pgpassPath)
	if err != nil {
		t.Fatalf("Failed to read .pgpass file: %v", err)
	}
	
	if string(content) != existingContent {
		t.Errorf("Expected pgpass content to remain unchanged %q, got %q", existingContent, string(content))
	}
}

func TestSavePgPassEntry_ErrorCases(t *testing.T) {
	// Test with nil endpoint
	service := api.Service{
		Endpoint: nil,
	}
	err := savePgPassEntry(service, "password")
	if err == nil {
		t.Error("Expected error when endpoint is nil")
	}
	if !strings.Contains(err.Error(), "service endpoint not available") {
		t.Errorf("Expected endpoint error, got: %v", err)
	}
	
	// Test with nil host
	service = api.Service{
		Endpoint: &api.Endpoint{
			Host: nil,
		},
	}
	err = savePgPassEntry(service, "password")
	if err == nil {
		t.Error("Expected error when host is nil")
	}
	if !strings.Contains(err.Error(), "service endpoint not available") {
		t.Errorf("Expected endpoint error, got: %v", err)
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
			name:         "CPU only auto-configure memory (2 CPU -> 8GB)",
			cpuMillis:    2000,
			cpuFlagSet:   true,
			memoryFlagSet: false,
			expectCPU:    2000,
			expectMemory: 8,
			expectError:  false,
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
			name:         "Invalid CPU only",
			cpuMillis:    1500,
			cpuFlagSet:   true,
			memoryFlagSet: false,
			expectError:  true,
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
	_, err = executeServiceCommand("service", "create", "--type", "postgres", "--region", "us-east-1")
	
	// The command should not fail due to missing service name
	if err != nil && strings.Contains(err.Error(), "service name is required") {
		t.Error("Service name should be auto-generated, not required")
	}
	
	// Verify that createServiceName gets set to something like "db-####"
	// by checking it's not empty after reset and execution
	// This is a bit indirect but tests the core functionality
	if createServiceName == "" {
		t.Error("Service name should have been auto-generated")
	}
	
	// Check pattern (should start with "db-" followed by numbers)
	if !strings.HasPrefix(createServiceName, "db-") {
		t.Errorf("Auto-generated name should start with 'db-', got: %s", createServiceName)
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
	_, err := executeServiceCommand("service", "describe", "svc-12345")
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
	_, err = executeServiceCommand("service", "describe")
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
	_, err = executeServiceCommand("service", "describe")
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
				CpuMillis   *int    `json:"cpu_millis,omitempty"`
				MemoryGbs   *int    `json:"memory_gbs,omitempty"`
				VolumeType  *string `json:"volume_type,omitempty"`
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