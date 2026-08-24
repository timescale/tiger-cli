package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

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

	creds, err := testConfig(t).GetStoredCredentials()
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
	creds, err := testConfig(t).GetStoredCredentials()
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
	testConfig(t).RemoveCredentials()

	// Store to file manually to simulate fallback
	expectedAPIKey := "fallback-public:fallback-secret"
	expectedProjectID := "test-project-id"
	if err := testConfig(t).StoreCredentialsToFile(expectedAPIKey, expectedProjectID); err != nil {
		t.Fatalf("Failed to store credentials to file: %v", err)
	}

	// Verify file storage works
	creds, err := testConfig(t).GetStoredCredentials()
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
	testConfig(t).RemoveCredentials()

	// Verify credentials were stored in file (since we'll manually write to file only)
	expectedAPIKey := "env-file-public:env-file-secret"
	expectedProjectID := "test-project-id"

	// Store to file manually to simulate fallback scenario
	if err := testConfig(t).StoreCredentialsToFile(expectedAPIKey, expectedProjectID); err != nil {
		t.Fatalf("Failed to store credentials to file: %v", err)
	}

	// Verify getCredentials works with file-only storage
	creds, err := testConfig(t).GetStoredCredentials()
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

func TestAuthLogin_APIKeyValidationFailure(t *testing.T) {
	// Set up test environment but don't use setupAuthTest since we want to test validation failure
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Point the command under test (and testConfig) at the test directory
	t.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Use a unique service name for this test
	config.SetTestServiceName(t)

	originalValidator := validateAPIKey

	// Mock the validator to return an error
	validateAPIKey = func(ctx context.Context, cfg *config.Config, client api.ClientWithResponsesInterface) (*api.AuthInfo, error) {
		return nil, errors.New("invalid API key: authentication failed")
	}

	defer func() {
		validateAPIKey = originalValidator
	}()

	// Write an empty config file in the test directory
	if _, err := config.UseTestConfig(tmpDir, map[string]any{}); err != nil {
		t.Fatalf("Failed to use test config: %v", err)
	}

	// Clean up credentials
	testConfig(t).RemoveCredentials()
	defer testConfig(t).RemoveCredentials()

	// Execute login command with public and secret key flags - should fail validation
	output, err := executeAuthCommand(t.Context(), "auth", "login", "--public-key", "invalid-public", "--secret-key", "invalid-secret")
	if err == nil {
		t.Fatal("Expected login to fail with invalid keys, but it succeeded")
	}

	expectedErrorMsg := "API key validation failed: invalid API key: authentication failed"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error to contain %q, got: %v", expectedErrorMsg, err)
	}

	// Verify that output contains validation message
	if !strings.Contains(output, "Validating API key...") {
		t.Errorf("Expected output to contain validation message, got: %s", output)
	}

	// Verify that no credentials were stored
	if _, err := testConfig(t).GetStoredCredentials(); err == nil {
		t.Error("Credentials should not be stored when validation fails")
	}
}

func TestAuthLogin_APIKeyValidationSuccess(t *testing.T) {
	// Set up test environment
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Point the command under test (and testConfig) at the test directory
	t.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Use a unique service name for this test
	config.SetTestServiceName(t)

	originalValidator := validateAPIKey

	// Mock the validator to return success
	validateAPIKey = func(ctx context.Context, cfg *config.Config, client api.ClientWithResponsesInterface) (*api.AuthInfo, error) {
		authInfo := &api.AuthInfo{}
		json.Unmarshal([]byte(`{"type":"apiKey","api_key":{"public_key":"test-access-key","project":{"id":"test-project-valid"}}}`), authInfo)
		return authInfo, nil // Success
	}

	defer func() {
		validateAPIKey = originalValidator
	}()

	// Write an empty config file in the test directory
	if _, err := config.UseTestConfig(tmpDir, map[string]any{}); err != nil {
		t.Fatalf("Failed to use test config: %v", err)
	}

	// Clean up credentials
	testConfig(t).RemoveCredentials()
	defer testConfig(t).RemoveCredentials()

	// Execute login command with public and secret key flags - should succeed
	output, err := executeAuthCommand(t.Context(), "auth", "login", "--public-key", "valid-public", "--secret-key", "valid-secret")
	if err != nil {
		t.Fatalf("Expected login to succeed with valid keys, got error: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-valid)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Expected output %q, got %q", expectedOutput, output)
	}

	// Verify that credentials were stored
	expectedAPIKey := "valid-public:valid-secret"
	expectedProjectID := "test-project-valid"
	creds, err := testConfig(t).GetStoredCredentials()
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

// assertOAuthLoginOutput checks the OAuth login flow's full output: the auth
// URL, the browser message, and the success line naming projectID.
func assertOAuthLoginOutput(t *testing.T, output, mockServerURL, projectID string) {
	t.Helper()

	expectedPattern := fmt.Sprintf(`^Auth URL is: %s/oauth/authorize\?client_id=45e1b16d-e435-4049-97b2-8daad150818c&code_challenge=[A-Za-z0-9_-]+&code_challenge_method=S256&redirect_uri=http%%3A%%2F%%2Flocalhost%%3A\d+%%2Fcallback&response_type=code&state=[A-Za-z0-9_-]+\n`+
		`Opening browser for authentication\.\.\.\n`+
		`Successfully logged in \(project: %s\)\n`+
		regexp.QuoteMeta(nextStepsMessage)+`$`, regexp.QuoteMeta(mockServerURL), regexp.QuoteMeta(projectID))

	matched, err := regexp.MatchString(expectedPattern, output)
	if err != nil {
		t.Fatalf("Regex compilation failed: %v", err)
	}
	if !matched {
		t.Errorf("Output doesn't match expected pattern.\nPattern: %s\nActual output: '%s'", expectedPattern, output)
	}
}

func TestAuthLogin_OAuth_SingleProject(t *testing.T) {
	mockServerURL := setupOAuthTest(t, []api.Project{
		{ID: "project-123", Name: "Test Project"},
	})

	// Execute login command - the mocked openBrowser will handle the callback automatically
	output, err := executeAuthCommand(t.Context(), "auth", "login")

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	assertOAuthLoginOutput(t, output, mockServerURL, "project-123")

	stored := assertStoredProject(t, "project-123")
	assertExpiresInAbout(t, stored.OAuth.Expiry)
}

func TestAuthLogin_OAuth_MultipleProjects(t *testing.T) {
	mockServerURL := setupOAuthTest(t, []api.Project{
		{ID: "project-123", Name: "Test Project 1"},
		{ID: "project-456", Name: "Test Project 2"},
		{ID: "project-789", Name: "Test Project 3"},
	})

	// The picker only runs on a TTY
	stubIsTerminal(t, true)

	// Mock the project selection to simulate user selecting the third project (index 2)
	stubSelectProject(t, func(_ *cobra.Command, projects []api.Project) (api.Project, error) {
		t.Logf("Mock project selection - user selects project at index 2: %s", projects[2].ID)
		// Simulate user pressing down arrow twice and then enter (selects third project)
		return projects[2], nil
	})

	// Execute login command - both mocked functions will handle OAuth flow and project selection
	output, err := executeAuthCommand(t.Context(), "auth", "login")

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	assertOAuthLoginOutput(t, output, mockServerURL, "project-789")

	stored := assertStoredProject(t, "project-789")
	assertExpiresInAbout(t, stored.OAuth.Expiry)
}

// TestAuthLogin_OAuth_ProjectIDFlag verifies --project-id replaces the picker,
// so a multi-project user can log in without a terminal.
func TestAuthLogin_OAuth_ProjectIDFlag(t *testing.T) {
	setupOAuthTest(t, testProjects)

	// No TTY: the picker isn't available, and isn't needed. If --project-id
	// were ignored, resolveProjectID's TTY gate would fail the login.
	stubIsTerminal(t, false)

	output, err := executeAuthCommand(t.Context(), "auth", "login", "--project-id", "project-456")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if !strings.Contains(output, "Successfully logged in (project: project-456)") {
		t.Errorf("Unexpected output: %q", output)
	}

	assertStoredProject(t, "project-456")
}

// TestAuthLogin_OAuth_ProjectIDEnvVar verifies TIGER_PROJECT_ID works like the
// flag, as the API key env vars do.
func TestAuthLogin_OAuth_ProjectIDEnvVar(t *testing.T) {
	setupOAuthTest(t, testProjects)
	t.Setenv("TIGER_PROJECT_ID", "project-789")

	stubIsTerminal(t, false)

	output, err := executeAuthCommand(t.Context(), "auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if !strings.Contains(output, "Successfully logged in (project: project-789)") {
		t.Errorf("Unexpected output: %q", output)
	}

	assertStoredProject(t, "project-789")
}

func TestAuthLogin_OAuth_ProjectIDFlag_Inaccessible(t *testing.T) {
	setupOAuthTest(t, testProjects)

	_, err := executeAuthCommand(t.Context(), "auth", "login", "--project-id", "project-nope")
	if err == nil {
		t.Fatal("Expected login to fail for an inaccessible project")
	}
	if !strings.Contains(err.Error(), `project "project-nope" not found or not accessible`) {
		t.Errorf("Unexpected error: %v", err)
	}
	assertExitCode(t, err, common.ExitInvalidParameters)

	// A failed selection stores nothing
	if _, err := testConfig(t).GetStoredCredentials(); err == nil {
		t.Error("Credentials should not be stored when project selection fails")
	}
}

// TestAuthLogin_OAuth_ProjectIDEnvVar_Inaccessible verifies an ambient
// TIGER_PROJECT_ID naming a project this account can't see only warns and
// falls back to normal selection, matching the API-key branch.
func TestAuthLogin_OAuth_ProjectIDEnvVar_Inaccessible(t *testing.T) {
	setupOAuthTest(t, testProjects[:1])
	t.Setenv("TIGER_PROJECT_ID", "project-nope")

	stubIsTerminal(t, false)

	output, err := executeAuthCommand(t.Context(), "auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if !strings.Contains(output, "Warning: ignoring TIGER_PROJECT_ID (project-nope) - project not found or not accessible") {
		t.Errorf("Expected a warning about the ignored env var, got: %q", output)
	}

	// Falls back to the single accessible project
	assertStoredProject(t, "project-123")
}

// TestAuthLogin_APIKey_ProjectIDEmptyFlagEnvMismatch verifies an explicitly
// empty --project-id counts as unset, so a mismatched ambient TIGER_PROJECT_ID
// still only warns (e.g. `--project-id "$PROJ"` in CI with $PROJ unset).
func TestAuthLogin_APIKey_ProjectIDEmptyFlagEnvMismatch(t *testing.T) {
	setupAuthTest(t)
	t.Setenv("TIGER_PROJECT_ID", "some-other-project")

	output, err := executeAuthCommand(t.Context(), "auth", "login",
		"--public-key", "test-public-key", "--secret-key", "test-secret-key",
		"--project-id", "")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if !strings.Contains(output, "Warning: ignoring TIGER_PROJECT_ID (some-other-project) - this API key is scoped to project test-project-id") {
		t.Errorf("Expected a warning about the ignored env var, got: %q", output)
	}
}

// TestAuthLogin_OAuth_MultipleProjectsWithoutTTY verifies the error names the
// non-interactive alternative instead of leaving the user stuck.
func TestAuthLogin_OAuth_MultipleProjectsWithoutTTY(t *testing.T) {
	setupOAuthTest(t, testProjects)

	stubIsTerminal(t, false)

	_, err := executeAuthCommand(t.Context(), "auth", "login")
	if err == nil {
		t.Fatal("Expected login to fail without a TTY to select a project on")
	}
	if !strings.Contains(err.Error(), "pass --project-id or set TIGER_PROJECT_ID") {
		t.Errorf("Expected the error to name the non-interactive alternative, got: %v", err)
	}
}

// TestAuthLogin_APIKey_ProjectIDMismatch verifies --project-id is checked
// against the key's own project instead of being ignored.
func TestAuthLogin_APIKey_ProjectIDMismatch(t *testing.T) {
	setupAuthTest(t)

	_, err := executeAuthCommand(t.Context(), "auth", "login",
		"--public-key", "test-public-key", "--secret-key", "test-secret-key",
		"--project-id", "some-other-project")
	if err == nil {
		t.Fatal("Expected login to fail when --project-id doesn't match the API key's project")
	}
	if !strings.Contains(err.Error(), "API key is scoped to project test-project-id, not the requested some-other-project") {
		t.Errorf("Unexpected error: %v", err)
	}
	assertExitCode(t, err, common.ExitInvalidParameters)

	if _, err := testConfig(t).GetStoredCredentials(); err == nil {
		t.Error("Credentials should not be stored when the requested project doesn't match")
	}
}

// TestAuthLogin_APIKey_ProjectIDEnvVarMismatch verifies an ambient
// TIGER_PROJECT_ID only warns, since it may be set for OAuth logins elsewhere
// in the environment.
func TestAuthLogin_APIKey_ProjectIDEnvVarMismatch(t *testing.T) {
	setupAuthTest(t)
	t.Setenv("TIGER_PROJECT_ID", "some-other-project")

	output, err := executeAuthCommand(t.Context(), "auth", "login",
		"--public-key", "test-public-key", "--secret-key", "test-secret-key")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if !strings.Contains(output, "Warning: ignoring TIGER_PROJECT_ID (some-other-project) - this API key is scoped to project test-project-id") {
		t.Errorf("Expected a warning about the ignored env var, got: %q", output)
	}

	creds, err := testConfig(t).GetStoredCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if creds.ProjectID != "test-project-id" {
		t.Errorf("Expected the API key's own project, got %q", creds.ProjectID)
	}
}

// TestAuthLogin_APIKey_ProjectIDMatches verifies a matching --project-id is
// accepted, so a script can pass it whatever the auth method.
func TestAuthLogin_APIKey_ProjectIDMatches(t *testing.T) {
	setupAuthTest(t)

	output, err := executeAuthCommand(t.Context(), "auth", "login",
		"--public-key", "test-public-key", "--secret-key", "test-secret-key",
		"--project-id", "test-project-id")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if !strings.Contains(output, "Successfully logged in (project: test-project-id)") {
		t.Errorf("Unexpected output: %q", output)
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

	// The config file above points api_url/gateway_url at the mock server, and
	// carries the test config dir so the refreshed token is persisted there.
	storeExpiredOAuthLogin(t, "project-789")

	cfg := testConfig(t)
	stored, err := cfg.GetStoredCredentials()
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

	reloaded, err := testConfig(t).GetStoredCredentials()
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

// setupOAuthTest creates a complete OAuth test environment with mock server and browser
func setupOAuthTest(t *testing.T, projects []api.Project) string {
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

// By default the mock mints one fixed, long-lived token, so tests can assert on
// its value. mockShortLivedRotatingTokens makes it behave like a real IdP —
// a distinct token per mint, expiring at once — so every request refreshes and
// every refresh persists.
// Both are atomic: the cleanup resetting rotatingTokens runs before the mock
// server closes (t.Cleanup is LIFO), so a straggling in-flight request can
// still be reading in a handler goroutine while the test goroutine writes.
var (
	rotatingTokens     atomic.Bool
	accessTokenCounter atomic.Int64
)

func mockShortLivedRotatingTokens(t *testing.T) {
	t.Helper()
	rotatingTokens.Store(true)
	accessTokenCounter.Store(0)
	t.Cleanup(func() { rotatingTokens.Store(false) })
}

func mockAccessToken() string {
	if !rotatingTokens.Load() {
		return "mock-access-token-12345"
	}
	return fmt.Sprintf("mock-access-token-%d", accessTokenCounter.Add(1))
}

// mockTokenExpiresIn is the expires_in the mock returns: 1 second under
// rotation, which oauth2 treats as already expired (10s delta).
func mockTokenExpiresIn() int {
	if rotatingTokens.Load() {
		return 1
	}
	return 3600
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
			// Exchange must carry the CLI User-Agent.
			if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "tiger-cli/") {
				t.Errorf("code exchange User-Agent = %q, want \"tiger-cli/\" prefix", ua)
			}
		}

		tokenResponse := map[string]interface{}{
			"access_token":  mockAccessToken(),
			"refresh_token": "mock-refresh-token-67890",
			"expires_in":    mockTokenExpiresIn(),
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

	// Backs the validation common.NewAPIClient runs on API keys from the
	// environment.
	mux.HandleFunc("GET /auth/info", func(w http.ResponseWriter, _ *http.Request) {
		t.Logf("Mock server received GET /auth/info request")
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"type":"apiKey","api_key":{"public_key":"env-public-key","project":{"id":"env-key-project","name":"Env Key Project","plan_type":"free"},"issuing_user":{"id":"1","name":"Test User","email":"test@example.com"},"name":"key","created":"2026-01-01T00:00:00Z"}}`)); err != nil {
			t.Errorf("Failed to write auth info response: %v", err)
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
