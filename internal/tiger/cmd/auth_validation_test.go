package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestAuthLogin_APIKeyValidationFailure(t *testing.T) {
	// Set up test environment but don't use setupAuthTest since we want to test validation failure
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalValidator := validateAPIKeyForLogin
	
	// Mock the validator to return an error
	validateAPIKeyForLogin = func(apiKey, projectID string) error {
		return errors.New("invalid API key: authentication failed")
	}
	
	defer func() {
		validateAPIKeyForLogin = originalValidator
	}()

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	// Clean up keyring
	keyring.Delete(serviceName, username)
	defer keyring.Delete(serviceName, username)

	// Execute login command with API key flag - should fail validation
	output, err := executeAuthCommand("auth", "login", "--api-key", "invalid-api-key")
	if err == nil {
		t.Fatal("Expected login to fail with invalid API key, but it succeeded")
	}

	expectedErrorMsg := "API key validation failed: invalid API key: authentication failed"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error to contain %q, got: %v", expectedErrorMsg, err)
	}

	// Verify that output contains validation message
	if !strings.Contains(output, "Validating API key...") {
		t.Errorf("Expected output to contain validation message, got: %s", output)
	}

	// Verify that no API key was stored
	_, err = keyring.Get(serviceName, username)
	if err == nil {
		t.Error("API key should not be stored when validation fails")
	}

	// Also check file fallback
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	if _, err := os.Stat(apiKeyFile); err == nil {
		t.Error("API key file should not exist when validation fails")
	}
}

func TestAuthLogin_APIKeyValidationSuccess(t *testing.T) {
	// Set up test environment
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalValidator := validateAPIKeyForLogin
	
	// Mock the validator to return success
	validateAPIKeyForLogin = func(apiKey, projectID string) error {
		return nil // Success
	}
	
	defer func() {
		validateAPIKeyForLogin = originalValidator
	}()

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	// Clean up keyring
	keyring.Delete(serviceName, username)
	defer keyring.Delete(serviceName, username)

	// Execute login command with API key flag - should succeed
	output, err := executeAuthCommand("auth", "login", "--api-key", "valid-api-key")
	if err != nil {
		t.Fatalf("Expected login to succeed with valid API key, got error: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key securely\n"
	if output != expectedOutput {
		t.Errorf("Expected output %q, got %q", expectedOutput, output)
	}

	// Verify that API key was stored (try keyring first, then file fallback)
	apiKey, err := keyring.Get(serviceName, username)
	if err != nil {
		// Keyring failed, check file fallback
		apiKeyFile := filepath.Join(tmpDir, "api-key")
		data, err := os.ReadFile(apiKeyFile)
		if err != nil {
			t.Fatalf("API key not stored in keyring or file: %v", err)
		}
		if string(data) != "valid-api-key" {
			t.Errorf("Expected API key 'valid-api-key', got '%s'", string(data))
		}
	} else {
		if apiKey != "valid-api-key" {
			t.Errorf("Expected API key 'valid-api-key', got '%s'", apiKey)
		}
	}
}