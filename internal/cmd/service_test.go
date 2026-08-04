package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func setupServiceTest(t *testing.T) string {
	t.Helper()

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

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
	mockTestPAT(t)

	// Create API client
	client, err := api.NewTigerClient(cfg, "test-api-key")
	if err != nil {
		t.Fatalf("Failed to create API Client: %v", err)
	}

	// Create a test command
	cmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	// Create a service object for the handler
	service := api.Service{}

	// Test common.WaitForService with very short timeout to trigger timeout quickly
	err = common.WaitForService(t.Context(), common.WaitForServiceArgs{
		Client:    client,
		ProjectID: "test-project-123",
		ServiceID: "svc-12345",
		Handler: &common.StatusWaitHandler{
			TargetStatus: "READY",
			Service:      &service,
		},
		Output:     cmd.ErrOrStderr(),
		Timeout:    100 * time.Millisecond,
		TimeoutMsg: "service may still be provisioning",
	})

	// Should return an error with common.ExitTimeout
	if err == nil {
		t.Error("Expected error for timeout, but got none")
		return
	}

	// Check that it's an exitCodeError with common.ExitTimeout
	if exitErr, ok := err.(interface{ ExitCode() int }); ok {
		if exitErr.ExitCode() != common.ExitTimeout {
			t.Errorf("Expected exit code %d for wait timeout, got %d", common.ExitTimeout, exitErr.ExitCode())
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

// TestDestructiveCommands_ReadOnly verifies that destructive service commands
// refuse to run when read_only mode is enabled, before any API call is made.
// The localhost:9999 api_url would surface any unintended request as a
// connection-refused error rather than ErrReadOnly.
func TestDestructiveCommands_ReadOnly(t *testing.T) {
	tmpDir := setupServiceTest(t)
	if _, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":   "http://localhost:9999",
		"read_only": true,
	}); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	mockTestPAT(t)

	cases := [][]string{
		{"service", "create", "--addons", "none", "--region", "us-east-1", "--cpu", "1000", "--memory", "4", "--replicas", "1"},
		{"service", "fork", "source-service-123", "--now"},
		{"service", "start", "source-service-123"},
		{"service", "stop", "source-service-123"},
		{"service", "resize", "source-service-123", "--cpu", "1000", "--memory", "4"},
		{"service", "update-password", "source-service-123", "--auto-generate"},
		{"service", "delete", "source-service-123", "--confirm"},
	}

	for _, args := range cases {
		t.Run(args[1], func(t *testing.T) {
			_, err, _ := executeServiceCommand(t.Context(), args...)
			if !errors.Is(err, common.ErrReadOnly) {
				t.Errorf("Expected common.ErrReadOnly, got: %v", err)
			}
		})
	}
}
