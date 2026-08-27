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

	// Backs the refresh_token grant for the deferred-analytics case; everything
	// else 404s, which is fine — the refresh happens before each request.
	oauthServer := startMockOAuthServer(t, nil)

	oauthToken := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		Expiry:       time.Now().Add(time.Hour),
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "rejects positional args",
			args:    []string{"auth", "logout", "extra"},
			wantErr: `unknown command "extra" for "tiger auth logout"`,
		},
		{
			name:       "not logged in still succeeds",
			args:       []string{"auth", "logout"},
			wantStdout: "Successfully logged out and removed stored credentials\n",
			checks:     []checkFunc{checkNoStoredCredentials},
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
			checks:     []checkFunc{checkNoStoredCredentials},
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
			checks: []checkFunc{checkNoStoredCredentials, func(t *testing.T, result cmdResult) {
				if want := `{"refresh_token":"test-refresh-token"}`; logoutBody != want {
					t.Errorf("server-side logout body = %q, want %q", logoutBody, want)
				}
			}},
		},
		{
			// Server-side revocation failures are non-fatal: warn on stderr,
			// still remove the local credentials and succeed. The transport
			// error's wording varies by OS, hence the prefix match.
			name: "warns when server-side revocation fails",
			args: []string{"auth", "logout"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": "http://127.0.0.1:1"}),
				withStoredCredentials(config.Credentials{
					OAuth:     oauthToken,
					ProjectID: "test-project-123",
				}),
			},
			wantStdout: "Successfully logged out and removed stored credentials\n",
			wantStderr: matchPrefix("warning: server-side logout failed: "),
			checks:     []checkFunc{checkNoStoredCredentials},
		},
		{
			// Guards an edge case in the App's cached client: for an OAuth
			// session that client persists refreshed tokens back to storage,
			// and the analytics event deferred by wrapCommands runs *after*
			// logout removed the credentials. If that event triggers a token
			// refresh, a persisting client would write the credentials straight
			// back — logout must hand the App a non-persisting one. The stored
			// token is expired with a refresh token the mock OAuth server still
			// honors: the state where a logout triggers a refresh. The deferred
			// analytics event has to actually be sent for this to be a real
			// test, so --analytics=true (overriding the harness default) and
			// the global opt-outs are neutralized.
			name: "OAuth credentials stay removed after deferred analytics",
			args: []string{"auth", "logout", "--analytics=true"},
			opts: []runOption{
				withConfig(map[string]any{
					"gateway_url": oauthServer.URL,
					"api_url":     oauthServer.URL,
				}),
				withStoredCredentials(config.Credentials{
					OAuth: &oauth2.Token{
						AccessToken:  "stale-access-token",
						RefreshToken: "mock-refresh-token-67890",
						Expiry:       time.Now().Add(-time.Hour),
					},
					ProjectID: "project-789",
				}),
				withEnv("DO_NOT_TRACK", ""),
				withEnv("NO_TELEMETRY", ""),
				withEnv("DISABLE_TELEMETRY", ""),
			},
			wantStdout: "Successfully logged out and removed stored credentials\n",
			checks:     []checkFunc{checkNoStoredCredentials},
		},
	})
}
