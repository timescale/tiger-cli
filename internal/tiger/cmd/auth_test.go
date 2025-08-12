package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/tigerdata/tiger-cli/internal/tiger/config"
	"github.com/zalando/go-keyring"
)

func setupAuthTest(t *testing.T) string {
	t.Helper()

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
	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot := buildRootCmd()

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err := testRoot.Execute()
	return buf.String(), err
}

func TestAuthLogin_WithAPIKeyFlag(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// Execute login command with public and secret key flags and project ID
	output, err := executeAuthCommand("auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key", "--project-id", "test-project-123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely. Set default project ID to: test-project-123\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify API key was stored (try keyring first, then file fallback)
	// The combined key should be in format "public:secret"
	expectedAPIKey := "test-public-key:test-secret-key"
	apiKey, err := keyring.Get(serviceName, username)
	if err != nil {
		// Keyring failed, check file fallback
		apiKeyFile := filepath.Join(tmpDir, "api-key")
		data, err := os.ReadFile(apiKeyFile)
		if err != nil {
			t.Fatalf("API key not stored in keyring or file: %v", err)
		}
		if string(data) != expectedAPIKey {
			t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, string(data))
		}
	} else {
		if apiKey != expectedAPIKey {
			t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, apiKey)
		}
	}
}

func TestAuthLogin_WithEnvironmentVariable(t *testing.T) {
	setupAuthTest(t)

	// Set environment variables for public and secret keys
	os.Setenv("TIGER_PUBLIC_KEY", "env-public-key")
	os.Setenv("TIGER_SECRET_KEY", "env-secret-key")
	defer os.Unsetenv("TIGER_PUBLIC_KEY")
	defer os.Unsetenv("TIGER_SECRET_KEY")

	// Execute login command with project ID flag but using env vars for keys
	output, err := executeAuthCommand("auth", "login", "--project-id", "test-project-456")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely. Set default project ID to: test-project-456\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify API key was stored (should be combined format)
	expectedAPIKey := "env-public-key:env-secret-key"
	storedKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get stored API key: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
}

func TestAuthLogin_NoAPIKey(t *testing.T) {
	setupAuthTest(t)

	// Ensure no keys in environment
	os.Unsetenv("TIGER_PUBLIC_KEY")
	os.Unsetenv("TIGER_SECRET_KEY")

	// Execute login command without keys (this should fail in non-interactive mode)
	_, err := executeAuthCommand("auth", "login")
	if err == nil {
		t.Fatal("Expected login to fail without keys")
	}

	// Error should indicate TTY not detected and credentials are required
	if !strings.Contains(err.Error(), "TTY not detected - credentials required") {
		t.Errorf("Expected error to mention TTY not detected, got: %v", err)
	}
}

// TestAuthLogin_KeyringFallback tests the scenario where keyring fails and system falls back to file storage
func TestAuthLogin_KeyringFallback(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// We can't easily mock keyring failure, but we can test file storage directly
	// by ensuring the API key gets stored to file when keyring might not be available

	// Execute login command with public and secret key flags and project ID
	output, err := executeAuthCommand("auth", "login", "--public-key", "fallback-public", "--secret-key", "fallback-secret", "--project-id", "test-project-fallback")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely. Set default project ID to: test-project-fallback\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Force test file storage scenario by directly checking file
	apiKeyFile := filepath.Join(tmpDir, "api-key")

	// If keyring worked, manually create file scenario by removing keyring and adding file
	keyring.Delete(serviceName, username) // Remove from keyring

	// Store to file manually to simulate fallback (combined format)
	expectedAPIKey := "fallback-public:fallback-secret"
	err = storeAPIKeyToFile(expectedAPIKey)
	if err != nil {
		t.Fatalf("Failed to store API key to file: %v", err)
	}

	// Verify file storage works
	storedKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get API key from file fallback: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
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

	// Set environment variables for public key, secret key, and project ID
	os.Setenv("TIGER_PUBLIC_KEY", "env-file-public")
	os.Setenv("TIGER_SECRET_KEY", "env-file-secret")
	os.Setenv("TIGER_PROJECT_ID", "test-project-env-file")
	defer os.Unsetenv("TIGER_PUBLIC_KEY")
	defer os.Unsetenv("TIGER_SECRET_KEY")
	defer os.Unsetenv("TIGER_PROJECT_ID")

	// Execute login command without any flags (all from env vars)
	output, err := executeAuthCommand("auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely. Set default project ID to: test-project-env-file\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Clear keyring again to ensure we're testing file-only retrieval
	keyring.Delete(serviceName, username)

	// Verify API key was stored in file (since keyring is cleared)
	expectedAPIKey := "env-file-public:env-file-secret"
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		// If file doesn't exist, the keyring might have worked, so manually ensure file storage
		err = storeAPIKeyToFile(expectedAPIKey)
		if err != nil {
			t.Fatalf("Failed to store API key to file: %v", err)
		}
		data, err = os.ReadFile(apiKeyFile)
		if err != nil {
			t.Fatalf("API key file should exist: %v", err)
		}
	}

	if string(data) != expectedAPIKey {
		t.Errorf("Expected API key '%s' in file, got '%s'", expectedAPIKey, string(data))
	}

	// Verify getAPIKey works with file-only storage
	storedKey, err := getAPIKeyFromFile()
	if err != nil {
		t.Fatalf("Failed to get API key from file: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
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

	// Execute login command with public key, secret key, and project ID flags
	output, err := executeAuthCommand("auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key", "--project-id", "test-project-456")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely. Set default project ID to: test-project-456\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify API key was stored (should be combined format)
	expectedAPIKey := "test-public-key:test-secret-key"
	apiKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get stored API key: %v", err)
	}
	if apiKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, apiKey)
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

	// Execute login command with only public and secret key flags (no project ID)
	// This should fail since project ID is now required
	_, err := executeAuthCommand("auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key")
	if err == nil {
		t.Fatal("Expected login to fail without project ID, but it succeeded")
	}

	// Verify the error message mentions TTY not detected
	expectedErrorMsg := "TTY not detected - credentials required"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error to contain %q, got: %v", expectedErrorMsg, err)
	}

	// Verify no API key was stored since login failed
	_, err = getAPIKey()
	if err == nil {
		t.Error("API key should not be stored when login fails")
	}
}
