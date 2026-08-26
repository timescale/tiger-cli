package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestAuthLoginCmd(t *testing.T) {
	// Backs the PAT path's key validation (/auth/info): keys with an "invalid-"
	// public key are rejected, everything else is accepted with the project ID
	// "test-project-id".
	authInfoServer := startFakeAuthInfoServer(t)
	patURLs := map[string]any{"api_url": authInfoServer.URL}

	tests := []cmdTest{
		{
			name:    "rejects positional args",
			args:    []string{"auth", "login", "extra"},
			wantErr: `unknown command "extra" for "tiger auth login"`,
		},
		{
			name:    "missing secret key without TTY",
			args:    []string{"auth", "login", "--public-key", "test-public-key"},
			wantErr: "failed to get credentials: TTY not detected - credentials required. Use flags (--public-key, --secret-key) or environment variables (TIGER_PUBLIC_KEY, TIGER_SECRET_KEY)",
		},
		{
			name:    "missing public key without TTY",
			args:    []string{"auth", "login", "--secret-key", "test-secret-key"},
			wantErr: "failed to get credentials: TTY not detected - credentials required. Use flags (--public-key, --secret-key) or environment variables (TIGER_PUBLIC_KEY, TIGER_SECRET_KEY)",
		},
		{
			name: "prompts for missing secret key",
			args: []string{"auth", "login", "--public-key", "test-public-key"},
			opts: []runOption{
				withConfig(patURLs),
				withIsTerminal(true),
				withReadPassword("prompted-secret"),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextStepsMessage,
			wantStderr: "You can find your API credentials at: https://console.cloud.tigerdata.com/dashboard/settings\n\nEnter your secret key: \nValidating API key...\n",
			check:      checkStoredAPIKey("test-public-key:prompted-secret", "test-project-id"),
		},
		{
			name: "prompts for missing public key",
			args: []string{"auth", "login", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(patURLs),
				withIsTerminal(true),
				withStdin("prompted-public\n"),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextStepsMessage,
			wantStderr: "You can find your API credentials at: https://console.cloud.tigerdata.com/dashboard/settings\n\nEnter your public key: Validating API key...\n",
			check:      checkStoredAPIKey("prompted-public:test-secret-key", "test-project-id"),
		},
		{
			name: "empty prompted public key",
			args: []string{"auth", "login", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withIsTerminal(true),
				withStdin("\n"),
			},
			wantErr:    "both public key and secret key are required",
			wantStderr: "You can find your API credentials at: https://console.cloud.tigerdata.com/dashboard/settings\n\nEnter your public key: Error: both public key and secret key are required\n",
		},
		{
			name:       "API key validation failure",
			args:       []string{"auth", "login", "--public-key", "invalid-public", "--secret-key", "invalid-secret"},
			opts:       []runOption{withConfig(patURLs)},
			wantErr:    "API key validation failed: invalid credentials",
			wantStderr: "Validating API key...\nError: API key validation failed: invalid credentials\n",
			check:      checkNoStoredCredentials,
		},
		{
			name:       "stores credentials from flags",
			args:       []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts:       []runOption{withConfig(patURLs)},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextStepsMessage,
			wantStderr: "Validating API key...\n",
			check:      checkStoredAPIKey("test-public-key:test-secret-key", "test-project-id"),
		},
		{
			// An API key carries its own project, so a different --project-id is
			// an error rather than a switch.
			name: "project-id flag mismatching API key project",
			args: []string{"auth", "login",
				"--public-key", "test-public-key", "--secret-key", "test-secret-key",
				"--project-id", "some-other-project"},
			opts:       []runOption{withConfig(patURLs)},
			wantErr:    "API key is scoped to a different project than the one requested with --project-id",
			wantStderr: "Validating API key...\nError: API key is scoped to a different project than the one requested with --project-id\n",
			check: func(t *testing.T, result cmdResult) {
				checkExitCode(common.ExitInvalidParameters)(t, result)
				checkNoStoredCredentials(t, result)
			},
		},
		{
			name: "project-id flag matching API key project",
			args: []string{"auth", "login",
				"--public-key", "test-public-key", "--secret-key", "test-secret-key",
				"--project-id", "test-project-id"},
			opts:       []runOption{withConfig(patURLs)},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextStepsMessage,
			wantStderr: "Validating API key...\n",
			check:      checkStoredAPIKey("test-public-key:test-secret-key", "test-project-id"),
		},
		{
			name: "stores credentials from environment variables",
			args: []string{"auth", "login"},
			opts: []runOption{
				withConfig(patURLs),
				withEnv("TIGER_PUBLIC_KEY", "env-public-key"),
				withEnv("TIGER_SECRET_KEY", "env-secret-key"),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextStepsMessage,
			wantStderr: "Validating API key...\n",
			check:      checkStoredAPIKey("env-public-key:env-secret-key", "test-project-id"),
		},
	}
	runCmdTests(t, tests)

	// The OAuth flow's stderr embeds a random state and callback port, so these
	// stay bespoke: exact stdout, pattern-matched stderr.
	t.Run("oauth single project", func(t *testing.T) {
		server := startMockOAuthServer(t, []api.Project{
			{ID: "project-123", Name: "Test Project"},
		})

		result := runCommand(t, []string{"auth", "login"}, nil,
			withConfig(oauthURLs(server.URL)),
			withOpenBrowser(mockOpenBrowser(t)))

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "Successfully logged in (project: project-123)\n"+nextStepsMessage)
		assertOAuthStderr(t, result.stderr, server.URL, "")
		assertStoredOAuthCredentials(t, result.configDir, "project-123")
	})

	t.Run("oauth multiple projects with interactive selection", func(t *testing.T) {
		server := startMockOAuthServer(t, []api.Project{
			{ID: "project-123", Name: "Test Project 1"},
			{ID: "project-456", Name: "Test Project 2"},
			{ID: "project-789", Name: "Test Project 3"},
		})
		stubSelectProject(t, 2)

		result := runCommand(t, []string{"auth", "login"}, nil,
			withConfig(oauthURLs(server.URL)),
			withOpenBrowser(mockOpenBrowser(t)),
			withIsTerminal(true)) // the picker only runs on a TTY

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "Successfully logged in (project: project-789)\n"+nextStepsMessage)
		assertOAuthStderr(t, result.stderr, server.URL, "")
		assertStoredOAuthCredentials(t, result.configDir, "project-789")
	})

	t.Run("oauth multiple projects without TTY", func(t *testing.T) {
		server := startMockOAuthServer(t, []api.Project{
			{ID: "project-123", Name: "Test Project 1"},
			{ID: "project-456", Name: "Test Project 2"},
			{ID: "project-789", Name: "Test Project 3"},
		})

		result := runCommand(t, []string{"auth", "login"}, nil,
			withConfig(oauthURLs(server.URL)),
			withOpenBrowser(mockOpenBrowser(t)))

		wantErr := "failed to select project: TTY not detected - cannot select between 3 projects. Log in with API keys instead (--public-key, --secret-key)"
		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), wantErr)
		assertOAuthStderr(t, result.stderr, server.URL, regexp.QuoteMeta("Error: "+wantErr+"\n"))
		checkNoStoredCredentials(t, result)
	})

	t.Run("oauth project-id flag skips selection", func(t *testing.T) {
		server := startMockOAuthServer(t, []api.Project{
			{ID: "project-123", Name: "Test Project 1"},
			{ID: "project-456", Name: "Test Project 2"},
			{ID: "project-789", Name: "Test Project 3"},
		})

		// No TTY and no picker stub: --project-id skips the interactive
		// selection entirely.
		result := runCommand(t, []string{"auth", "login", "--project-id", "project-456"}, nil,
			withConfig(oauthURLs(server.URL)),
			withOpenBrowser(mockOpenBrowser(t)))

		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "Successfully logged in (project: project-456)\n"+nextStepsMessage)
		assertOAuthStderr(t, result.stderr, server.URL, "")
		assertStoredOAuthCredentials(t, result.configDir, "project-456")
	})

	t.Run("oauth project-id flag without access", func(t *testing.T) {
		server := startMockOAuthServer(t, []api.Project{
			{ID: "project-123", Name: "Test Project 1"},
		})

		result := runCommand(t, []string{"auth", "login", "--project-id", "project-999"}, nil,
			withConfig(oauthURLs(server.URL)),
			withOpenBrowser(mockOpenBrowser(t)))

		wantErr := "failed to select project: no access to the requested project"
		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), wantErr)
		// The requested ID is echoed on stderr, not in the error, and the exit
		// code must survive the "failed to select project" wrap.
		assertOAuthStderr(t, result.stderr, server.URL, regexp.QuoteMeta(
			"Project project-999 is not among your accessible projects\nError: "+wantErr+"\n"))
		checkExitCode(common.ExitInvalidParameters)(t, result)
		checkNoStoredCredentials(t, result)
	})

	// finishLogin clears the default service unless the login landed on the
	// same project it was set under: a service belongs to its project.
	t.Run("default service clearing", func(t *testing.T) {
		testCases := []struct {
			name          string
			prevProjectID string // "" = no stored credentials before login (e.g. after a logout)
			wantCleared   bool
		}{
			{name: "different project clears", prevProjectID: "project-old", wantCleared: true},
			{name: "unknown previous project clears", prevProjectID: "", wantCleared: true},
			{name: "same project keeps", prevProjectID: "project-123", wantCleared: false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				server := startMockOAuthServer(t, []api.Project{
					{ID: "project-123", Name: "Test Project"},
				})

				opts := []runOption{
					withConfig(oauthURLs(server.URL)),
					withConfig(map[string]any{"service_id": "svc-before"}),
					withOpenBrowser(mockOpenBrowser(t)),
				}
				if tc.prevProjectID != "" {
					opts = append(opts, withStoredCredentials(config.Credentials{
						OAuth: &oauth2.Token{
							AccessToken:  "prev-access-token",
							RefreshToken: "prev-refresh-token",
							Expiry:       time.Now().Add(time.Hour),
						},
						ProjectID: tc.prevProjectID,
					}))
				}

				result := runCommand(t, []string{"auth", "login"}, nil, opts...)
				if result.err != nil {
					t.Fatalf("unexpected error: %v", result.err)
				}

				assertOutput(t, result.stdout, "Successfully logged in (project: project-123)\n"+nextStepsMessage)
				suffix := ""
				wantServiceID := "svc-before"
				if tc.wantCleared {
					suffix = regexp.QuoteMeta("Cleared default service (config key service_id): it belonged to the previous project\n")
					wantServiceID = ""
				}
				assertOAuthStderr(t, result.stderr, server.URL, suffix)
				checkDefaultService(wantServiceID)(t, result)
			})
		}
	})

	t.Run("oauth no accessible projects", func(t *testing.T) {
		server := startMockOAuthServer(t, []api.Project{})

		result := runCommand(t, []string{"auth", "login"}, nil,
			withConfig(oauthURLs(server.URL)),
			withOpenBrowser(mockOpenBrowser(t)))

		wantErr := "failed to select project: user has no accessible projects"
		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertOutput(t, result.err.Error(), wantErr)
		assertOAuthStderr(t, result.stderr, server.URL, regexp.QuoteMeta("Error: "+wantErr+"\n"))
		checkNoStoredCredentials(t, result)
	})
}

// TestOAuthRefreshPersistsExpiry verifies that when an expired OAuth token is
// refreshed, the rotated token is persisted with a non-zero Expiry derived from
// the standard `expires_in` returned by the gateway. This runs below the
// command layer (runCommand injects a mock client, so the real refreshing
// client is never built there).
func TestOAuthRefreshPersistsExpiry(t *testing.T) {
	config.SetTestServiceName(t)

	// The mock server backs the refresh_token grant (returns expires_in=3600).
	server := startMockOAuthServer(t, nil)
	cfg, err := config.UseTestConfig(t.TempDir(), map[string]any{
		"gateway_url": server.URL,
		"api_url":     server.URL,
	})
	if err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	// An already-expired OAuth token that still has a valid refresh token.
	expired := &oauth2.Token{
		AccessToken:  "stale-access-token",
		RefreshToken: "mock-refresh-token-67890",
		Expiry:       time.Now().Add(-time.Hour),
	}
	if err := cfg.StoreOAuthCredentials(expired, "project-789"); err != nil {
		t.Fatalf("failed to store oauth credentials: %v", err)
	}

	stored, err := cfg.GetStoredCredentials()
	if err != nil {
		t.Fatalf("failed to load stored credentials: %v", err)
	}
	client, err := api.NewTigerClientForCredentials(cfg, stored)
	if err != nil {
		t.Fatalf("failed to build client: %v", err)
	}

	// Any request makes the oauth2 transport mint a token first; since the
	// stored token is expired, that triggers a refresh + persist. The response
	// status itself is irrelevant — we only care about the persisted token.
	if _, err := client.GetAuthInfoWithResponse(t.Context()); err != nil {
		t.Fatalf("request failed: %v", err)
	}

	reloaded, err := cfg.GetStoredCredentials()
	if err != nil {
		t.Fatalf("failed to reload credentials: %v", err)
	}
	if reloaded.OAuth == nil {
		t.Fatal("expected OAuth credentials to remain stored after refresh")
	}
	if reloaded.OAuth.AccessToken != "mock-access-token-12345" {
		t.Fatalf("expected token to be refreshed, got access token %q", reloaded.OAuth.AccessToken)
	}
	assertExpiresInAbout(t, reloaded.OAuth.Expiry)
}

// startFakeAuthInfoServer backs common.ValidateAPIKey's GET /auth/info call for
// the PAT login path. Keys arrive as HTTP basic auth; a public key starting
// with "invalid" is rejected with a 401.
func startFakeAuthInfoServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/info", func(w http.ResponseWriter, r *http.Request) {
		publicKey, _, ok := r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		if !ok || strings.HasPrefix(publicKey, "invalid") {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"message": "invalid credentials"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"type": "apiKey",
			"api_key": map[string]any{
				"name":         "Test Credentials",
				"public_key":   publicKey,
				"created":      "2025-01-01T00:00:00Z",
				"project":      map[string]any{"id": "test-project-id", "name": "Test Project", "plan_type": "free"},
				"issuing_user": map[string]any{"id": "user-123", "name": "Test User", "email": "test@example.com"},
			},
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// oauthURLs points every endpoint the OAuth login flow touches at the mock
// server: the authorize URL (console), the token endpoint (gateway), and the
// project listing (API).
func oauthURLs(serverURL string) map[string]any {
	return map[string]any{
		"console_url": serverURL,
		"gateway_url": serverURL,
		"api_url":     serverURL,
	}
}

// mockOpenBrowser simulates the user completing browser authentication: it
// validates the PKCE parameters in the auth URL, then hits the local callback
// with a fake authorization code.
func mockOpenBrowser(t *testing.T) func(string) error {
	t.Helper()
	return func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		q := parsed.Query()
		for _, param := range []string{"client_id", "code_challenge", "redirect_uri", "state"} {
			if q.Get(param) == "" {
				return fmt.Errorf("missing %s in auth URL", param)
			}
		}
		if q.Get("response_type") != "code" {
			return fmt.Errorf("unexpected response_type %q in auth URL", q.Get("response_type"))
		}
		if q.Get("code_challenge_method") != "S256" {
			return fmt.Errorf("unexpected code_challenge_method %q in auth URL", q.Get("code_challenge_method"))
		}

		callbackURL := fmt.Sprintf("%s?code=test-auth-code&state=%s", q.Get("redirect_uri"), q.Get("state"))
		resp, err := http.Get(callbackURL)
		if err != nil {
			return fmt.Errorf("callback request failed: %w", err)
		}
		return resp.Body.Close()
	}
}

// stubSelectProject makes the interactive project picker select the project at
// the given index for the duration of the test.
func stubSelectProject(t *testing.T, index int) {
	t.Helper()
	original := selectProjectInteractively
	selectProjectInteractively = func(_ *cobra.Command, projects []api.Project) (string, error) {
		return projects[index].ID, nil
	}
	t.Cleanup(func() { selectProjectInteractively = original })
}

// assertOAuthStderr matches the OAuth flow's stderr: the auth URL line (which
// embeds a random state and callback port) plus the browser-open notice,
// followed by suffix (an already-quoted pattern, e.g. an Error line).
func assertOAuthStderr(t *testing.T, stderr, serverURL, suffix string) {
	t.Helper()
	pattern := fmt.Sprintf(
		`^Auth URL is: %s/oauth/authorize\?client_id=%s&code_challenge=[A-Za-z0-9_-]+&code_challenge_method=S256&redirect_uri=http%%3A%%2F%%2Flocalhost%%3A\d+%%2Fcallback&response_type=code&state=[A-Za-z0-9_-]+\nOpening browser for authentication\.\.\.\n%s$`,
		regexp.QuoteMeta(serverURL), config.TigerCLIClientID, suffix)
	matched, err := regexp.MatchString(pattern, stderr)
	if err != nil {
		t.Fatalf("regex compilation failed: %v", err)
	}
	if !matched {
		t.Errorf("stderr doesn't match expected pattern.\npattern: %s\nstderr: %q", pattern, stderr)
	}
}

// assertStoredOAuthCredentials checks that the mock server's canned OAuth token
// was stored for the given project.
func assertStoredOAuthCredentials(t *testing.T, configDir, projectID string) {
	t.Helper()
	stored, err := readStoredCredentials(t, configDir)
	if err != nil {
		t.Fatalf("failed to get stored credentials: %v", err)
	}
	if stored.OAuth == nil {
		t.Fatalf("expected OAuth credentials, got: %+v", stored)
	}
	if stored.OAuth.AccessToken != "mock-access-token-12345" {
		t.Errorf("stored access token = %q, want %q", stored.OAuth.AccessToken, "mock-access-token-12345")
	}
	if stored.OAuth.RefreshToken != "mock-refresh-token-67890" {
		t.Errorf("stored refresh token = %q, want %q", stored.OAuth.RefreshToken, "mock-refresh-token-67890")
	}
	assertExpiresInAbout(t, stored.OAuth.Expiry)
	if stored.ProjectID != projectID {
		t.Errorf("stored project ID = %q, want %q", stored.ProjectID, projectID)
	}
}

// assertExpiresInAbout checks that the token Expiry was derived from the
// standard `expires_in` (the mock returns 3600s), allowing slack for elapsed
// test time.
func assertExpiresInAbout(t *testing.T, expiry time.Time) {
	t.Helper()
	d := time.Until(expiry)
	if d < 3540*time.Second || d > 3600*time.Second {
		t.Errorf("expected expiry ~3600s from now (from expires_in=3600), got %v (in %v)", expiry, d)
	}
}

// checkStoredAPIKey returns a check func asserting that the given PAT
// credentials were stored.
func checkStoredAPIKey(apiKey, projectID string) func(t *testing.T, result cmdResult) {
	return func(t *testing.T, result cmdResult) {
		t.Helper()
		creds, err := readStoredCredentials(t, result.configDir)
		if err != nil {
			t.Fatalf("failed to get stored credentials: %v", err)
		}
		if creds.APIKey != apiKey {
			t.Errorf("stored API key = %q, want %q", creds.APIKey, apiKey)
		}
		if creds.ProjectID != projectID {
			t.Errorf("stored project ID = %q, want %q", creds.ProjectID, projectID)
		}
	}
}
