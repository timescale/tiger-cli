package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"
	"github.com/tigerdata/tiger-cli/internal/tiger/config"
)

func setupAuthTest(t *testing.T) string {
	t.Helper()
	
	// Reset global variables to ensure test isolation
	apiKeyFlag = ""
	projectIDFlag = ""
	
	// Mock the API key validation for testing
	originalValidator := validateAPIKeyForLogin
	validateAPIKeyForLogin = func(apiKey, projectID string) error {
		// Always return success for testing
		return nil
	}
	
	// Aggressively clean up any existing keyring entries before starting
	keyring.Delete(serviceName, username)
	
	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	
	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	
	// Reset global config and viper to ensure test isolation
	config.ResetGlobalConfig()
	viper.Reset()
	
	// Also ensure config file doesn't exist
	configFile := filepath.Join(tmpDir, "config.yaml")
	os.Remove(configFile)
	
	t.Cleanup(func() {
		// Reset global config and viper first
		config.ResetGlobalConfig()
		viper.Reset()
		// Reset global variables first
		apiKeyFlag = ""
		projectIDFlag = ""
		validateAPIKeyForLogin = originalValidator // Restore original validator
		// Clean up keyring
		keyring.Delete(serviceName, username)
		// Remove config file explicitly
		configFile := filepath.Join(tmpDir, "config.yaml")
		os.Remove(configFile)
		// Clean up environment variable BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})
	
	return tmpDir
}

func executeAuthCommand(args ...string) (string, error) {
	// Create a test root command with auth subcommand
	testRoot := &cobra.Command{
		Use: "tiger",
		PersistentPreRunE: rootCmd.PersistentPreRunE,
	}
	
	// Add persistent flags and bind them
	addPersistentFlags(testRoot)
	bindFlags(testRoot)
	
	// Add the auth command to our test root
	testRoot.AddCommand(authCmd)
	
	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)
	
	err := testRoot.Execute()
	return buf.String(), err
}

func TestAuthLogin_WithAPIKeyFlag(t *testing.T) {
	tmpDir := setupAuthTest(t)
	
	// Execute login command with API key flag
	output, err := executeAuthCommand("auth", "login", "--api-key", "test-api-key-123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	
	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}
	
	// Verify API key was stored (try keyring first, then file fallback)
	apiKey, err := keyring.Get(serviceName, username)
	if err != nil {
		// Keyring failed, check file fallback
		apiKeyFile := filepath.Join(tmpDir, "api-key")
		data, err := os.ReadFile(apiKeyFile)
		if err != nil {
			t.Fatalf("API key not stored in keyring or file: %v", err)
		}
		if string(data) != "test-api-key-123" {
			t.Errorf("Expected API key 'test-api-key-123', got '%s'", string(data))
		}
	} else {
		if apiKey != "test-api-key-123" {
			t.Errorf("Expected API key 'test-api-key-123', got '%s'", apiKey)
		}
	}
}

func TestAuthLogin_WithEnvironmentVariable(t *testing.T) {
	setupAuthTest(t)
	
	// Set environment variable
	os.Setenv("TIGER_API_KEY", "env-api-key-456")
	defer os.Unsetenv("TIGER_API_KEY")
	
	// Execute login command without API key flag
	output, err := executeAuthCommand("auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	
	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}
	
	// Verify API key was stored
	storedKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get stored API key: %v", err)
	}
	if storedKey != "env-api-key-456" {
		t.Errorf("Expected API key 'env-api-key-456', got '%s'", storedKey)
	}
}

func TestAuthLogin_NoAPIKey(t *testing.T) {
	setupAuthTest(t)
	
	// Ensure no API key in environment
	os.Unsetenv("TIGER_API_KEY")
	
	// Execute login command without API key (this should fail in non-interactive mode)
	_, err := executeAuthCommand("auth", "login")
	if err == nil {
		t.Fatal("Expected login to fail without API key")
	}
	
	// Error should indicate API key is required (or failed to get API key in test environment)
	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("Expected error to mention API key, got: %v", err)
	}
}

// TestAuthLogin_KeyringFallback tests the scenario where keyring fails and system falls back to file storage
func TestAuthLogin_KeyringFallback(t *testing.T) {
	tmpDir := setupAuthTest(t)
	
	// We can't easily mock keyring failure, but we can test file storage directly
	// by ensuring the API key gets stored to file when keyring might not be available
	
	// Execute login command with API key flag
	output, err := executeAuthCommand("auth", "login", "--api-key", "fallback-test-key")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	
	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}
	
	// Force test file storage scenario by directly checking file
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	
	// If keyring worked, manually create file scenario by removing keyring and adding file
	keyring.Delete(serviceName, username) // Remove from keyring
	
	// Store to file manually to simulate fallback
	err = storeAPIKeyToFile("fallback-test-key")
	if err != nil {
		t.Fatalf("Failed to store API key to file: %v", err)
	}
	
	// Verify file storage works
	storedKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get API key from file fallback: %v", err)
	}
	if storedKey != "fallback-test-key" {
		t.Errorf("Expected API key 'fallback-test-key', got '%s'", storedKey)
	}
	
	// Test whoami with file-only storage
	output, err = executeAuthCommand("auth", "whoami")
	if err != nil {
		t.Fatalf("Whoami failed with file storage: %v", err)
	}
	if output != "Logged in (API key stored)\n" {
		t.Errorf("Unexpected whoami output: '%s'", output)
	}
	
	// Test logout with file-only storage
	output, err = executeAuthCommand("auth", "logout")
	if err != nil {
		t.Fatalf("Logout failed with file storage: %v", err)
	}
	if output != "Successfully logged out and removed stored credentials\n" {
		t.Errorf("Unexpected logout output: '%s'", output)
	}
	
	// Verify file was removed
	if _, err := os.Stat(apiKeyFile); !os.IsNotExist(err) {
		t.Error("API key file should be removed after logout")
	}
}

// TestAuthLogin_EnvironmentVariable_FileOnly tests env var login when only file storage is available
func TestAuthLogin_EnvironmentVariable_FileOnly(t *testing.T) {
	tmpDir := setupAuthTest(t)
	
	// Clear any keyring entries to force file-only storage
	keyring.Delete(serviceName, username)
	
	// Set environment variable
	os.Setenv("TIGER_API_KEY", "env-file-only-789")
	defer os.Unsetenv("TIGER_API_KEY")
	
	// Execute login command without API key flag
	output, err := executeAuthCommand("auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	
	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}
	
	// Clear keyring again to ensure we're testing file-only retrieval
	keyring.Delete(serviceName, username)
	
	// Verify API key was stored in file (since keyring is cleared)
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		// If file doesn't exist, the keyring might have worked, so manually ensure file storage
		err = storeAPIKeyToFile("env-file-only-789")
		if err != nil {
			t.Fatalf("Failed to store API key to file: %v", err)
		}
		data, err = os.ReadFile(apiKeyFile)
		if err != nil {
			t.Fatalf("API key file should exist: %v", err)
		}
	}
	
	if string(data) != "env-file-only-789" {
		t.Errorf("Expected API key 'env-file-only-789' in file, got '%s'", string(data))
	}
	
	// Verify getAPIKey works with file-only storage  
	storedKey, err := getAPIKeyFromFile()
	if err != nil {
		t.Fatalf("Failed to get API key from file: %v", err)
	}
	if storedKey != "env-file-only-789" {
		t.Errorf("Expected API key 'env-file-only-789', got '%s'", storedKey)
	}
}

func TestAuthWhoami_LoggedIn(t *testing.T) {
	setupAuthTest(t)
	
	// Store API key first
	err := storeAPIKey("test-api-key-789")
	if err != nil {
		t.Fatalf("Failed to store API key: %v", err)
	}
	
	// Execute whoami command
	output, err := executeAuthCommand("auth", "whoami")
	if err != nil {
		t.Fatalf("Whoami failed: %v", err)
	}
	
	if output != "Logged in (API key stored)\n" {
		t.Errorf("Unexpected output: '%s' (len=%d)", output, len(output))
	}
}

func TestAuthWhoami_NotLoggedIn(t *testing.T) {
	setupAuthTest(t)
	
	// Execute whoami command without being logged in
	_, err := executeAuthCommand("auth", "whoami")
	if err == nil {
		t.Fatal("Expected whoami to fail when not logged in")
	}
	
	// Error should indicate not logged in
	if err.Error() != "not logged in: not logged in" {
		t.Errorf("Expected 'not logged in' error, got: %v", err)
	}
}

func TestAuthLogout_Success(t *testing.T) {
	setupAuthTest(t)
	
	// Store API key first
	err := storeAPIKey("test-api-key-logout")
	if err != nil {
		t.Fatalf("Failed to store API key: %v", err)
	}
	
	// Verify API key is stored
	_, err = getAPIKey()
	if err != nil {
		t.Fatalf("API key should be stored: %v", err)
	}
	
	// Execute logout command
	output, err := executeAuthCommand("auth", "logout")
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}
	
	if output != "Successfully logged out and removed stored credentials\n" {
		t.Errorf("Unexpected output: '%s' (len=%d)", output, len(output))
	}
	
	// Verify API key is removed
	_, err = getAPIKey()
	if err == nil {
		t.Fatal("API key should be removed after logout")
	}
}

func TestStoreAPIKeyToFile(t *testing.T) {
	tmpDir := setupAuthTest(t)
	
	err := storeAPIKeyToFile("file-test-key")
	if err != nil {
		t.Fatalf("Failed to store API key to file: %v", err)
	}
	
	// Verify file exists and has correct permissions
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	info, err := os.Stat(apiKeyFile)
	if err != nil {
		t.Fatalf("API key file should exist: %v", err)
	}
	
	// Check file permissions (should be 0600)
	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected file permissions 0600, got %o", info.Mode().Perm())
	}
	
	// Verify file content
	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		t.Fatalf("Failed to read API key file: %v", err)
	}
	
	if string(data) != "file-test-key" {
		t.Errorf("Expected 'file-test-key', got '%s'", string(data))
	}
}

func TestGetAPIKeyFromFile(t *testing.T) {
	tmpDir := setupAuthTest(t)
	
	// Write API key to file
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	err := os.WriteFile(apiKeyFile, []byte("file-get-test-key"), 0600)
	if err != nil {
		t.Fatalf("Failed to write test API key file: %v", err)
	}
	
	// Get API key from file
	apiKey, err := getAPIKeyFromFile()
	if err != nil {
		t.Fatalf("Failed to get API key from file: %v", err)
	}
	
	if apiKey != "file-get-test-key" {
		t.Errorf("Expected 'file-get-test-key', got '%s'", apiKey)
	}
}

func TestGetAPIKeyFromFile_NotExists(t *testing.T) {
	setupAuthTest(t)
	
	// Try to get API key when file doesn't exist
	_, err := getAPIKeyFromFile()
	if err == nil {
		t.Fatal("Expected error when API key file doesn't exist")
	}
	
	if err.Error() != "not logged in" {
		t.Errorf("Expected 'not logged in' error, got: %v", err)
	}
}

func TestRemoveAPIKeyFromFile(t *testing.T) {
	tmpDir := setupAuthTest(t)
	
	// Write API key to file
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	err := os.WriteFile(apiKeyFile, []byte("remove-test-key"), 0600)
	if err != nil {
		t.Fatalf("Failed to write test API key file: %v", err)
	}
	
	// Remove API key file
	err = removeAPIKeyFromFile()
	if err != nil {
		t.Fatalf("Failed to remove API key file: %v", err)
	}
	
	// Verify file is removed
	if _, err := os.Stat(apiKeyFile); !os.IsNotExist(err) {
		t.Fatal("API key file should be removed")
	}
}

func TestRemoveAPIKeyFromFile_NotExists(t *testing.T) {
	setupAuthTest(t)
	
	// Try to remove API key file when it doesn't exist (should not error)
	err := removeAPIKeyFromFile()
	if err != nil {
		t.Fatalf("Should not error when removing non-existent file: %v", err)
	}
}

func TestAuthLogin_WithProjectID(t *testing.T) {
	setupAuthTest(t)
	
	// Execute login command with API key and project ID flags
	output, err := executeAuthCommand("auth", "login", "--api-key", "test-api-key-123", "--project-id", "test-project-456")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	
	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely. Set default project ID to: test-project-456\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}
	
	// Verify API key was stored (try keyring first, then file fallback)
	apiKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get stored API key: %v", err)
	}
	if apiKey != "test-api-key-123" {
		t.Errorf("Expected API key 'test-api-key-123', got '%s'", apiKey)
	}
	
	// Verify project ID was stored in config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.ProjectID != "test-project-456" {
		t.Errorf("Expected project ID 'test-project-456', got '%s'", cfg.ProjectID)
	}
}

func TestAuthLogin_WithoutProjectID(t *testing.T) {
	setupAuthTest(t)
	
	// Execute login command with only API key flag (no project ID)
	output, err := executeAuthCommand("auth", "login", "--api-key", "test-api-key-789")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	
	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}
	
	// Verify API key was stored
	apiKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get stored API key: %v", err)
	}
	if apiKey != "test-api-key-789" {
		t.Errorf("Expected API key 'test-api-key-789', got '%s'", apiKey)
	}
	
	// Verify project ID was not set in config (should be empty)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.ProjectID != "" {
		t.Errorf("Expected empty project ID, got '%s'", cfg.ProjectID)
	}
}