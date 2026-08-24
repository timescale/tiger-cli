package cmd

import (
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestAuthLogout_Success(t *testing.T) {
	setupAuthTest(t)

	// Store credentials first
	err := testConfig(t).StoreCredentials("test-api-key-logout", "test-project-logout")
	if err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	// Verify credentials are stored
	_, err = testConfig(t).GetStoredCredentials()
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
	_, err = testConfig(t).GetStoredCredentials()
	if err == nil {
		t.Fatal("Credentials should be removed after logout")
	}
}

// TestAuthLogout_ClearsDefaultService verifies logout removes the stored
// default service — anchored to the login's project, it must not carry over
// to whatever project the next login lands on.
func TestAuthLogout_ClearsDefaultService(t *testing.T) {
	setupAuthTest(t)

	if err := testConfig(t).StoreCredentials("test-api-key-logout", "test-project-logout"); err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}
	if err := testConfig(t).Set("service_id", "svc1234567"); err != nil {
		t.Fatalf("Failed to set default service: %v", err)
	}

	if _, err := executeAuthCommand(t.Context(), "auth", "logout"); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	assertServiceIDCleared(t)
}

// TestAuthLogout_OAuthCredentialsStayRemoved guards an edge case in the App's
// cached client: for an OAuth session that client persists refreshed tokens back
// to storage, and the analytics event deferred by wrapCommands runs *after*
// logout removed the credentials. If that event triggers a token refresh, the
// persist callback would write the credentials straight back.
func TestAuthLogout_OAuthCredentialsStayRemoved(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// The deferred analytics event has to actually be sent for this to be a
	// real test, so enable analytics and neutralize the global opt-outs.
	t.Setenv("TIGER_ANALYTICS", "true")
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("NO_TELEMETRY", "")
	t.Setenv("DISABLE_TELEMETRY", "")

	// The mock backs the refresh_token grant; everything else 404s, which is
	// fine — the refresh happens before the request is issued.
	mockServer := startMockOAuthServer(t, nil)
	configContent := fmt.Sprintf("gateway_url: \"%s\"\napi_url: \"%s\"\n", mockServer.URL, mockServer.URL)
	if err := os.WriteFile(config.GetConfigFile(tmpDir), []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// An expired access token with a refresh token the mock still honors: the
	// state where a logout triggers a refresh.
	cfg := testConfig(t)
	expired := &oauth2.Token{
		AccessToken:  "stale-access-token",
		RefreshToken: "mock-refresh-token-67890",
		Expiry:       time.Now().Add(-time.Hour),
	}
	if err := cfg.StoreOAuthCredentials(expired, "project-789"); err != nil {
		t.Fatalf("Failed to store oauth credentials: %v", err)
	}

	if _, err := executeAuthCommand(t.Context(), "auth", "logout"); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	if creds, err := testConfig(t).GetStoredCredentials(); err == nil {
		t.Fatalf("Credentials were resurrected after logout: %+v", creds)
	}
}
