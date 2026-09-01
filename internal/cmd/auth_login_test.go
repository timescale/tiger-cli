package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
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
	// its own TestOfferProdProtection.
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

// TestOfferProdProtection covers the post-login read-only prompt with the menu
// itself stubbed out; TestReadOnlyModel_KeySelection covers the menu. The
// "already ..." cases guard the ask-exactly-once property, which a stored
// read_only value is the only thing enforcing.
func TestOfferProdProtection(t *testing.T) {
	tests := []struct {
		name string
		// preexisting is empty when read_only is absent from the file.
		preexisting string
		// answer is what the menu returns; "" means dismissed without choosing.
		answer config.ReadOnlyMode
		// env is TIGER_READ_ONLY, which outranks the config file.
		env         string
		notATTY     bool
		wantMenu    bool
		wantStored  string // "" = key must stay absent
		wantMessage string // "" = nothing printed
		// wantReadOnlySet is the return value, which decides whether the caller
		// prints the "tiger config set read_only" bullet - false means it does.
		wantReadOnlySet bool
	}{
		{name: "prod stores prod", answer: config.ReadOnlyProd, wantMenu: true, wantStored: "prod", wantMessage: "Services tagged PROD are now protected", wantReadOnlySet: true},
		{name: "all stores all", answer: config.ReadOnlyAll, wantMenu: true, wantStored: "all", wantMessage: "All services are now protected", wantReadOnlySet: true},
		{name: "off stores off silently", answer: config.ReadOnlyOff, wantMenu: true, wantStored: "off", wantReadOnlySet: true},
		{name: "dismissed records nothing", answer: "", wantMenu: true, wantStored: ""},
		{name: "not a terminal skips silently", answer: config.ReadOnlyProd, notATTY: true, wantStored: ""},
		{name: "already declined", preexisting: "off", answer: config.ReadOnlyProd, wantStored: "off", wantReadOnlySet: true},
		{name: "already opted in", preexisting: "prod", answer: config.ReadOnlyOff, wantStored: "prod", wantReadOnlySet: true},
		{name: "already set to all", preexisting: "all", answer: config.ReadOnlyProd, wantStored: "all", wantReadOnlySet: true},
		// The env var wins over the file, so writing a choice there would be a lie.
		{name: "env var answers it without prompting", env: "all", answer: config.ReadOnlyProd, wantStored: "", wantReadOnlySet: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if tt.env != "" {
				t.Setenv("TIGER_READ_ONLY", tt.env)
			}
			values := map[string]any{}
			if tt.preexisting != "" {
				values["read_only"] = tt.preexisting
			}
			writeConfigFile(t, tmpDir, values)
			cfg, err := config.Load(testFlags(t, tmpDir))
			if err != nil {
				t.Fatalf("config.Load failed: %v", err)
			}
			stubIsTerminal(t, !tt.notATTY)

			shown := false
			original := selectReadOnlyMode
			t.Cleanup(func() { selectReadOnlyMode = original })
			selectReadOnlyMode = func(_ *cobra.Command) (config.ReadOnlyMode, bool) {
				shown = true
				return tt.answer, tt.answer != ""
			}

			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			errOut := new(bytes.Buffer)
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(errOut)

			readOnlySet := offerProdProtection(cmd, cfg)

			if shown != tt.wantMenu {
				t.Errorf("menu shown = %t, want %t", shown, tt.wantMenu)
			}
			if readOnlySet != tt.wantReadOnlySet {
				t.Errorf("readOnlySet = %t, want %t", readOnlySet, tt.wantReadOnlySet)
			}

			stored, err := config.LoadForOutput(tmpDir, false, true)
			if err != nil {
				t.Fatalf("LoadForOutput failed: %v", err)
			}
			got := ""
			if stored.ReadOnly != nil {
				got = string(*stored.ReadOnly)
			}
			if got != tt.wantStored {
				t.Errorf("stored read_only = %q, want %q", got, tt.wantStored)
			}

			if tt.wantMessage == "" {
				if errOut.Len() != 0 {
					t.Errorf("expected no output, got %q", errOut.String())
				}
			} else if !strings.Contains(errOut.String(), tt.wantMessage) {
				t.Errorf("expected %q in output, got %q", tt.wantMessage, errOut.String())
			}
		})
	}
}

// TestReadOnlyModel_KeySelection checks that every mode is reachable and that
// dismissing leaves chosen empty, which is what keeps a quit from being recorded
// as picking whichever mode the cursor rested on.
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
