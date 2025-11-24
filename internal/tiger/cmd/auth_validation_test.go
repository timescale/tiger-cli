package cmd

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestAuthLogin_APIKeyValidationFailure(t *testing.T) {
	// Set up test environment but don't use setupAuthTest since we want to test validation failure
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use a unique service name for this test
	config.SetTestServiceName(t)

	originalValidator := validateAndGetAuthInfo

	// Mock the validator to return an error
	validateAndGetAuthInfo = func(ctx context.Context, cfg *config.Config, apiKey string) (*api.AuthInfo, error) {
		return nil, errors.New("invalid API key: authentication failed")
	}

	defer func() {
		validateAndGetAuthInfo = originalValidator
	}()

	// Initialize viper with test directory BEFORE calling RemoveCredentials()
	// This ensures RemoveCredentials() operates on the test directory, not the user's real directory
	if _, err := config.UseTestConfig(tmpDir, map[string]any{}); err != nil {
		t.Fatalf("Failed to use test config: %v", err)
	}

	// Clean up credentials
	config.RemoveCredentials()
	defer config.RemoveCredentials()

	// Execute login command with public and secret key flags - should fail validation
	output, err := executeAuthCommand(t.Context(), "auth", "login", "--public-key", "invalid-public", "--secret-key", "invalid-secret")
	if err == nil {
		t.Fatal("Expected login to fail with invalid keys, but it succeeded")
	}

	expectedErrorMsg := "API key validation failed: invalid API key: authentication failed"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error to contain %q, got: %v", expectedErrorMsg, err)
	}

	// Verify that output contains validation message
	if !strings.Contains(output, "Validating API key...") {
		t.Errorf("Expected output to contain validation message, got: %s", output)
	}

	// Verify that no credentials were stored
	if _, _, err := config.GetCredentials(); err == nil {
		t.Error("Credentials should not be stored when validation fails")
	}
}

func TestAuthLogin_APIKeyValidationSuccess(t *testing.T) {
	// Set up test environment
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use a unique service name for this test
	config.SetTestServiceName(t)

	originalValidator := validateAndGetAuthInfo

	// Mock the validator to return success
	validateAndGetAuthInfo = func(ctx context.Context, cfg *config.Config, apiKey string) (*api.AuthInfo, error) {
		authInfo := &api.AuthInfo{
			Type: api.ApiKey,
		}
		authInfo.ApiKey.Project.Id = "test-project-valid"
		authInfo.ApiKey.PublicKey = "test-access-key"
		return authInfo, nil // Success
	}

	defer func() {
		validateAndGetAuthInfo = originalValidator
	}()

	// Initialize viper with test directory BEFORE calling RemoveCredentials()
	// This ensures RemoveCredentials() operates on the test directory, not the user's real directory
	if _, err := config.UseTestConfig(tmpDir, map[string]any{}); err != nil {
		t.Fatalf("Failed to use test config: %v", err)
	}

	// Clean up credentials
	config.RemoveCredentials()
	defer config.RemoveCredentials()

	// Execute login command with public and secret key flags - should succeed
	output, err := executeAuthCommand(t.Context(), "auth", "login", "--public-key", "valid-public", "--secret-key", "valid-secret")
	if err != nil {
		t.Fatalf("Expected login to succeed with valid keys, got error: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-valid)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Expected output %q, got %q", expectedOutput, output)
	}

	// Verify that credentials were stored
	expectedAPIKey := "valid-public:valid-secret"
	expectedProjectID := "test-project-valid"
	apiKey, projectID, err := config.GetCredentials()
	if err != nil {
		t.Fatalf("Credentials not stored in keyring or file: %v", err)
	}
	if apiKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, apiKey)
	}
	if projectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, projectID)
	}
}
