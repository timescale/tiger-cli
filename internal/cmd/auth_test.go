package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
)

// startMockOAuthServer serves the endpoints the OAuth flows hit: the token
// endpoint (both the authorization_code exchange and the refresh_token grant),
// the project listing backing project selection, and the post-login success
// redirect. Both grants return the same canned token so downstream assertions
// stay stable. Shared by the auth login and logout tests.
func startMockOAuthServer(t *testing.T, projects []api.Project) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /idp/external/cli/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			return
		}

		switch r.FormValue("grant_type") {
		case "refresh_token":
			if r.FormValue("refresh_token") == "" || r.FormValue("client_id") == "" {
				http.Error(w, "missing required parameters", http.StatusBadRequest)
				return
			}
		default:
			if r.FormValue("client_id") == "" || r.FormValue("code") == "" || r.FormValue("code_verifier") == "" {
				http.Error(w, "missing required parameters", http.StatusBadRequest)
				return
			}
			// The code exchange must carry the CLI User-Agent (recorded
			// server-side as the session's user_agent).
			if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "tiger-cli/") {
				t.Errorf("code exchange User-Agent = %q, want \"tiger-cli/\" prefix", ua)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "mock-access-token-12345",
			"refresh_token": "mock-refresh-token-67890",
			"expires_in":    3600,
		})
	})

	// REST endpoint backing selectProjectID
	mux.HandleFunc("GET /projects", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projects)
	})

	mux.HandleFunc("GET /oauth/code/success", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// checkNoStoredCredentials asserts that no credentials are stored (either none
// were written, or they were removed). Shared by the login and logout tests.
func checkNoStoredCredentials(t *testing.T, result cmdResult) {
	t.Helper()
	if creds, err := readStoredCredentials(t, result.configDir); err == nil {
		t.Errorf("expected no stored credentials, got: %+v", creds)
	}
}
