package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/config"
)

func TestAuthLogoutCmd(t *testing.T) {
	// Backs the server-side revocation (POST /auth/logout) for OAuth sessions,
	// capturing the request body so the table can assert the refresh token was
	// sent.
	var logoutBody string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		logoutBody = string(body)
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	oauthToken := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		Expiry:       time.Now().Add(time.Hour),
	}

	checkNothingStored := func(t *testing.T, result cmdResult) {
		t.Helper()
		if creds, err := readStoredCredentials(t, result.configDir); err == nil {
			t.Errorf("expected credentials to be removed, got: %+v", creds)
		}
	}

	tests := []cmdTest{
		{
			name:    "rejects positional args",
			args:    []string{"auth", "logout", "extra"},
			wantErr: `unknown command "extra" for "tiger auth logout"`,
		},
		{
			name:       "not logged in still succeeds",
			args:       []string{"auth", "logout"},
			wantStdout: "Successfully logged out and removed stored credentials\n",
			check:      checkNothingStored,
		},
		{
			name: "removes stored PAT credentials",
			args: []string{"auth", "logout"},
			opts: []runOption{
				withStoredCredentials(config.Credentials{
					APIKey:    "test-public-key:test-secret-key",
					ProjectID: "test-project-123",
				}),
			},
			wantStdout: "Successfully logged out and removed stored credentials\n",
			check:      checkNothingStored,
		},
		{
			name: "revokes OAuth session server-side and removes credentials",
			args: []string{"auth", "logout"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": server.URL}),
				withStoredCredentials(config.Credentials{
					OAuth:     oauthToken,
					ProjectID: "test-project-123",
				}),
			},
			wantStdout: "Successfully logged out and removed stored credentials\n",
			check: func(t *testing.T, result cmdResult) {
				checkNothingStored(t, result)
				if want := `{"refresh_token":"test-refresh-token"}`; logoutBody != want {
					t.Errorf("server-side logout body = %q, want %q", logoutBody, want)
				}
			},
		},
	}
	runCmdTests(t, tests)

	// Guards an edge case in the App's cached client: for an OAuth session that
	// client persists refreshed tokens back to storage, and the analytics event
	// deferred by wrapCommands runs *after* logout removed the credentials. If
	// that event triggers a token refresh, a persisting client would write the
	// credentials straight back — logout must hand the App a non-persisting one.
	t.Run("OAuth credentials stay removed after deferred analytics", func(t *testing.T) {
		// The mock backs the refresh_token grant; everything else 404s, which
		// is fine — the refresh happens before each request is issued.
		oauthServer := startMockOAuthServer(t, nil)

		// An expired access token with a refresh token the mock still honors:
		// the state where a logout triggers a refresh. The deferred analytics
		// event has to actually be sent for this to be a real test, so enable
		// analytics and neutralize the global opt-outs.
		expired := &oauth2.Token{
			AccessToken:  "stale-access-token",
			RefreshToken: "mock-refresh-token-67890",
			Expiry:       time.Now().Add(-time.Hour),
		}
		result := runCommand(t, []string{"auth", "logout", "--analytics=true"}, nil,
			withConfig(map[string]any{
				"gateway_url": oauthServer.URL,
				"api_url":     oauthServer.URL,
			}),
			withStoredCredentials(config.Credentials{OAuth: expired, ProjectID: "project-789"}),
			withEnv("DO_NOT_TRACK", ""),
			withEnv("NO_TELEMETRY", ""),
			withEnv("DISABLE_TELEMETRY", ""))

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "Successfully logged out and removed stored credentials\n")
		if creds, err := readStoredCredentials(t, result.configDir); err == nil {
			t.Fatalf("credentials were resurrected after logout: %+v", creds)
		}
	})
}
