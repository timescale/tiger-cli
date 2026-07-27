package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

func TestServiceUpdatePassword_NoServiceID(t *testing.T) {
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

	// Execute service update-password command without service ID
	_, err, _ = executeServiceCommand(t.Context(), "service", "update-password", "--new-password", "new-password")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestServiceUpdatePassword_NoAuth(t *testing.T) {
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

	// Execute service update-password command
	_, err, _ = executeServiceCommand(t.Context(), "service", "update-password", "--new-password", "new-password")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

// update-password on a read replica ID is rejected, pointing at the primary.
func TestServiceUpdatePassword_ReadReplicaRejected(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// getService resolves the replica ID to a standby linked to its parent.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.Service{
			ServiceId:  util.Ptr("rep1234567"),
			ProjectId:  util.Ptr("test-project-123"),
			ForkedFrom: &api.ForkSpec{IsStandby: util.Ptr(true), ServiceId: util.Ptr("svcprimary")},
		})
	}))
	defer srv.Close()

	if _, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    srv.URL,
		"project_id": "test-project-123",
	}); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}
	mockTestPAT(t)

	_, err, _ := executeServiceCommand(t.Context(), "service", "update-password", "rep1234567", "--new-password", "irrelevant")
	if err == nil {
		t.Fatal("expected update-password on a read replica to be rejected")
	}
	if !strings.Contains(err.Error(), "read replica") || !strings.Contains(err.Error(), "svcprimary") {
		t.Errorf("expected guidance pointing at the primary service, got: %v", err)
	}
}

func TestServiceUpdatePassword_EnvironmentVariable(t *testing.T) {
	tmpDir := setupServiceTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999", // Use a local URL that will fail fast
		"service_id": "test-service-456",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	mockTestPAT(t)

	// Set environment variable BEFORE creating command (like root test does)
	originalEnv := os.Getenv("TIGER_NEW_PASSWORD")
	os.Setenv("TIGER_NEW_PASSWORD", "env-password-123")
	defer func() {
		if originalEnv != "" {
			os.Setenv("TIGER_NEW_PASSWORD", originalEnv)
		} else {
			os.Unsetenv("TIGER_NEW_PASSWORD")
		}
	}()

	// Execute command without --password flag (should use environment variable)
	_, err, _ = executeServiceCommand(t.Context(), "service", "update-password", "test-service-456")

	// Should fail with network error (not password missing error) since we have password from env
	if err == nil {
		t.Fatal("Expected network error since we're using a mock URL")
	}

	// Should not be a password validation error - if it gets to network call, env var worked
	if strings.Contains(err.Error(), "password is required") {
		t.Errorf("Environment variable was not picked up, got password required error: %v", err)
	}

	// Should be network/API error showing the password was found and we proceeded to API calls
	errStr := err.Error()
	if !strings.Contains(errStr, "API request failed") &&
		!strings.Contains(errStr, "failed to update service password") &&
		!strings.Contains(errStr, "failed to get service details") {
		t.Errorf("Expected network/API error indicating password was found, got: %v", err)
	}
}
