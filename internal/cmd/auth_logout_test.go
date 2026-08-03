package cmd

import (
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestAuthLogout_Success(t *testing.T) {
	setupAuthTest(t)

	// Store credentials first
	err := config.StoreCredentials("test-api-key-logout", "test-project-logout")
	if err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	// Verify credentials are stored
	_, err = config.GetStoredCredentials()
	if err != nil {
		t.Fatalf("Credentials should be stored: %v", err)
	}

	// Execute logout command
	output, err := executeAuthCommand(t.Context(), "auth", "logout")
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	if output != "Successfully logged out and removed stored credentials\n" {
		t.Errorf("Unexpected output: '%s' (len=%d)", output, len(output))
	}

	// Verify credentials are removed
	_, err = config.GetStoredCredentials()
	if err == nil {
		t.Fatal("Credentials should be removed after logout")
	}
}
