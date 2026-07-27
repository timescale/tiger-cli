package cmd

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestServiceGet_NoServiceID(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with project ID but no default service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"project_id": "test-project-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	mockTestPAT(t)

	// Execute service get command without service ID
	_, err, _ = executeServiceCommand(t.Context(), "service", "get")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestServiceGet_NoAuth(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	mockNotLoggedIn(t)

	// Execute service get command
	_, err, _ = executeServiceCommand(t.Context(), "service", "get")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}
