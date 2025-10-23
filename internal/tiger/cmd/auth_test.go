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

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func setupAuthTest(t *testing.T) string {
	t.Helper()

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Mock the API key validation for testing
	originalValidator := validateAPIKeyForLogin
	validateAPIKeyForLogin = func(apiKey, projectID string) error {
		// Always return success for testing
		return nil
	}

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set TIGER_CONFIG_DIR environment variable so that when commands execute
	// and reinitialize viper, they use the test directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

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
		validateAPIKeyForLogin = originalValidator // Restore original validator
		// Remove config file explicitly
		configFile := config.GetConfigFile(tmpDir)
		os.Remove(configFile)
		// Clean up environment variable BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
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

func TestAuthLogin_KeyAndProjectIDFlags(t *testing.T) {
	setupAuthTest(t)

	// Execute login command with public and secret key flags and project ID
	output, err := executeAuthCommand(t.Context(), "auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key", "--project-id", "test-project-123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-123)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify credentials were stored (try keyring first, then file fallback)
	// The combined key should be in format "public:secret"
	expectedAPIKey := "test-public-key:test-secret-key"
	expectedProjectID := "test-project-123"

	apiKey, projectID, err := config.GetCredentials()
	if err != nil {
		t.Fatalf("Credentials not stored in keyring or file: %v", err)
	}

	if apiKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, apiKey)
	}
	if projectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, projectID)
	}
}

func TestAuthLogin_KeyFlags_NoProjectID(t *testing.T) {
	setupAuthTest(t)

	// Execute login command with only public and secret key flags (no project ID)
	// This should fail since project ID is now required
	_, err := executeAuthCommand(t.Context(), "auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key")
	if err == nil {
		t.Fatal("Expected login to fail without project ID, but it succeeded")
	}

	// Verify the error message mentions TTY not detected
	expectedErrorMsg := "TTY not detected - credentials required"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error to contain %q, got: %v", expectedErrorMsg, err)
	}

	// Verify no credentials were stored since login failed
	if _, _, err = config.GetCredentials(); err == nil {
		t.Error("Credentials should not be stored when login fails")
	}
}

func TestAuthLogin_KeyAndProjectIDEnvironmentVariables(t *testing.T) {
	setupAuthTest(t)

	// Set environment variables for public and secret keys
	os.Setenv("TIGER_PUBLIC_KEY", "env-public-key")
	os.Setenv("TIGER_SECRET_KEY", "env-secret-key")
	os.Setenv("TIGER_PROJECT_ID", "env-project-id")
	defer os.Unsetenv("TIGER_PUBLIC_KEY")
	defer os.Unsetenv("TIGER_SECRET_KEY")
	defer os.Unsetenv("TIGER_PROJECT_ID")

	// Execute login command with project ID flag but using env vars for keys
	output, err := executeAuthCommand(t.Context(), "auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: env-project-id)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify credentials were stored
	expectedAPIKey := "env-public-key:env-secret-key"
	expectedProjectID := "env-project-id"
	storedKey, storedProjectID, err := config.GetCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
	if storedProjectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, storedProjectID)
	}
}

func TestAuthLogin_KeyEnvironmentVariables_ProjectIDFlag(t *testing.T) {
	setupAuthTest(t)

	// Set environment variables for public and secret keys
	os.Setenv("TIGER_PUBLIC_KEY", "env-public-key")
	os.Setenv("TIGER_SECRET_KEY", "env-secret-key")
	defer os.Unsetenv("TIGER_PUBLIC_KEY")
	defer os.Unsetenv("TIGER_SECRET_KEY")

	// Execute login command with project ID flag but using env vars for keys
	output, err := executeAuthCommand(t.Context(), "auth", "login", "--project-id", "test-project-456")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-456)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify credentials were stored
	expectedAPIKey := "env-public-key:env-secret-key"
	expectedProjectID := "test-project-456"
	storedKey, storedProjectID, err := config.GetCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
	if storedProjectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, storedProjectID)
	}
}

// setupOAuthTest creates a complete OAuth test environment with mock server and browser
func setupOAuthTest(t *testing.T, projects []Project, expectedProjectID string) string {
	t.Helper()
	tmpDir := setupAuthTest(t)

	// Ensure no keys in environment
	os.Unsetenv("TIGER_PUBLIC_KEY")
	os.Unsetenv("TIGER_SECRET_KEY")
	os.Unsetenv("TIGER_PROJECT_ID")

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
`, mockServer.URL, mockServer.URL)
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
func startMockOAuthServer(t *testing.T, projects []Project) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Token exchange endpoint
	mux.HandleFunc("POST /idp/external/cli/token", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock server received token exchange request")

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		clientID := r.FormValue("client_id")
		code := r.FormValue("code")
		codeVerifier := r.FormValue("code_verifier")

		if clientID == "" || code == "" || codeVerifier == "" {
			http.Error(w, "Missing required parameters", http.StatusBadRequest)
			return
		}

		tokenResponse := map[string]interface{}{
			"access_token":  "mock-access-token-12345",
			"refresh_token": "mock-refresh-token-67890",
			"expires_at":    1234567890,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(tokenResponse); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	})

	// GraphQL endpoint for getUserProjects and other queries
	mux.HandleFunc("POST /query", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock server received GraphQL request")

		var requestBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, "Failed to decode request body", http.StatusBadRequest)
			return
		}

		query, ok := requestBody["query"].(string)
		if !ok {
			http.Error(w, "Missing query in request", http.StatusBadRequest)
			return
		}

		// Handle different GraphQL queries
		if strings.Contains(query, "getAllProjects") {
			response := GraphQLResponse[GetAllProjectsData]{
				Data: &GetAllProjectsData{
					GetAllProjects: projects,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			}
		} else if strings.Contains(query, "getUser") {
			response := GraphQLResponse[GetUserData]{
				Data: &GetUserData{
					GetUser: User{
						ID:    "user-456",
						Name:  "Test User",
						Email: "test@example.com",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			}
		} else if strings.Contains(query, "createPATRecord") {
			response := GraphQLResponse[CreatePATRecordData]{
				Data: &CreatePATRecordData{
					CreatePATRecord: PATRecordResponse{
						ClientCredentials: struct {
							AccessKey string `json:"accessKey"`
							SecretKey string `json:"secretKey"`
						}{
							AccessKey: "test-access-key",
							SecretKey: "test-secret-key",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			}
		} else {
			http.Error(w, "Unknown GraphQL query", http.StatusBadRequest)
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
			// Sleep to ensure the OAuth callback server is listening
			// This prevents "EOF" errors in CI when the server hasn't started yet
			time.Sleep(100 * time.Millisecond)

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

func TestAuthLogin_OAuth_SingleProject(t *testing.T) {
	mockServerURL := setupOAuthTest(t, []Project{
		{ID: "project-123", Name: "Test Project"},
	}, "project-123")

	// Execute login command - the mocked openBrowser will handle the callback automatically
	output, err := executeAuthCommand(t.Context(), "auth", "login")

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Build regex pattern to match the complete output
	// Auth URL format: http://mockserver/oauth/authorize?client_id=45e1b16d-e435-4049-97b2-8daad150818c&code_challenge=base64&code_challenge_method=S256&redirect_uri=http%3A%2F%2Flocalhost%3APORT%2Fcallback&response_type=code&state=randomstring
	expectedPattern := fmt.Sprintf(`^Auth URL is: %s/oauth/authorize\?client_id=45e1b16d-e435-4049-97b2-8daad150818c&code_challenge=[A-Za-z0-9_-]+&code_challenge_method=S256&redirect_uri=http%%3A%%2F%%2Flocalhost%%3A\d+%%2Fcallback&response_type=code&state=[A-Za-z0-9_-]+\n`+
		`Opening browser for authentication\.\.\.\n`+
		`Validating API key\.\.\.\n`+
		`Successfully logged in \(project: project-123\)\n`+
		regexp.QuoteMeta(nextStepsMessage)+`$`, regexp.QuoteMeta(mockServerURL))

	matched, err := regexp.MatchString(expectedPattern, output)
	if err != nil {
		t.Fatalf("Regex compilation failed: %v", err)
	}
	if !matched {
		t.Errorf("Output doesn't match expected pattern.\nPattern: %s\nActual output: '%s'", expectedPattern, output)
	}

	// Verify credentials were stored
	expectedAPIKey := "test-access-key:test-secret-key"
	expectedProjectID := "project-123"
	storedKey, storedProjectID, err := config.GetCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
	if storedProjectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, storedProjectID)
	}
}

func TestAuthLogin_OAuth_MultipleProjects(t *testing.T) {
	mockServerURL := setupOAuthTest(t, []Project{
		{ID: "project-123", Name: "Test Project 1"},
		{ID: "project-456", Name: "Test Project 2"},
		{ID: "project-789", Name: "Test Project 3"},
	}, "project-789")

	// Mock the project selection to simulate user selecting the third project (index 2)
	originalSelectProjectInteractively := selectProjectInteractively
	defer func() {
		selectProjectInteractively = originalSelectProjectInteractively
	}()

	selectProjectInteractively = func(projects []Project, out io.Writer) (string, error) {
		t.Logf("Mock project selection - user selects project at index 2: %s", projects[2].ID)
		// Simulate user pressing down arrow twice and then enter (selects third project)
		return projects[2].ID, nil
	}

	// Execute login command - both mocked functions will handle OAuth flow and project selection
	output, err := executeAuthCommand(t.Context(), "auth", "login")

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Build regex pattern to match the complete output
	expectedPattern := fmt.Sprintf(`^Auth URL is: %s/oauth/authorize\?client_id=45e1b16d-e435-4049-97b2-8daad150818c&code_challenge=[A-Za-z0-9_-]+&code_challenge_method=S256&redirect_uri=http%%3A%%2F%%2Flocalhost%%3A\d+%%2Fcallback&response_type=code&state=[A-Za-z0-9_-]+\n`+
		`Opening browser for authentication\.\.\.\n`+
		`Validating API key\.\.\.\n`+
		`Successfully logged in \(project: project-789\)\n`+
		regexp.QuoteMeta(nextStepsMessage)+`$`, regexp.QuoteMeta(mockServerURL))

	matched, err := regexp.MatchString(expectedPattern, output)
	if err != nil {
		t.Fatalf("Regex compilation failed: %v", err)
	}
	if !matched {
		t.Errorf("Output doesn't match expected pattern.\nPattern: %s\nActual output: '%s'", expectedPattern, output)
	}

	// Verify credentials were stored (should be the third project - project-789)
	expectedAPIKey := "test-access-key:test-secret-key"
	expectedProjectID := "project-789"
	storedKey, storedProjectID, err := config.GetCredentials()
	if err != nil {
		t.Fatalf("Failed to get stored credentials: %v", err)
	}
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

	// Execute login command with public and secret key flags and project ID
	output, err := executeAuthCommand(t.Context(), "auth", "login", "--public-key", "fallback-public", "--secret-key", "fallback-secret", "--project-id", "test-project-fallback")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-fallback)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Force test file storage scenario by directly checking file
	credentialsFile := filepath.Join(tmpDir, "credentials")

	// If keyring worked, manually create file scenario by clearing all credentials and adding to file
	config.RemoveCredentials()

	// Store to file manually to simulate fallback
	expectedAPIKey := "fallback-public:fallback-secret"
	expectedProjectID := "test-project-fallback"
	if err := config.StoreCredentialsToFile(expectedAPIKey, expectedProjectID); err != nil {
		t.Fatalf("Failed to store credentials to file: %v", err)
	}

	// Verify file storage works
	storedKey, storedProjectID, err := config.GetCredentials()
	if err != nil {
		t.Fatalf("Failed to get credentials from file fallback: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
	if storedProjectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, storedProjectID)
	}

	// Test status with file-only storage
	output, err = executeAuthCommand(t.Context(), "auth", "status")
	if err != nil {
		t.Fatalf("Status failed with file storage: %v", err)
	}
	if output != "Logged in (API key stored)\nProject ID: test-project-fallback\n" {
		t.Errorf("Unexpected status output: '%s'", output)
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

	// Set environment variables for public key, secret key, and project ID
	os.Setenv("TIGER_PUBLIC_KEY", "env-file-public")
	os.Setenv("TIGER_SECRET_KEY", "env-file-secret")
	os.Setenv("TIGER_PROJECT_ID", "test-project-env-file")
	defer os.Unsetenv("TIGER_PUBLIC_KEY")
	defer os.Unsetenv("TIGER_SECRET_KEY")
	defer os.Unsetenv("TIGER_PROJECT_ID")

	// Execute login command without any flags (all from env vars)
	output, err := executeAuthCommand(t.Context(), "auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in (project: test-project-env-file)\n" + nextStepsMessage
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Clear all credentials to ensure we're testing file-only retrieval
	config.RemoveCredentials()

	// Verify credentials were stored in file (since we'll manually write to file only)
	expectedAPIKey := "env-file-public:env-file-secret"
	expectedProjectID := "test-project-env-file"

	// Store to file manually to simulate fallback scenario
	if err := config.StoreCredentialsToFile(expectedAPIKey, expectedProjectID); err != nil {
		t.Fatalf("Failed to store credentials to file: %v", err)
	}

	// Verify getCredentials works with file-only storage
	storedKey, storedProjectID, err := config.GetCredentials()
	if err != nil {
		t.Fatalf("Failed to get credentials from file: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
	if storedProjectID != expectedProjectID {
		t.Errorf("Expected project ID '%s', got '%s'", expectedProjectID, storedProjectID)
	}
}

func TestAuthStatus_LoggedIn(t *testing.T) {
	setupAuthTest(t)

	// Store credentials first
	err := config.StoreCredentials("test-api-key-789", "test-project-789")
	if err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}

	// Execute status command
	output, err := executeAuthCommand(t.Context(), "auth", "status")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if output != "Logged in (API key stored)\nProject ID: test-project-789\n" {
		t.Errorf("Unexpected output: '%s' (len=%d)", output, len(output))
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
	_, _, err = config.GetCredentials()
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
	_, _, err = config.GetCredentials()
	if err == nil {
		t.Fatal("Credentials should be removed after logout")
	}
}
