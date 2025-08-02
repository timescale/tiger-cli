package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	// Clean up any global state before tests
	viper.Reset()
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
		viper.Reset()
		
		// Reset global variables
		cfgFile = ""
		debug = false
		output = ""
		apiKey = ""
		projectID = ""
		serviceID = ""
		analytics = true
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
	configFile := filepath.Join(tmpDir, "config.yaml")
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
	
	// Create a test command that uses the same persistent pre-run logic
	testCmd := &cobra.Command{
		Use: "test",
		PersistentPreRunE: rootCmd.PersistentPreRunE,
		Run: func(cmd *cobra.Command, args []string) {
			// Test command - just verify flag values
		},
	}
	
	// Use the same flag setup as the real application
	addPersistentFlags(testCmd)
	bindFlags(testCmd)
	
	// Set CLI flags (these should take precedence)
	args := []string{
		"--config", configFile,
		"--project-id", "flag-project",
		"--service-id", "flag-service", 
		"--output", "yaml",
		"--analytics=false",
		"--debug",
	}
	
	testCmd.SetArgs(args)
	
	// Execute the command to trigger PersistentPreRunE
	err := testCmd.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}
	
	// Verify CLI flags take precedence
	if projectID != "flag-project" {
		t.Errorf("Expected projectID 'flag-project' (from CLI flag), got '%s'", projectID)
	}
	if serviceID != "flag-service" {
		t.Errorf("Expected serviceID 'flag-service' (from CLI flag), got '%s'", serviceID)
	}
	if output != "yaml" {
		t.Errorf("Expected output 'yaml' (from CLI flag), got '%s'", output)
	}
	if analytics != false {
		t.Errorf("Expected analytics false (from CLI flag), got %t", analytics)
	}
	if debug != true {
		t.Errorf("Expected debug true (from CLI flag), got %t", debug)
	}
	
	// Verify Viper also reflects the flag values
	if viper.GetString("project_id") != "flag-project" {
		t.Errorf("Expected Viper project_id 'flag-project', got '%s'", viper.GetString("project_id"))
	}
	if viper.GetString("output") != "yaml" {
		t.Errorf("Expected Viper output 'yaml', got '%s'", viper.GetString("output"))
	}
}

func TestFlagBindingWithViper(t *testing.T) {
	tmpDir, _ := setupTestCommand(t)
	
	// Set environment variable
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_OUTPUT", "json")
	
	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_OUTPUT")
	}()
	
	// Test 1: Environment variable should be used when no flag is set
	testCmd1 := &cobra.Command{
		Use: "test1",
		PersistentPreRunE: rootCmd.PersistentPreRunE,
		Run: func(cmd *cobra.Command, args []string) {},
	}
	
	addPersistentFlags(testCmd1)
	bindFlags(testCmd1)
	
	testCmd1.SetArgs([]string{})
	err := testCmd1.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}
	
	if viper.GetString("output") != "json" {
		t.Errorf("Expected output 'json' from env var, got '%s'", viper.GetString("output"))
	}
	
	// Reset for next test
	viper.Reset()
	
	// Test 2: Flag should override environment variable
	testCmd2 := &cobra.Command{
		Use: "test2", 
		PersistentPreRunE: rootCmd.PersistentPreRunE,
		Run: func(cmd *cobra.Command, args []string) {},
	}
	
	addPersistentFlags(testCmd2)
	bindFlags(testCmd2)
	
	testCmd2.SetArgs([]string{"--output", "table"})
	err = testCmd2.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}
	
	if viper.GetString("output") != "table" {
		t.Errorf("Expected output 'table' from flag, got '%s'", viper.GetString("output"))
	}
}

func TestConfigFilePrecedence(t *testing.T) {
	tmpDir, _ := setupTestCommand(t)
	
	// Create config file
	configContent := `output: json
analytics: false
`
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	// Set environment that should be overridden by config file
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	
	defer os.Unsetenv("TIGER_CONFIG_DIR")
	
	// Create test command
	testCmd := &cobra.Command{
		Use: "test",
		PersistentPreRunE: rootCmd.PersistentPreRunE,
		Run: func(cmd *cobra.Command, args []string) {},
	}
	
	addPersistentFlags(testCmd)
	bindFlags(testCmd)
	
	// Execute with config file specified
	testCmd.SetArgs([]string{"--config", configFile})
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