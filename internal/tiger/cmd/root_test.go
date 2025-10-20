package cmd

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestMain(m *testing.M) {
	// Clean up any global state before tests
	config.ResetGlobalConfig()
	code := m.Run()
	os.Exit(code)
}

func setupTestCommand(t *testing.T) (string, func()) {
	t.Helper()

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-test-cmd-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Clean up function
	cleanup := func() {
		os.RemoveAll(tmpDir)
		config.ResetGlobalConfig()
	}

	t.Cleanup(cleanup)

	return tmpDir, cleanup
}

func TestFlagPrecedence(t *testing.T) {
	tmpDir, _ := setupTestCommand(t)

	// Create config file with some values
	configContent := `api_url: https://file.api.com/v1
project_id: file-project
service_id: file-service
output: table
analytics: true
`
	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variables
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_PROJECT_ID", "env-project")
	os.Setenv("TIGER_SERVICE_ID", "env-service")
	os.Setenv("TIGER_OUTPUT", "json")
	os.Setenv("TIGER_ANALYTICS", "false")

	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_PROJECT_ID")
		os.Unsetenv("TIGER_SERVICE_ID")
		os.Unsetenv("TIGER_OUTPUT")
		os.Unsetenv("TIGER_ANALYTICS")
	}()

	// Use buildRootCmd() to get a complete root command
	testCmd := buildRootCmd()

	// Set CLI flags (these should take precedence)
	args := []string{
		"--config-dir", tmpDir,
		"--service-id", "flag-service",
		"--analytics=false",
		"--debug",
		"version", // Need a subcommand to execute
	}

	testCmd.SetArgs(args)

	// Execute the command to trigger PersistentPreRunE
	err := testCmd.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	// Verify Viper reflects the CLI flag values (highest precedence)
	if viper.GetString("service_id") != "flag-service" {
		t.Errorf("Expected Viper service_id 'flag-service', got '%s'", viper.GetString("service_id"))
	}
}

func TestFlagBindingWithViper(t *testing.T) {
	tmpDir, _ := setupTestCommand(t)

	// Set environment variable
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_SERVICE_ID", "test-service-1")

	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_SERVICE_ID")
	}()

	// Test 1: Environment variable should be used when no flag is set
	testCmd1 := buildRootCmd()
	testCmd1.SetArgs([]string{"version"}) // Need a subcommand
	err := testCmd1.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	if viper.GetString("service_id") != "test-service-1" {
		t.Errorf("Expected service_id 'test-service-1' from env var, got '%s'", viper.GetString("service_id"))
	}

	// Reset for next test
	config.ResetGlobalConfig()

	// Test 2: Flag should override environment variable
	testCmd2 := buildRootCmd()
	testCmd2.SetArgs([]string{"--service-id", "test-service-2", "version"})
	err = testCmd2.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	if viper.GetString("service_id") != "test-service-2" {
		t.Errorf("Expected service_id 'test-service-2' from flag, got '%s'", viper.GetString("service_id"))
	}
}

func TestConfigFilePrecedence(t *testing.T) {
	tmpDir, _ := setupTestCommand(t)

	// Create config file
	configContent := `output: json
analytics: false
`
	configFile := config.GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment that should be overridden by config file
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	defer os.Unsetenv("TIGER_CONFIG_DIR")

	// Use buildRootCmd() to get a complete root command
	testCmd := buildRootCmd()

	// Execute with config file specified
	testCmd.SetArgs([]string{"--config-dir", tmpDir, "version"})
	err := testCmd.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	// Values should come from config file since no flags were set
	if viper.GetString("output") != "json" {
		t.Errorf("Expected output 'json' from config file, got '%s'", viper.GetString("output"))
	}
	if viper.GetBool("analytics") != false {
		t.Errorf("Expected analytics false from config file, got %t", viper.GetBool("analytics"))
	}
}
