package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setupCredentialTest(t *testing.T) string {
	t.Helper()

	// Use a unique service name for this test to avoid conflicts
	SetTestServiceName(t)

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-api-key-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Reset viper completely and set up with test directory
	// This ensures proper test isolation by resetting all viper state
	// MUST be done before RemoveCredentials() so it uses the test directory!
	if _, err := UseTestConfig(tmpDir, map[string]any{}); err != nil {
		t.Fatalf("Failed to use test config: %v", err)
	}

	// Clean up any existing credentials in the test directory
	RemoveCredentials()

	t.Cleanup(func() {
		// Clean up credentials
		RemoveCredentials()

		// Reset global config to ensure test isolation
		ResetGlobalConfig()

		// Clean up file system
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func TestStoreCredentialsToFile(t *testing.T) {
	tmpDir := setupCredentialTest(t)

	// Store credentials in new JSON format
	if err := StoreCredentialsToFile("public:secret", "project123"); err != nil {
		t.Fatalf("Failed to store credentials to file: %v", err)
	}

	// Verify file exists and has correct permissions (now stored as "credentials")
	credentialsFile := filepath.Join(tmpDir, "credentials")
	info, err := os.Stat(credentialsFile)
	if err != nil {
		t.Fatalf("Credentials file should exist: %v", err)
	}

	// Check file permissions (should be 0600)
	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected file permissions 0600, got %o", info.Mode().Perm())
	}

	// Verify file content is valid JSON
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		t.Fatalf("Failed to read credentials file: %v", err)
	}

	expectedJSON := `{"api_key":"public:secret","project_id":"project123"}`
	if string(data) != expectedJSON {
		t.Errorf("Expected '%s', got '%s'", expectedJSON, string(data))
	}
}

func TestGetCredentialsFromFile(t *testing.T) {
	tmpDir := setupCredentialTest(t)

	// Write credentials to file in JSON format
	credentialsFile := filepath.Join(tmpDir, "credentials")
	jsonData := `{"api_key":"public:secret","project_id":"project456"}`
	if err := os.WriteFile(credentialsFile, []byte(jsonData), 0600); err != nil {
		t.Fatalf("Failed to write test credentials file: %v", err)
	}

	// Get credentials - should get from file since keyring is empty
	// (each test uses a unique keyring service name)
	apiKey, projectID, err := GetCredentials()
	if err != nil {
		t.Fatalf("Failed to get credentials from file: %v", err)
	}

	// Should return combined API key (publicKey:secretKey) and project ID
	if apiKey != "public:secret" {
		t.Errorf("Expected API key 'public:secret', got '%s'", apiKey)
	}
	if projectID != "project456" {
		t.Errorf("Expected project ID 'project456', got '%s'", projectID)
	}
}

func TestGetCredentialsFromFile_NotExists(t *testing.T) {
	setupCredentialTest(t)

	// Try to get credentials when file doesn't exist
	_, _, err := GetCredentials()
	if err == nil {
		t.Fatal("Expected error when credentials file doesn't exist")
	}

	if err.Error() != "not logged in" {
		t.Errorf("Expected 'not logged in' error, got: %v", err)
	}
}

func TestRemoveCredentialsFromFile(t *testing.T) {
	tmpDir := setupCredentialTest(t)

	// Write credentials to file
	credentialsFile := filepath.Join(tmpDir, "credentials")
	if err := os.WriteFile(credentialsFile, []byte(`{"api_key":"test:key","project_id":"test-proj"}`), 0600); err != nil {
		t.Fatalf("Failed to write test credentials file: %v", err)
	}

	// Remove credentials file
	if err := RemoveCredentials(); err != nil {
		t.Fatalf("Failed to remove credentials file: %v", err)
	}

	// Verify file is removed
	if _, err := os.Stat(credentialsFile); !os.IsNotExist(err) {
		t.Fatal("Credentials file should be removed")
	}
}

func TestRemoveCredentialsFromFile_NotExists(t *testing.T) {
	setupCredentialTest(t)

	// Try to remove credentials file when it doesn't exist (should not error)
	if err := RemoveCredentials(); err != nil {
		t.Fatalf("Should not error when removing non-existent file: %v", err)
	}
}
