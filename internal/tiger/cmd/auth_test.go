package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func setupAuthTest(t *testing.T) string {
	t.Helper()

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Mock the API key validation for testing
	originalValidator := validateAPIKey
	validateAPIKey = func(ctx context.Context, cfg *config.Config, client *api.ClientWithResponses) (*api.AuthInfo, error) {
		authInfo := &api.AuthInfo{}
		json.Unmarshal([]byte(`{"type":"apiKey","apiKey":{"public_key":"test-access-key","project":{"id":"test-project-id"}}}`), authInfo)
		return authInfo, nil
	}

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set TIGER_CONFIG_DIR environment variable so that when commands execute
	// and reinitialize viper, they use the test directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Disable analytics for auth tests to avoid tracking test events
	os.Setenv("TIGER_ANALYTICS", "false")

	// Reset global config and viper to ensure test isolation
	// This ensures proper test isolation by resetting all viper state
	// MUST be done before RemoveCredentials() so it uses the test directory!
	if _, err := config.UseTestConfig(tmpDir, map[string]any{}); err != nil {
		t.Fatalf("Failed to use test config: %v", err)
	}

	// Clean up any existing test credentials
	config.RemoveCredentials()

	t.Cleanup(func() {
		// Clean up test credentials
		config.RemoveCredentials()
		// Reset global config and viper first
		config.ResetGlobalConfig()
		validateAPIKey = originalValidator // Restore original validator
		// Remove config file explicitly
		configFile := config.GetConfigFile(tmpDir)
		os.Remove(configFile)
		// Clean up environment variables BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_ANALYTICS")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func executeAuthCommand(ctx context.Context, args ...string) (string, error) {
	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot, err := buildRootCmd(ctx)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err = testRoot.Execute()
	return buf.String(), err
}

func TestAuthLogin_KeyFlags(t *testing.T) {
	setupAuthTest(t)

	// Execute login command with public and secret key flags (project ID is auto-detected)
	output, err := executeAuthCommand(t.Context(), "auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-id)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify credentials were stored (try keyring first, then file fallback)
	// The combined key should be in format "public:secret"
	expectedAPIKey := "test-public-key:test-secret-key"
	expectedProjectID := "test-project-id" // Comes from mock validation function

	creds, err := config.GetStoredCredentials()
	if err != nil {
		t.Fatalf("Credentials not stored in keyring or file: %v", err)
	}
	apiKey, projectID := creds.APIKey, creds.ProjectID

	if apiKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, apiKey)
	}
	if projectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, projectID)
	}
}

func TestAuthLogin_KeyEnvironmentVariables(t *testing.T) {
	setupAuthTest(t)

	// Set environment variables for public and secret keys (project ID is auto-detected)
	os.Setenv("TIGER_PUBLIC_KEY", "env-public-key")
	os.Setenv("TIGER_SECRET_KEY", "env-secret-key")
	defer os.Unsetenv("TIGER_PUBLIC_KEY")
	defer os.Unsetenv("TIGER_SECRET_KEY")

	// Execute login command using env vars for keys
	output, err := executeAuthCommand(t.Context(), "auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-id)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify credentials were stored
	expectedAPIKey := "env-public-key:env-secret-key"
	expectedProjectID := "test-project-id" // Auto-detected from mock
	creds, err := config.GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	storedKey, storedProjectID := creds.APIKey, creds.ProjectID
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
	if storedProjectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, storedProjectID)
	}
}

// setupOAuthTest creates a complete OAuth test environment with mock server and browser
func setupOAuthTest(t *testing.T, projects []api.Project, expectedProjectID string) string {
	t.Helper()
	tmpDir := setupAuthTest(t)

	// Ensure no keys in environment
	os.Unsetenv("TIGER_PUBLIC_KEY")
	os.Unsetenv("TIGER_SECRET_KEY")

	// Start mock server for OAuth endpoints
	mockServer := startMockOAuthServer(t, projects)

	// Set up mock browser function
	originalOpenBrowser := openBrowser
	openBrowser = mockOpenBrowser(t)

	// Set config URLs to point to mock server
	configFile := config.GetConfigFile(tmpDir)
	configContent := fmt.Sprintf(`
console_url: "%s"
gateway_url: "%s"
api_url: "%s"
`, mockServer.URL, mockServer.URL, mockServer.URL)
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Return cleanup function
	t.Cleanup(func() {
		mockServer.Close()
		openBrowser = originalOpenBrowser
	})

	return mockServer.URL
}

// startMockOAuthServer starts a mock server that handles all OAuth endpoints
func startMockOAuthServer(t *testing.T, projects []api.Project) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Token exchange endpoint
	mux.HandleFunc("POST /idp/external/cli/token", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock server received token exchange request")

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		// The mock backs both the initial authorization_code exchange and the
		// silent refresh that oauth2.NewClient performs once the access token
		// is past its expiry. Both grants return the same canned token so
		// downstream assertions remain stable.
		grantType := r.FormValue("grant_type")
		switch grantType {
		case "refresh_token":
			if r.FormValue("refresh_token") == "" || r.FormValue("client_id") == "" {
				http.Error(w, "Missing required parameters", http.StatusBadRequest)
				return
			}
		default:
			if r.FormValue("client_id") == "" || r.FormValue("code") == "" || r.FormValue("code_verifier") == "" {
				http.Error(w, "Missing required parameters", http.StatusBadRequest)
				return
			}
		}

		tokenResponse := map[string]interface{}{
			"access_token":  "mock-access-token-12345",
			"refresh_token": "mock-refresh-token-67890",
			"expires_in":    3600,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(tokenResponse); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	})

	// REST endpoint backing selectProjectID
	mux.HandleFunc("GET /projects", func(w http.ResponseWriter, _ *http.Request) {
		t.Logf("Mock server received GET /projects request")
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(projects); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	})

	// OAuth success endpoint (just returns 200 OK)
	mux.HandleFunc("GET /oauth/code/success", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock server received OAuth success request")
		w.WriteHeader(http.StatusOK)
	})

	// Create test server
	return httptest.NewServer(mux)
}

// mockOpenBrowser returns a mock openBrowser function that simulates OAuth callback
func mockOpenBrowser(t *testing.T) func(string) error {
	return func(authURL string) error {
		t.Logf("Mock browser opening URL: %s", authURL)

		// Extract redirect_uri and state from the URL parameters
		parsedURL, err := url.Parse(authURL)
		if err != nil {
			return err
		}

		clientID := parsedURL.Query().Get("client_id")
		responseType := parsedURL.Query().Get("response_type")
		codeChallengeMethod := parsedURL.Query().Get("code_challenge_method")
		codeChallenge := parsedURL.Query().Get("code_challenge")
		redirectURI := parsedURL.Query().Get("redirect_uri")
		state := parsedURL.Query().Get("state")

		if clientID == "" {
			t.Fatal("no client_id found in OAuth URL")
			return errors.New("no client_id found in OAuth URL")
		}

		if responseType != "code" {
			t.Fatal("invalid response_type found in OAuth URL")
			return errors.New("no response_type found in OAuth URL")
		}

		if codeChallengeMethod != "S256" {
			t.Fatal("invalid code_challenge_method found in OAuth URL")
			return errors.New("no code_challenge_method found in OAuth URL")
		}

		if codeChallenge == "" {
			t.Fatal("no code_challenge found in OAuth URL")
			return errors.New("no code_challenge found in OAuth URL")
		}

		if redirectURI == "" {
			t.Fatal("no redirect_uri found in OAuth URL")
			return errors.New("no redirect_uri found in OAuth URL")
		}

		if state == "" {
			t.Fatal("no state found in OAuth URL")
			return errors.New("no state found in OAuth URL")
		}

		// Give the OAuth server a moment to start
		go func() {
			// Make the OAuth callback request directly
			callbackURL := fmt.Sprintf("%s?code=test-auth-code&state=%s", redirectURI, state)
			t.Logf("Mock browser making callback request to: %s", callbackURL)

			resp, err := http.Get(callbackURL)
			if err != nil {
				t.Errorf("Mock callback request failed: %v", err)
				return
			}
			if err := resp.Body.Close(); err != nil {
				t.Errorf("Error closing callback request body: %v", err)
			}
		}()

		return nil
	}
}

// assertExpiresInAbout checks that the token Expiry was derived from the
// standard `expires_in` (the mock returns 3600s), allowing slack for elapsed
// test time.
func assertExpiresInAbout(t *testing.T, expiry time.Time) {
	t.Helper()
	d := time.Until(expiry)
	if d < 3540*time.Second || d > 3600*time.Second {
		t.Errorf("Expected expiry ~3600s from now (from expires_in=3600), got %v (in %v)", expiry, d)
	}
}

func TestAuthLogin_OAuth_SingleProject(t *testing.T) {
	mockServerURL := setupOAuthTest(t, []api.Project{
		{Id: "project-123", Name: "Test Project"},
	}, "project-123")

	// Execute login command - the mocked openBrowser will handle the callback automatically
	output, err := executeAuthCommand(t.Context(), "auth", "login")

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedPattern := fmt.Sprintf(`^Auth URL is: %s/oauth/authorize\?client_id=45e1b16d-e435-4049-97b2-8daad150818c&code_challenge=[A-Za-z0-9_-]+&code_challenge_method=S256&redirect_uri=http%%3A%%2F%%2Flocalhost%%3A\d+%%2Fcallback&response_type=code&state=[A-Za-z0-9_-]+\n`+
		`Opening browser for authentication\.\.\.\n`+
		`Successfully logged in \(project: project-123\)\n`+
		regexp.QuoteMeta(nextStepsMessage)+`$`, regexp.QuoteMeta(mockServerURL))

	matched, err := regexp.MatchString(expectedPattern, output)
	if err != nil {
		t.Fatalf("Regex compilation failed: %v", err)
	}
	if !matched {
		t.Errorf("Output doesn't match expected pattern.\nPattern: %s\nActual output: '%s'", expectedPattern, output)
	}

	stored, err := config.GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if stored.OAuth == nil {
		t.Fatalf("Expected OAuth credentials, got PAT: %+v", stored)
	}
	token, projectID := stored.OAuth, stored.ProjectID
	if token.AccessToken != "mock-access-token-12345" {
		t.Errorf("Expected access token 'mock-access-token-12345', got '%s'", token.AccessToken)
	}
	if token.RefreshToken != "mock-refresh-token-67890" {
		t.Errorf("Expected refresh token 'mock-refresh-token-67890', got '%s'", token.RefreshToken)
	}
	assertExpiresInAbout(t, token.Expiry)
	if projectID != "project-123" {
		t.Errorf("Expected project ID 'project-123', got '%s'", projectID)
	}
}

func TestAuthLogin_OAuth_MultipleProjects(t *testing.T) {
	mockServerURL := setupOAuthTest(t, []api.Project{
		{Id: "project-123", Name: "Test Project 1"},
		{Id: "project-456", Name: "Test Project 2"},
		{Id: "project-789", Name: "Test Project 3"},
	}, "project-789")

	// Mock the project selection to simulate user selecting the third project (index 2)
	originalSelectProjectInteractively := selectProjectInteractively
	defer func() {
		selectProjectInteractively = originalSelectProjectInteractively
	}()

	selectProjectInteractively = func(projects []api.Project, out io.Writer) (string, error) {
		t.Logf("Mock project selection - user selects project at index 2: %s", projects[2].Id)
		// Simulate user pressing down arrow twice and then enter (selects third project)
		return projects[2].Id, nil
	}

	// Execute login command - both mocked functions will handle OAuth flow and project selection
	output, err := executeAuthCommand(t.Context(), "auth", "login")

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedPattern := fmt.Sprintf(`^Auth URL is: %s/oauth/authorize\?client_id=45e1b16d-e435-4049-97b2-8daad150818c&code_challenge=[A-Za-z0-9_-]+&code_challenge_method=S256&redirect_uri=http%%3A%%2F%%2Flocalhost%%3A\d+%%2Fcallback&response_type=code&state=[A-Za-z0-9_-]+\n`+
		`Opening browser for authentication\.\.\.\n`+
		`Successfully logged in \(project: project-789\)\n`+
		regexp.QuoteMeta(nextStepsMessage)+`$`, regexp.QuoteMeta(mockServerURL))

	matched, err := regexp.MatchString(expectedPattern, output)
	if err != nil {
		t.Fatalf("Regex compilation failed: %v", err)
	}
	if !matched {
		t.Errorf("Output doesn't match expected pattern.\nPattern: %s\nActual output: '%s'", expectedPattern, output)
	}

	stored, err := config.GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if stored.OAuth == nil {
		t.Fatalf("Expected OAuth credentials, got PAT: %+v", stored)
	}
	token, projectID := stored.OAuth, stored.ProjectID
	if token.AccessToken != "mock-access-token-12345" {
		t.Errorf("Expected access token 'mock-access-token-12345', got '%s'", token.AccessToken)
	}
	if token.RefreshToken != "mock-refresh-token-67890" {
		t.Errorf("Expected refresh token 'mock-refresh-token-67890', got '%s'", token.RefreshToken)
	}
	assertExpiresInAbout(t, token.Expiry)
	if projectID != "project-789" {
		t.Errorf("Expected project ID 'project-789', got '%s'", projectID)
	}
}

// TestOAuthRefresh_PersistsExpiry verifies that when an expired OAuth token is
// refreshed, the rotated token is persisted with a non-zero Expiry derived from
// the standard `expires_in` returned by the gateway.
func TestOAuthRefresh_PersistsExpiry(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// Mock server backs the refresh_token grant (returns expires_in=3600).
	mockServer := startMockOAuthServer(t, nil)
	configFile := config.GetConfigFile(tmpDir)
	configContent := fmt.Sprintf("gateway_url: \"%s\"\napi_url: \"%s\"\n", mockServer.URL, mockServer.URL)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Store an already-expired OAuth token that still has a valid refresh token.
	expired := &oauth2.Token{
		AccessToken:  "stale-access-token",
		RefreshToken: "mock-refresh-token-67890",
		Expiry:       time.Now().Add(-time.Hour),
	}
	if err := config.StoreOAuthCredentials(expired, "project-789"); err != nil {
		t.Fatalf("Failed to store oauth credentials: %v", err)
	}

	cfg := &config.Config{APIURL: mockServer.URL, GatewayURL: mockServer.URL}
	stored, err := config.GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to load stored credentials: %v", err)
	}

	client, err := api.NewTigerClientForCredentials(cfg, stored)
	if err != nil {
		t.Fatalf("Failed to build client: %v", err)
	}

	// Any request makes the oauth2 transport mint a token first; since the
	// stored token is expired, that triggers a refresh + persist. The response
	// status itself is irrelevant — we only care about the persisted token.
	if _, err := client.GetAuthInfoWithResponse(t.Context()); err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	reloaded, err := config.GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to reload credentials: %v", err)
	}
	if reloaded.OAuth == nil {
		t.Fatal("Expected OAuth credentials to remain stored after refresh")
	}
	if reloaded.OAuth.AccessToken != "mock-access-token-12345" {
		t.Fatalf("Expected token to be refreshed, got access token %q", reloaded.OAuth.AccessToken)
	}
	assertExpiresInAbout(t, reloaded.OAuth.Expiry)
}

// TestAuthLogin_KeyringFallback tests the scenario where keyring fails and system falls back to file storage
func TestAuthLogin_KeyringFallback(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// We can't easily mock keyring failure, but we can test file storage directly
	// by ensuring the API key gets stored to file when keyring might not be available

	// Execute login command with public and secret key flags (project ID auto-detected)
	output, err := executeAuthCommand(t.Context(), "auth", "login", "--public-key", "fallback-public", "--secret-key", "fallback-secret")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-id)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Force test file storage scenario by directly checking file
	credentialsFile := filepath.Join(tmpDir, "credentials")

	// If keyring worked, manually create file scenario by clearing all credentials and adding to file
	config.RemoveCredentials()

	// Store to file manually to simulate fallback
	expectedAPIKey := "fallback-public:fallback-secret"
	expectedProjectID := "test-project-id"
	if err := config.StoreCredentialsToFile(expectedAPIKey, expectedProjectID); err != nil {
		t.Fatalf("Failed to store credentials to file: %v", err)
	}

	// Verify file storage works
	creds, err := config.GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get credentials from file fallback: %v", err)
	}
	storedKey, storedProjectID := creds.APIKey, creds.ProjectID
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
	if storedProjectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, storedProjectID)
	}

	// Test logout with file-only storage
	output, err = executeAuthCommand(t.Context(), "auth", "logout")
	if err != nil {
		t.Fatalf("Logout failed with file storage: %v", err)
	}
	if output != "Successfully logged out and removed stored credentials\n" {
		t.Errorf("Unexpected logout output: '%s'", output)
	}

	// Verify file was removed
	if _, err := os.Stat(credentialsFile); !os.IsNotExist(err) {
		t.Error("Credentials file should be removed after logout")
	}
}

// TestAuthLogin_EnvironmentVariable_FileOnly tests env var login when only file storage is available
func TestAuthLogin_EnvironmentVariable_FileOnly(t *testing.T) {
	setupAuthTest(t)

	// Set environment variables for public key and secret key (project ID auto-detected)
	os.Setenv("TIGER_PUBLIC_KEY", "env-file-public")
	os.Setenv("TIGER_SECRET_KEY", "env-file-secret")
	defer os.Unsetenv("TIGER_PUBLIC_KEY")
	defer os.Unsetenv("TIGER_SECRET_KEY")

	// Execute login command without any flags (keys from env vars, project ID auto-detected)
	output, err := executeAuthCommand(t.Context(), "auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-id)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Clear all credentials to ensure we're testing file-only retrieval
	config.RemoveCredentials()

	// Verify credentials were stored in file (since we'll manually write to file only)
	expectedAPIKey := "env-file-public:env-file-secret"
	expectedProjectID := "test-project-id"

	// Store to file manually to simulate fallback scenario
	if err := config.StoreCredentialsToFile(expectedAPIKey, expectedProjectID); err != nil {
		t.Fatalf("Failed to store credentials to file: %v", err)
	}

	// Verify getCredentials works with file-only storage
	creds, err := config.GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get credentials from file: %v", err)
	}
	storedKey, storedProjectID := creds.APIKey, creds.ProjectID
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
	if storedProjectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, storedProjectID)
	}
}

func TestAuthStatus_LoggedIn(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// Create a mock server for the /auth/info endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/info" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"type": "apiKey",
				"apiKey": {
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
	err := config.StoreCredentials("test-api-key-789", "test-project-789")
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
