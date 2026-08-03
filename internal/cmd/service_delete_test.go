package cmd

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestServiceDelete_NoServiceID(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Execute service delete command without service ID
	_, err, _ = executeServiceCommand(t.Context(), "service", "delete")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestServiceDelete_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	mockNotLoggedIn(t)

	// Execute service delete command
	_, err, _ = executeServiceCommand(t.Context(), "service", "delete", "svc-12345", "--confirm")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestServiceDelete_WithConfirmFlag(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "http://localhost:9999", // Non-existent server for testing
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	mockTestPAT(t)

	// Execute service delete command with --confirm flag
	// This should fail due to network error (which is expected in tests)
	_, err, _ = executeServiceCommand(t.Context(), "service", "delete", "svc-12345", "--confirm")
	if err == nil {
		t.Fatal("Expected error due to network failure, but got none")
	}

	// Should fail with network error, not confirmation error
	if strings.Contains(err.Error(), "confirmation") {
		t.Errorf("Should not prompt for confirmation with --confirm flag, got: %v", err)
	}
}

func TestServiceDelete_ConfirmationPrompt(t *testing.T) {
	// This test verifies that without --confirm flag, the command would prompt for confirmation
	// Since we can't easily test interactive input, we test that it tries to prompt

	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	mockTestPAT(t)

	// Execute service delete command without --confirm flag
	// This should try to read from stdin for confirmation, which will fail in test environment
	output, err, _ := executeServiceCommand(t.Context(), "service", "delete", "svc-12345")

	// Should either fail due to stdin read error or show cancellation message
	// The exact behavior depends on the test environment
	if err == nil && !strings.Contains(output, "Delete operation cancelled") {
		t.Error("Expected either error or cancellation message when no confirmation provided")
	}
}

func TestServiceDelete_HelpOutput(t *testing.T) {
	// Test that the help output contains expected information
	output, err, _ := executeServiceCommand(t.Context(), "service", "delete", "--help")
	if err != nil {
		t.Fatalf("Help command should not fail: %v", err)
	}

	expectedStrings := []string{
		"Delete a database service permanently",
		"irreversible",
		"--confirm",
		"--no-wait",
		"--wait-timeout",
		"tiger service delete svc-12345",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected help output to contain '%s', but it didn't. Output: %s", expected, output)
		}
	}
}

func TestServiceDelete_FlagsValidation(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"project_id": "test-project-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Test various flag combinations
	testCases := []struct {
		name string
		args []string
	}{
		{"with confirm flag", []string{"service", "delete", "svc-12345", "--confirm"}},
		{"with no-wait flag", []string{"service", "delete", "svc-12345", "--confirm", "--no-wait"}},
		{"with wait-timeout", []string{"service", "delete", "svc-12345", "--confirm", "--wait-timeout", "15m"}},
		{"with all flags", []string{"service", "delete", "svc-12345", "--confirm", "--no-wait", "--wait-timeout", "10m"}},
	}

	// Mock authentication
	mockTestPAT(t)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// All these should fail due to network (which is expected)
			// but they should NOT fail due to flag parsing errors
			_, err, _ := executeServiceCommand(t.Context(), tc.args...)

			// Should fail with network error, not flag parsing error
			if err != nil && strings.Contains(err.Error(), "flag") {
				t.Errorf("Should not have flag parsing error, got: %v", err)
			}
		})
	}
}
