package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestAuthStatus_LoggedIn(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// Create a mock server for the /auth/info endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/info" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"type": "apiKey",
				"api_key": {
					"public_key": "test-access-key",
					"name": "Test Credentials",
					"created": "2025-01-01T00:00:00Z",
					"project": {"id": "test-project-789", "name": "Test Project", "plan_type": "free"},
					"issuing_user": {"id": "user-123", "name": "Test User", "email": "test@example.com"}
				}
			}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	// Update config to use mock server
	configFile := config.GetConfigFile(tmpDir)
	configContent := fmt.Sprintf("api_url: \"%s\"\n", mockServer.URL)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Store credentials first
	err := testConfig(t).StoreCredentials("test-api-key-789", "test-project-789")
	if err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	// Execute status command - it will call the mock /auth/info endpoint
	output, err := executeAuthCommand(t.Context(), "auth", "status")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	// Verify output contains key auth information
	if !strings.Contains(output, "test-project-789") {
		t.Errorf("Expected output to contain project ID 'test-project-789': '%s'", output)
	}
	if !strings.Contains(output, "test-access-key") {
		t.Errorf("Expected output to contain access key: '%s'", output)
	}
	if !strings.Contains(output, "Test User") {
		t.Errorf("Expected output to contain issuing user name: '%s'", output)
	}
	if !strings.Contains(output, "Free") {
		t.Errorf("Expected output to contain plan type 'Free': '%s'", output)
	}
}

func TestAuthStatus_NotLoggedIn(t *testing.T) {
	setupAuthTest(t)

	// Execute status command without being logged in
	_, err := executeAuthCommand(t.Context(), "auth", "status")
	if err == nil {
		t.Fatal("Expected status to fail when not logged in")
	}

	// Error should indicate not logged in
	if err.Error() != config.ErrNotLoggedIn.Error() {
		t.Errorf("Expected 'not logged in' error, got: %v", err)
	}
}
