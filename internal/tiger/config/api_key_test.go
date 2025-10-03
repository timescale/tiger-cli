package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func setupAPIKeyTest(t *testing.T) string {
	t.Helper()

	// Clean up any existing keyring entries before test
	RemoveAPIKeyFromKeyring()

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-api-key-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Wire up the tmp directory as the viper config directory
	viper.SetConfigFile(GetConfigFile(tmpDir))

	t.Cleanup(func() {
		// Clean up keyring entries
		RemoveAPIKeyFromKeyring()

		// Reset global config to ensure test isolation
		ResetGlobalConfig()

		// Clean up file system
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func TestStoreAPIKeyToFile(t *testing.T) {
	tmpDir := setupAPIKeyTest(t)

	if err := StoreAPIKeyToFile("file-test-key"); err != nil {
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
	tmpDir := setupAPIKeyTest(t)
	viper.SetConfigFile(GetConfigFile(tmpDir))

	// Write API key to file
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	if err := os.WriteFile(apiKeyFile, []byte("file-get-test-key"), 0600); err != nil {
		t.Fatalf("Failed to write test API key file: %v", err)
	}

	// Get API key from file
	apiKey, err := GetAPIKey()
	if err != nil {
		t.Fatalf("Failed to get API key from file: %v", err)
	}

	if apiKey != "file-get-test-key" {
		t.Errorf("Expected 'file-get-test-key', got '%s'", apiKey)
	}
}

func TestGetAPIKeyFromFile_NotExists(t *testing.T) {
	tmpDir := setupAPIKeyTest(t)
	viper.SetConfigFile(GetConfigFile(tmpDir))

	// Try to get API key when file doesn't exist
	_, err := GetAPIKey()
	if err == nil {
		t.Fatal("Expected error when API key file doesn't exist")
	}

	if err.Error() != "not logged in" {
		t.Errorf("Expected 'not logged in' error, got: %v", err)
	}
}

func TestRemoveAPIKeyFromFile(t *testing.T) {
	tmpDir := setupAPIKeyTest(t)
	viper.SetConfigFile(GetConfigFile(tmpDir))

	// Write API key to file
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	if err := os.WriteFile(apiKeyFile, []byte("remove-test-key"), 0600); err != nil {
		t.Fatalf("Failed to write test API key file: %v", err)
	}

	// Remove API key file
	if err := RemoveAPIKeyFromFile(); err != nil {
		t.Fatalf("Failed to remove API key file: %v", err)
	}

	// Verify file is removed
	if _, err := os.Stat(apiKeyFile); !os.IsNotExist(err) {
		t.Fatal("API key file should be removed")
	}
}

func TestRemoveAPIKeyFromFile_NotExists(t *testing.T) {
	setupAPIKeyTest(t)

	// Try to remove API key file when it doesn't exist (should not error)
	if err := RemoveAPIKeyFromFile(); err != nil {
		t.Fatalf("Should not error when removing non-existent file: %v", err)
	}
}
