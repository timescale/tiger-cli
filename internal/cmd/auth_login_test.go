package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestAuthLoginCmd(t *testing.T) {
	// Backs the PAT path's key validation (/auth/info): keys with an "invalid-"
	// public key are rejected, everything else is accepted with the project ID
	// "test-project-id".
	authInfoServer := startMockAuthInfoServer(t)
	// read_only is pinned so the post-login prompt never fires in these cases:
	// a stored value is what makes offerProdProtection skip it. The prompt has
	// its own case group at the end of the table.
	patURLs := map[string]any{"api_url": authInfoServer.URL, "read_only": "off"}

	// OAuth flow cases. The flow's stderr embeds a random state and callback
	// port, so it is matched with matchOAuthStderr rather than exactly.
	singleProject := startMockOAuthServer(t, []api.Project{
		{ID: "project-123", Name: "Test Project"},
	})
	multiProject := startMockOAuthServer(t, []api.Project{
		{ID: "project-123", Name: "Test Project 1"},
		{ID: "project-456", Name: "Test Project 2"},
		{ID: "project-789", Name: "Test Project 3"},
	})
	noProjects := startMockOAuthServer(t, []api.Project{})
	staleStateErr := "failed to authenticate via OAuth: invalid state parameter"
	oauthOpts := func(serverURL string, extra ...runOption) []runOption {
		return append([]runOption{
			withConfig(oauthURLs(serverURL)),
			withOpenBrowser(mockOpenBrowser(t)),
		}, extra...)
	}

	multiNoTTYErr := "failed to select project: TTY not detected - cannot select between 3 projects. Log in with API keys instead (--public-key, --secret-key)"
	noAccessErr := "failed to select project: no access to the requested project"
	noProjectsErr := "failed to select project: user has no accessible projects"

	runCmdTests(t, []cmdTest{
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
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "You can find your API credentials at: https://console.cloud.tigerdata.com/dashboard/settings\n\nEnter your secret key: \nValidating API key...\n",
			checks:     []checkFunc{checkStoredAPIKey("test-public-key:prompted-secret", "test-project-id")},
		},
		{
			name: "prompts for missing public key",
			args: []string{"auth", "login", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(patURLs),
				withIsTerminal(true),
				withStdin("prompted-public\n"),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "You can find your API credentials at: https://console.cloud.tigerdata.com/dashboard/settings\n\nEnter your public key: Validating API key...\n",
			checks:     []checkFunc{checkStoredAPIKey("prompted-public:test-secret-key", "test-project-id")},
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
			checks:     []checkFunc{checkNoStoredCredentials},
		},
		{
			name:       "stores credentials from flags",
			args:       []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts:       []runOption{withConfig(patURLs)},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkStoredAPIKey("test-public-key:test-secret-key", "test-project-id")},
		},
		{
			// An API key carries its own project, so a different --project-id is
			// an error rather than a switch.
			name: "project-id flag mismatching API key project",
			args: []string{
				"auth", "login",
				"--public-key", "test-public-key", "--secret-key", "test-secret-key",
				"--project-id", "some-other-project",
			},
			opts:       []runOption{withConfig(patURLs)},
			wantErr:    "API key is scoped to a different project than the one requested with --project-id",
			wantStderr: "Validating API key...\nError: API key is scoped to a different project than the one requested with --project-id\n",
			checks: []checkFunc{
				checkExitCode(common.ExitInvalidParameters),
				checkNoStoredCredentials,
			},
		},
		{
			name: "project-id flag matching API key project",
			args: []string{
				"auth", "login",
				"--public-key", "test-public-key", "--secret-key", "test-secret-key",
				"--project-id", "test-project-id",
			},
			opts:       []runOption{withConfig(patURLs)},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkStoredAPIKey("test-public-key:test-secret-key", "test-project-id")},
		},
		{
			name: "stores credentials from environment variables",
			args: []string{"auth", "login"},
			opts: []runOption{
				withConfig(patURLs),
				withEnv("TIGER_PUBLIC_KEY", "env-public-key"),
				withEnv("TIGER_SECRET_KEY", "env-secret-key"),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkStoredAPIKey("env-public-key:env-secret-key", "test-project-id")},
		},
		{
			name:       "oauth single project",
			args:       []string{"auth", "login"},
			opts:       oauthOpts(singleProject.URL),
			wantStdout: "Successfully logged in (project: project-123)\n" + nextSteps(true),
			wantStderr: matchOAuthStderr(singleProject.URL, ""),
			checks:     []checkFunc{checkStoredOAuthCredentials("project-123")},
		},
		{
			// Only the first callback can be delivered, and the ones after it
			// must not park a handler on the channel: the server never shuts
			// down while one is open, and the command never returns.
			name:       "repeated callbacks don't block the login",
			args:       []string{"auth", "login"},
			opts:       []runOption{withConfig(oauthURLs(singleProject.URL)), withOpenBrowser(repeatCallback(t, 3))},
			wantStdout: "Successfully logged in (project: project-123)\n" + nextSteps(true),
			wantStderr: matchOAuthStderr(singleProject.URL, ""),
			checks:     []checkFunc{checkStoredOAuthCredentials("project-123")},
		},
		{
			// A callback that isn't ours -- a stale tab from an earlier login --
			// is refused, and the login ends rather than accepting its code.
			name:       "callback with a stale state is refused",
			args:       []string{"auth", "login"},
			opts:       []runOption{withConfig(oauthURLs(singleProject.URL)), withOpenBrowser(callbackWithBadState(t))},
			wantErr:    staleStateErr,
			wantStderr: matchOAuthStderr(singleProject.URL, regexp.QuoteMeta("Error: "+staleStateErr+"\n")),
			checks:     []checkFunc{checkNoStoredCredentials},
		},
		{
			// The picker only runs on a TTY.
			name:       "oauth multiple projects with interactive selection",
			args:       []string{"auth", "login"},
			opts:       oauthOpts(multiProject.URL, withIsTerminal(true), withSelectProject(2)),
			wantStdout: "Successfully logged in (project: project-789)\n" + nextSteps(true),
			wantStderr: matchOAuthStderr(multiProject.URL, ""),
			checks:     []checkFunc{checkStoredOAuthCredentials("project-789")},
		},
		{
			name:       "oauth multiple projects without TTY",
			args:       []string{"auth", "login"},
			opts:       oauthOpts(multiProject.URL),
			wantErr:    multiNoTTYErr,
			wantStderr: matchOAuthStderr(multiProject.URL, regexp.QuoteMeta("Error: "+multiNoTTYErr+"\n")),
			checks:     []checkFunc{checkNoStoredCredentials},
		},
		{
			// No TTY and no picker stub: --project-id skips the interactive
			// selection entirely.
			name:       "oauth project-id flag skips selection",
			args:       []string{"auth", "login", "--project-id", "project-456"},
			opts:       oauthOpts(multiProject.URL),
			wantStdout: "Successfully logged in (project: project-456)\n" + nextSteps(true),
			wantStderr: matchOAuthStderr(multiProject.URL, ""),
			checks:     []checkFunc{checkStoredOAuthCredentials("project-456")},
		},
		{
			// The requested ID is echoed on stderr, not in the error, and the
			// exit code must survive the "failed to select project" wrap.
			name:    "oauth project-id flag without access",
			args:    []string{"auth", "login", "--project-id", "project-999"},
			opts:    oauthOpts(singleProject.URL),
			wantErr: noAccessErr,
			wantStderr: matchOAuthStderr(singleProject.URL, regexp.QuoteMeta(
				"Project project-999 is not among your accessible projects\nError: "+noAccessErr+"\n")),
			checks: []checkFunc{
				checkExitCode(common.ExitInvalidParameters),
				checkNoStoredCredentials,
			},
		},
		{
			name:       "oauth no accessible projects",
			args:       []string{"auth", "login"},
			opts:       oauthOpts(noProjects.URL),
			wantErr:    noProjectsErr,
			wantStderr: matchOAuthStderr(noProjects.URL, regexp.QuoteMeta("Error: "+noProjectsErr+"\n")),
			checks:     []checkFunc{checkNoStoredCredentials},
		},
		{
			// This case and the two below cover finishLogin's default-service
			// handling: it clears the default service unless the login landed
			// on the same project it was set under, since a service belongs to
			// its project.
			name: "login on different project clears default service",
			args: []string{"auth", "login"},
			opts: oauthOpts(singleProject.URL,
				withConfig(map[string]any{"service_id": "svc-before"}),
				withStoredCredentials(prevOAuthCredentials("project-old"))),
			wantStdout: "Successfully logged in (project: project-123)\n" + nextSteps(true),
			wantStderr: matchOAuthStderr(singleProject.URL, regexp.QuoteMeta(
				"Cleared default service (config key service_id): it belonged to the previous project\n")),
			checks: []checkFunc{checkDefaultService("")},
		},
		{
			// No stored credentials before the login, e.g. after a logout: the
			// default service may belong to any project, so it is cleared too.
			name: "login after logout clears default service",
			args: []string{"auth", "login"},
			opts: oauthOpts(singleProject.URL,
				withConfig(map[string]any{"service_id": "svc-before"})),
			wantStdout: "Successfully logged in (project: project-123)\n" + nextSteps(true),
			wantStderr: matchOAuthStderr(singleProject.URL, regexp.QuoteMeta(
				"Cleared default service (config key service_id): it belonged to the previous project\n")),
			checks: []checkFunc{checkDefaultService("")},
		},
		{
			name: "login on same project keeps default service",
			args: []string{"auth", "login"},
			opts: oauthOpts(singleProject.URL,
				withConfig(map[string]any{"service_id": "svc-before"}),
				withStoredCredentials(prevOAuthCredentials("project-123"))),
			wantStdout: "Successfully logged in (project: project-123)\n" + nextSteps(true),
			wantStderr: matchOAuthStderr(singleProject.URL, ""),
			checks:     []checkFunc{checkDefaultService("svc-before")},
		},

		// Post-login read-only prompt cases. The menu itself is a Bubble Tea
		// program (covered by TestReadOnlyModel_KeySelection); here it is
		// stubbed via withSelectReadOnlyMode. The skip cases stub an answer
		// that would be stored if the menu were wrongly shown, so the absence
		// of a stored value is what proves it wasn't.
		{
			name: "read-only prompt stores prod",
			args: []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": authInfoServer.URL}),
				withIsTerminal(true),
				withSelectReadOnlyMode(config.ReadOnlyProd, true),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\nServices tagged PROD are now protected from writes.\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": authInfoServer.URL, "read_only": "prod"})},
		},
		{
			name: "read-only prompt stores all",
			args: []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": authInfoServer.URL}),
				withIsTerminal(true),
				withSelectReadOnlyMode(config.ReadOnlyAll, true),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\nAll services are now protected from writes.\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": authInfoServer.URL, "read_only": "all"})},
		},
		{
			// "off" is a real answer: it is stored (so the next login won't
			// ask again) but confirms nothing.
			name: "read-only prompt stores off silently",
			args: []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": authInfoServer.URL}),
				withIsTerminal(true),
				withSelectReadOnlyMode(config.ReadOnlyOff, true),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": authInfoServer.URL, "read_only": "off"})},
		},
		{
			// Dismissing is not an answer: nothing is recorded, the next login
			// asks again, and nextSteps says how to set it by hand.
			name: "read-only prompt dismissed records nothing",
			args: []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": authInfoServer.URL}),
				withIsTerminal(true),
				withSelectReadOnlyMode("", false),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(false),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": authInfoServer.URL})},
		},
		{
			name: "read-only prompt skipped without TTY",
			args: []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": authInfoServer.URL}),
				withSelectReadOnlyMode(config.ReadOnlyProd, true),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(false),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": authInfoServer.URL})},
		},
		{
			name: "read-only prompt skipped when already declined",
			args: []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": authInfoServer.URL, "read_only": "off"}),
				withIsTerminal(true),
				withSelectReadOnlyMode(config.ReadOnlyProd, true),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": authInfoServer.URL, "read_only": "off"})},
		},
		{
			name: "read-only prompt skipped when already opted in",
			args: []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": authInfoServer.URL, "read_only": "prod"}),
				withIsTerminal(true),
				withSelectReadOnlyMode(config.ReadOnlyOff, true),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": authInfoServer.URL, "read_only": "prod"})},
		},
		{
			name: "read-only prompt skipped when already set to all",
			args: []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": authInfoServer.URL, "read_only": "all"}),
				withIsTerminal(true),
				withSelectReadOnlyMode(config.ReadOnlyProd, true),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": authInfoServer.URL, "read_only": "all"})},
		},
		{
			// TIGER_READ_ONLY outranks the config file, so a mode chosen at
			// the prompt would be a lie: nothing is asked or stored.
			name: "read-only prompt skipped when env var set",
			args: []string{"auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key"},
			opts: []runOption{
				withConfig(map[string]any{"api_url": authInfoServer.URL}),
				withEnv("TIGER_READ_ONLY", "all"),
				withIsTerminal(true),
				withSelectReadOnlyMode(config.ReadOnlyProd, true),
			},
			wantStdout: "Successfully logged in (project: test-project-id)\n" + nextSteps(true),
			wantStderr: "Validating API key...\n",
			checks:     []checkFunc{checkConfigFile(map[string]any{"api_url": authInfoServer.URL})},
		},
	})
}

// TestOAuthRefreshPersistsExpiry verifies that when an expired OAuth token is
// refreshed, the rotated token is persisted with a non-zero Expiry derived from
// the standard `expires_in` returned by the gateway. This runs below the
// command layer (runCommand injects a mock client, so the real refreshing
// client is never built there).
func TestOAuthRefreshPersistsExpiry(t *testing.T) {
	keyring.MockInit()

	// The mock server backs the refresh_token grant (returns expires_in=3600).
	server := startMockOAuthServer(t, nil)
	cfg := &config.Config{ConfigDir: t.TempDir(), GatewayURL: server.URL, APIURL: server.URL}

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

// startMockAuthInfoServer backs common.ValidateAPIKey's GET /auth/info call for
// the PAT login path. Keys arrive as HTTP basic auth; a public key starting
// with "invalid" is rejected with a 401.
func startMockAuthInfoServer(t *testing.T) *httptest.Server {
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
		// See patURLs: pinned so the post-login prompt stays out of these cases.
		"read_only": "off",
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

// repeatCallback completes the same successful callback n times, as a browser
// reloading the redirect does. Only the first can be delivered; the rest must
// not leave a handler parked on the channel.
func repeatCallback(t *testing.T, n int) func(string) error {
	t.Helper()
	complete := mockOpenBrowser(t)
	return func(authURL string) error {
		for range n {
			if err := complete(authURL); err != nil {
				return err
			}
		}
		return nil
	}
}

// callbackWithBadState answers the auth URL with a callback carrying the wrong
// state, as a stale tab from an earlier login does.
func callbackWithBadState(t *testing.T) func(string) error {
	t.Helper()
	return func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		resp, err := http.Get(parsed.Query().Get("redirect_uri") + "?code=test-auth-code&state=stale")
		if err != nil {
			return fmt.Errorf("callback request failed: %w", err)
		}
		return resp.Body.Close()
	}
}

// withSelectReadOnlyMode stubs the post-login read-only menu to return the
// given answer (chose=false means dismissed without choosing).
func withSelectReadOnlyMode(mode config.ReadOnlyMode, chose bool) runOption {
	return withSetup(func(t *testing.T) {
		original := selectReadOnlyMode
		selectReadOnlyMode = func(*cobra.Command) (config.ReadOnlyMode, bool) { return mode, chose }
		t.Cleanup(func() { selectReadOnlyMode = original })
	})
}

// withSelectProject makes the interactive project picker select the project at
// the given index for the duration of the test.
func withSelectProject(index int) runOption {
	return withSetup(func(t *testing.T) {
		original := selectProjectInteractively
		selectProjectInteractively = func(_ *cobra.Command, projects []api.Project) (string, error) {
			return projects[index].ID, nil
		}
		t.Cleanup(func() { selectProjectInteractively = original })
	})
}

// prevOAuthCredentials builds a stored OAuth login for projectID, as left
// behind by an earlier `tiger auth login`.
func prevOAuthCredentials(projectID string) config.Credentials {
	return config.Credentials{
		OAuth: &oauth2.Token{
			AccessToken:  "prev-access-token",
			RefreshToken: "prev-refresh-token",
			Expiry:       time.Now().Add(time.Hour),
		},
		ProjectID: projectID,
	}
}

// matchOAuthStderr matches the OAuth flow's stderr: the auth URL line (which
// embeds a random state and callback port) plus the browser-open notice,
// followed by suffix (an already-quoted pattern, e.g. an Error line).
func matchOAuthStderr(serverURL, suffix string) matcher {
	return matchRegexp(fmt.Sprintf(
		`Auth URL is: %s/oauth/authorize\?client_id=%s&code_challenge=[A-Za-z0-9_-]+&code_challenge_method=S256&redirect_uri=http%%3A%%2F%%2Flocalhost%%3A\d+%%2Fcallback&response_type=code&state=[A-Za-z0-9_-]+\nOpening browser for authentication\.\.\.\n%s`,
		regexp.QuoteMeta(serverURL), config.TigerCLIClientID, suffix))
}

// checkStoredOAuthCredentials returns a check asserting that the mock server's
// canned OAuth token was stored for the given project.
func checkStoredOAuthCredentials(projectID string) checkFunc {
	return func(t *testing.T, result cmdResult) {
		t.Helper()
		checkStoredOAuthToken(t, result.configDir, projectID)
	}
}

func checkStoredOAuthToken(t *testing.T, configDir, projectID string) {
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
func checkStoredAPIKey(apiKey, projectID string) checkFunc {
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

// TestReadOnlyModel_KeySelection checks that every mode is reachable and that
// dismissing leaves chosen empty, which is what keeps a quit from being recorded
// as picking whichever mode the cursor rested on. Helper-level because the menu
// is a Bubble Tea model that needs a real TTY to run through the command; the
// command tests stub it via withSelectReadOnlyMode (same precedent as
// TestConnectTargetModel).
func TestReadOnlyModel_KeySelection(t *testing.T) {
	// Ctrl+C is {Code: 'c', Mod: tea.ModCtrl}; the raw control byte {Code: 3}
	// stringifies to "\x03" and would match nothing.
	cases := []struct {
		name string
		keys []tea.KeyPressMsg
		want config.ReadOnlyMode // "" = dismissed
	}{
		{"enter takes the recommended default", []tea.KeyPressMsg{{Code: tea.KeyEnter}}, config.ReadOnlyProd},
		{"space takes it too", []tea.KeyPressMsg{{Code: tea.KeySpace}}, config.ReadOnlyProd},
		{"'2' selects all", []tea.KeyPressMsg{{Code: '2'}}, config.ReadOnlyAll},
		{"'3' selects off", []tea.KeyPressMsg{{Code: '3'}}, config.ReadOnlyOff},
		{"down then enter selects all", []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyEnter}}, config.ReadOnlyAll},
		{"q dismisses", []tea.KeyPressMsg{{Code: 'q'}}, ""},
		{"esc dismisses", []tea.KeyPressMsg{{Code: tea.KeyEsc}}, ""},
		{"ctrl+c dismisses", []tea.KeyPressMsg{{Code: 'c', Mod: tea.ModCtrl}}, ""},
		{"cursor can't run past the end", []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyEnter}}, config.ReadOnlyOff},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m tea.Model = readOnlyModel{}
			for _, key := range tc.keys {
				m, _ = m.Update(key)
			}
			if got := m.(readOnlyModel).chosen; got != tc.want {
				t.Errorf("chosen = %q, want %q", got, tc.want)
			}
		})
	}
}

// deviceReply is one canned token-endpoint response for a device poll.
type deviceReply struct {
	status int
	// body is marshalled as JSON; nil sends a short text body instead.
	body map[string]any
}

var (
	devicePending = deviceReply{
		status: http.StatusBadRequest,
		body: map[string]any{
			"error":             "authorization_pending",
			"error_description": "The authorization request is still pending.",
		},
	}
	deviceExpired = deviceReply{
		status: http.StatusBadRequest,
		body: map[string]any{
			"error":             "expired_token",
			"error_description": "The device code has expired.",
		},
	}
	// FusionAuth's answer for a code already redeemed, or never issued.
	deviceConsumed = deviceReply{
		status: http.StatusBadRequest,
		body: map[string]any{
			"error":             "invalid_request",
			"error_description": "invalid_device_code",
		},
	}
	// A 502 with a non-OAuth body: a 5xx is terminal whether or not an OAuth
	// error came with it.
	deviceBadGateway = deviceReply{status: http.StatusBadGateway}
	// A 429, the shape that is neither a verdict nor a 5xx, so it is retried.
	deviceRateLimited = deviceReply{status: http.StatusTooManyRequests}
	// An OAuth-shaped 500: the error code describes a failure to answer, not a
	// verdict on the device flow.
	deviceServerError = deviceReply{
		status: http.StatusInternalServerError,
		body: map[string]any{
			"error":             "server_error",
			"error_description": "Failed to exchange device code",
		},
	}
	deviceTokens = deviceReply{
		status: http.StatusOK,
		body: map[string]any{
			"access_token":  "mock-access-token-12345",
			"refresh_token": "mock-refresh-token-67890",
			"expires_in":    3600,
		},
	}
)

// deviceVerificationURI is what the mock code endpoint reports.
const deviceVerificationURI = "https://console.example.com/oauth/device"

// deviceInstructions is the device flow's stderr.
const deviceInstructions = "\nTo authenticate, visit: " + deviceVerificationURI + "\n" +
	"and enter code: K7QP3XVR\n\n" +
	"Waiting for authorization (this can take a few seconds after you enter the code)...\n"

// deviceRetryNotice is printed before re-entering polling after a 429.
const deviceRetryNotice = "Authorization check failed, retrying: " +
	"oauth2: cannot fetch token: 429 Too Many Requests\nResponse: too many requests\n\n"

// startMockDeviceServer backs the device flow: the code endpoint, the token
// endpoint (answering polls from replies in order, the last repeating), and
// the project listing. expiresIn becomes DeviceAccessToken's deadline, so a
// small value exercises a code that runs out on its own.
func startMockDeviceServer(t *testing.T, projects []api.Project, expiresIn int, replies ...deviceReply) *httptest.Server {
	t.Helper()

	var mu sync.Mutex
	polls := 0

	mux := http.NewServeMux()

	mux.HandleFunc("POST /idp/external/cli/device/code", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			return
		}
		if got := r.FormValue("client_id"); got != config.TigerCLIClientID {
			t.Errorf("device code client_id = %q, want %q", got, config.TigerCLIClientID)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "mock-device-code",
			"user_code":        "K7QP3XVR",
			"verification_uri": deviceVerificationURI,
			"expires_in":       expiresIn,
			"interval":         1,
		})
	})

	mux.HandleFunc("POST /idp/external/cli/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			return
		}
		if got := r.FormValue("grant_type"); got != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("token grant_type = %q, want the device code grant", got)
			http.Error(w, "unsupported grant", http.StatusBadRequest)
			return
		}
		// The redeemable half must be presented, and the client identified.
		if got := r.FormValue("device_code"); got != "mock-device-code" {
			t.Errorf("device_code = %q, want %q", got, "mock-device-code")
		}
		if got := r.FormValue("client_id"); got != config.TigerCLIClientID {
			t.Errorf("token client_id = %q, want %q", got, config.TigerCLIClientID)
		}
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "tiger-cli/") {
			t.Errorf("device poll User-Agent = %q, want \"tiger-cli/\" prefix", ua)
		}

		mu.Lock()
		reply := replies[min(polls, len(replies)-1)]
		polls++
		mu.Unlock()

		if reply.body == nil {
			http.Error(w, strings.ToLower(http.StatusText(reply.status)), reply.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		json.NewEncoder(w).Encode(reply.body)
	})

	mux.HandleFunc("GET /projects", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projects)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// withDeviceRetryDelay shrinks the pause between polling retries so tests don't
// sleep out its production duration. The 1s poll interval is x/oauth2's own floor.
func withDeviceRetryDelay(delay time.Duration) runOption {
	return withSetup(func(t *testing.T) {
		original := deviceRetryDelay
		deviceRetryDelay = delay
		t.Cleanup(func() { deviceRetryDelay = original })
	})
}

// withDeviceCodeTTL shrinks the bound the CLI supplies when the gateway omits
// expires_in, so a test doesn't sit out its production duration.
func withDeviceCodeTTL(ttl time.Duration) runOption {
	return withSetup(func(t *testing.T) {
		original := defaultDeviceCodeTTL
		defaultDeviceCodeTTL = ttl
		t.Cleanup(func() { defaultDeviceCodeTTL = original })
	})
}

// withNoBrowserOpen fails the test if the login tries to open a browser.
func withNoBrowserOpen() runOption {
	return withSetup(func(t *testing.T) {
		original := openBrowser
		openBrowser = func(url string) error {
			t.Errorf("unexpected browser open: %s", url)
			return nil
		}
		t.Cleanup(func() { openBrowser = original })
	})
}

// TestAuthLoginDeviceFlow covers --headless, the fallback from a browser that
// won't open, and each ending the server can hand back. Polls cost 1s each.
func TestAuthLoginDeviceFlow(t *testing.T) {
	projects := []api.Project{{ID: "project-123", Name: "Test Project"}}

	const ttl = 900 // seconds, as configured on the tenant

	success := startMockDeviceServer(t, projects, ttl, deviceTokens)
	pendingThenTokens := startMockDeviceServer(t, projects, ttl, devicePending, deviceTokens)
	expired := startMockDeviceServer(t, projects, ttl, deviceExpired)
	consumed := startMockDeviceServer(t, projects, ttl, deviceConsumed)
	throttled := startMockDeviceServer(t, projects, ttl, deviceRateLimited, deviceTokens)
	badGateway := startMockDeviceServer(t, projects, ttl, deviceBadGateway)
	unavailable := startMockDeviceServer(t, projects, ttl, deviceServerError)
	// A code too short-lived to use: DeviceAccessToken hits its own deadline
	// first. Nonzero matters -- expires_in=0 takes the fallback deadline below.
	stale := startMockDeviceServer(t, projects, 1, devicePending)
	// No expires_in at all, so x/oauth2 has no deadline of its own to install.
	noExpiry := startMockDeviceServer(t, projects, 0, devicePending)
	// A gateway that can't issue a code pair at all.
	noCode := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(noCode.Close)

	loggedIn := "Successfully logged in (project: project-123)\n" + nextSteps(true)

	// Every failure below reaches the user through loginWithOAuth's wrapper.
	const authFailed = "failed to authenticate via OAuth: "
	expiredErr := authFailed + "the code expired before it was authorized - run 'tiger auth login' again for a new code"
	consumedErr := authFailed + "the code is no longer valid - run 'tiger auth login' again for a new code"
	unavailableErr := authFailed + "the authorization service is unavailable - try again in a moment"
	noCodeErr := authFailed + "failed to start device authorization: " +
		"oauth2: cannot fetch token: 404 Not Found\nResponse: 404 page not found\n"

	runCmdTests(t, []cmdTest{
		{
			// No browser is touched, and the code and URL are the whole output.
			name:       "headless prints the code and stores the session",
			args:       []string{"auth", "login", "--headless"},
			opts:       []runOption{withConfig(oauthURLs(success.URL)), withNoBrowserOpen()},
			wantStdout: loggedIn,
			wantStderr: deviceInstructions,
			checks:     []checkFunc{checkStoredOAuthCredentials("project-123")},
		},
		{
			// authorization_pending is a flow state: x/oauth2 polls on, silently.
			name:       "headless keeps polling while authorization is pending",
			args:       []string{"auth", "login", "--headless"},
			opts:       []runOption{withConfig(oauthURLs(pendingThenTokens.URL)), withNoBrowserOpen()},
			wantStdout: loggedIn,
			wantStderr: deviceInstructions,
			checks:     []checkFunc{checkStoredOAuthCredentials("project-123")},
		},
		{
			// No code pair, no flow -- and still an authentication failure, so
			// callers branching on the exit code classify it with the rest.
			name:       "device code request failure ends the login",
			args:       []string{"auth", "login", "--headless"},
			opts:       []runOption{withConfig(oauthURLs(noCode.URL)), withNoBrowserOpen()},
			wantErr:    noCodeErr,
			wantStderr: "Error: " + noCodeErr + "\n",
			checks: []checkFunc{
				checkExitCode(common.ExitAuthenticationError),
				checkNoStoredCredentials,
			},
		},
		{
			name:       "expired code ends the login with guidance",
			args:       []string{"auth", "login", "--headless"},
			opts:       []runOption{withConfig(oauthURLs(expired.URL)), withNoBrowserOpen()},
			wantErr:    expiredErr,
			wantStderr: deviceInstructions + "Error: " + expiredErr + "\n",
			checks: []checkFunc{
				checkExitCode(common.ExitAuthenticationError),
				checkNoStoredCredentials,
			},
		},
		{
			// The codes ran out before the server said so; same message either way.
			name:       "code that expires locally ends the login the same way",
			args:       []string{"auth", "login", "--headless"},
			opts:       []runOption{withConfig(oauthURLs(stale.URL)), withNoBrowserOpen()},
			wantErr:    expiredErr,
			wantStderr: deviceInstructions + "Error: " + expiredErr + "\n",
			checks: []checkFunc{
				checkExitCode(common.ExitAuthenticationError),
				checkNoStoredCredentials,
			},
		},
		{
			// Without expires_in there is nothing to poll against, so the CLI
			// supplies the bound rather than waiting on the codes forever.
			name: "missing expires_in still bounds the wait",
			args: []string{"auth", "login", "--headless"},
			opts: []runOption{
				withConfig(oauthURLs(noExpiry.URL)),
				withNoBrowserOpen(),
				withDeviceCodeTTL(2 * time.Second),
			},
			wantErr:    expiredErr,
			wantStderr: deviceInstructions + "Error: " + expiredErr + "\n",
			checks: []checkFunc{
				checkExitCode(common.ExitAuthenticationError),
				checkNoStoredCredentials,
			},
		},
		{
			// A consumed or unknown device_code, as a second racing poll gets.
			name:       "consumed or unknown code ends the login immediately",
			args:       []string{"auth", "login", "--headless"},
			opts:       []runOption{withConfig(oauthURLs(consumed.URL)), withNoBrowserOpen()},
			wantErr:    consumedErr,
			wantStderr: deviceInstructions + "Error: " + consumedErr + "\n",
			checks: []checkFunc{
				checkExitCode(common.ExitAuthenticationError),
				checkNoStoredCredentials,
			},
		},
		{
			// Neither a verdict nor a failure to answer, so polling re-enters.
			name: "throttled poll is retried",
			args: []string{"auth", "login", "--headless"},
			opts: []runOption{
				withConfig(oauthURLs(throttled.URL)),
				withNoBrowserOpen(),
				withDeviceRetryDelay(time.Millisecond),
			},
			wantStdout: loggedIn,
			wantStderr: deviceInstructions + deviceRetryNotice,
			checks:     []checkFunc{checkStoredOAuthCredentials("project-123")},
		},
		{
			// An OAuth-shaped 500 ends the login, but as a service failure: the
			// code was never the problem, so it isn't blamed.
			name:       "server failure ends the login without blaming the code",
			args:       []string{"auth", "login", "--headless"},
			opts:       []runOption{withConfig(oauthURLs(unavailable.URL)), withNoBrowserOpen()},
			wantErr:    unavailableErr,
			wantStderr: deviceInstructions + "Error: " + unavailableErr + "\n",
			checks: []checkFunc{
				checkExitCode(common.ExitAuthenticationError),
				checkNoStoredCredentials,
			},
		},
		{
			// A 5xx with no OAuth error in it says the same thing, so it ends the
			// login the same way.
			name:       "non-OAuth 5xx ends the login too",
			args:       []string{"auth", "login", "--headless"},
			opts:       []runOption{withConfig(oauthURLs(badGateway.URL)), withNoBrowserOpen()},
			wantErr:    unavailableErr,
			wantStderr: deviceInstructions + "Error: " + unavailableErr + "\n",
			checks: []checkFunc{
				checkExitCode(common.ExitAuthenticationError),
				checkNoStoredCredentials,
			},
		},
		{
			// The one condition the device code stands in for: no browser to
			// redirect back from.
			name:       "failed browser open falls back to the device flow",
			args:       []string{"auth", "login"},
			opts:       []runOption{withConfig(oauthURLs(success.URL))},
			wantStdout: loggedIn,
			wantStderr: matchOAuthStderr(success.URL, regexp.QuoteMeta(
				"Failed to open browser: browser disabled in tests\n"+
					"Falling back to device authorization...\n"+deviceInstructions)),
			checks: []checkFunc{checkStoredOAuthCredentials("project-123")},
		},
	})
}
