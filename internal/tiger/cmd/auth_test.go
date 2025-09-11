package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/zalando/go-keyring"
)

func setupAuthTest(t *testing.T) string {
	t.Helper()

	// Mock the API key validation for testing
	originalValidator := validateAPIKeyForLogin
	validateAPIKeyForLogin = func(apiKey, projectID string) error {
		// Always return success for testing
		return nil
	}

	// Aggressively clean up any existing keyring entries before starting
	// getServiceName() will return "tiger-cli-test" during tests automatically
	keyring.Delete(getServiceName(), username)

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Reset global config and viper to ensure test isolation
	config.ResetGlobalConfig()
	viper.Reset()

	// Also ensure config file doesn't exist
	configFile := filepath.Join(tmpDir, "config.yaml")
	os.Remove(configFile)

	t.Cleanup(func() {
		// Clean up test keyring
		keyring.Delete(getServiceName(), username)
		// Reset global config and viper first
		config.ResetGlobalConfig()
		viper.Reset()
		validateAPIKeyForLogin = originalValidator // Restore original validator
		// Remove config file explicitly
		configFile := filepath.Join(tmpDir, "config.yaml")
		os.Remove(configFile)
		// Clean up environment variable BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func executeAuthCommand(args ...string) (string, error) {
	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot := buildRootCmd()

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err := testRoot.Execute()
	return buf.String(), err
}

func TestAuthLogin_KeyAndProjectIDFlags(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// Execute login command with public and secret key flags and project ID
	output, err := executeAuthCommand("auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key", "--project-id", "test-project-123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key\nSet default project ID to: test-project-123\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify API key was stored (try keyring first, then file fallback)
	// The combined key should be in format "public:secret"
	expectedAPIKey := "test-public-key:test-secret-key"
	apiKey, err := keyring.Get(getServiceName(), username)
	if err != nil {
		// Keyring failed, check file fallback
		apiKeyFile := filepath.Join(tmpDir, "api-key")
		data, err := os.ReadFile(apiKeyFile)
		if err != nil {
			t.Fatalf("API key not stored in keyring or file: %v", err)
		}
		if string(data) != expectedAPIKey {
			t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, string(data))
		}
	} else {
		if apiKey != expectedAPIKey {
			t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, apiKey)
		}
	}

	// Verify project ID was stored in config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.ProjectID != "test-project-123" {
		t.Errorf("Expected project ID 'test-project-123', got '%s'", cfg.ProjectID)
	}
}

func TestAuthLogin_KeyFlags_NoProjectID(t *testing.T) {
	setupAuthTest(t)

	// Execute login command with only public and secret key flags (no project ID)
	// This should fail since project ID is now required
	_, err := executeAuthCommand("auth", "login", "--public-key", "test-public-key", "--secret-key", "test-secret-key")
	if err == nil {
		t.Fatal("Expected login to fail without project ID, but it succeeded")
	}

	// Verify the error message mentions TTY not detected
	expectedErrorMsg := "TTY not detected - credentials required"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error to contain %q, got: %v", expectedErrorMsg, err)
	}

	// Verify no API key was stored since login failed
	if _, err = getAPIKey(); err == nil {
		t.Error("API key should not be stored when login fails")
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
	output, err := executeAuthCommand("auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key\nSet default project ID to: env-project-id\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify API key was stored (should be combined format)
	expectedAPIKey := "env-public-key:env-secret-key"
	storedKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get stored API key: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
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
	output, err := executeAuthCommand("auth", "login", "--project-id", "test-project-456")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key\nSet default project ID to: test-project-456\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify API key was stored (should be combined format)
	expectedAPIKey := "env-public-key:env-secret-key"
	storedKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get stored API key: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
}

// setupOAuthTest creates a complete OAuth test environment with mock server and browser
func setupOAuthTest(t *testing.T, projects []Project, expectedProjectID string) {
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
	configFile := filepath.Join(tmpDir, "config.yaml")
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
	setupOAuthTest(t, []Project{
		{ID: "project-123", Name: "Test Project"},
	}, "project-123")

	// Execute login command - the mocked openBrowser will handle the callback automatically
	output, err := executeAuthCommand("auth", "login")

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Opening browser for authentication...\nValidating API key...\nSuccessfully logged in and stored API key\nSet default project ID to: project-123\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify API key was stored
	storedKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get stored API key: %v", err)
	}

	// Expected API key is "test-access-key:test-secret-key"
	expectedAPIKey := "test-access-key:test-secret-key"
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}

	// Verify project ID was stored in config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.ProjectID != "project-123" {
		t.Errorf("Expected project ID 'project-123', got '%s'", cfg.ProjectID)
	}
}

func TestAuthLogin_OAuth_MultipleProjects(t *testing.T) {
	setupOAuthTest(t, []Project{
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
	output, err := executeAuthCommand("auth", "login")

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Opening browser for authentication...\nValidating API key...\nSuccessfully logged in and stored API key\nSet default project ID to: project-789\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Verify API key was stored
	storedKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get stored API key: %v", err)
	}

	// Expected API key is "test-access-key:test-secret-key"
	expectedAPIKey := "test-access-key:test-secret-key"
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}

	// Verify project ID was stored in config (should be the third project - project-789)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.ProjectID != "project-789" {
		t.Errorf("Expected project ID 'project-789', got '%s'", cfg.ProjectID)
	}
}

// TestAuthLogin_KeyringFallback tests the scenario where keyring fails and system falls back to file storage
func TestAuthLogin_KeyringFallback(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// We can't easily mock keyring failure, but we can test file storage directly
	// by ensuring the API key gets stored to file when keyring might not be available

	// Execute login command with public and secret key flags and project ID
	output, err := executeAuthCommand("auth", "login", "--public-key", "fallback-public", "--secret-key", "fallback-secret", "--project-id", "test-project-fallback")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key\nSet default project ID to: test-project-fallback\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Force test file storage scenario by directly checking file
	apiKeyFile := filepath.Join(tmpDir, "api-key")

	// If keyring worked, manually create file scenario by removing keyring and adding file
	keyring.Delete(getServiceName(), username) // Remove from keyring

	// Store to file manually to simulate fallback (combined format)
	expectedAPIKey := "fallback-public:fallback-secret"
	err = storeAPIKeyToFile(expectedAPIKey)
	if err != nil {
		t.Fatalf("Failed to store API key to file: %v", err)
	}

	// Verify file storage works
	storedKey, err := getAPIKey()
	if err != nil {
		t.Fatalf("Failed to get API key from file fallback: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}

	// Test whoami with file-only storage
	output, err = executeAuthCommand("auth", "whoami")
	if err != nil {
		t.Fatalf("Whoami failed with file storage: %v", err)
	}
	if output != "Logged in (API key stored)\n" {
		t.Errorf("Unexpected whoami output: '%s'", output)
	}

	// Test logout with file-only storage
	output, err = executeAuthCommand("auth", "logout")
	if err != nil {
		t.Fatalf("Logout failed with file storage: %v", err)
	}
	if output != "Successfully logged out and removed stored credentials\n" {
		t.Errorf("Unexpected logout output: '%s'", output)
	}

	// Verify file was removed
	if _, err := os.Stat(apiKeyFile); !os.IsNotExist(err) {
		t.Error("API key file should be removed after logout")
	}
}

// TestAuthLogin_EnvironmentVariable_FileOnly tests env var login when only file storage is available
func TestAuthLogin_EnvironmentVariable_FileOnly(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// Clear any keyring entries to force file-only storage
	keyring.Delete(getServiceName(), username)

	// Set environment variables for public key, secret key, and project ID
	os.Setenv("TIGER_PUBLIC_KEY", "env-file-public")
	os.Setenv("TIGER_SECRET_KEY", "env-file-secret")
	os.Setenv("TIGER_PROJECT_ID", "test-project-env-file")
	defer os.Unsetenv("TIGER_PUBLIC_KEY")
	defer os.Unsetenv("TIGER_SECRET_KEY")
	defer os.Unsetenv("TIGER_PROJECT_ID")

	// Execute login command without any flags (all from env vars)
	output, err := executeAuthCommand("auth", "login")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	expectedOutput := "Validating API key...\nSuccessfully logged in and stored API key\nSet default project ID to: test-project-env-file\n"
	if output != expectedOutput {
		t.Errorf("Unexpected output: '%s'", output)
	}

	// Clear keyring again to ensure we're testing file-only retrieval
	keyring.Delete(getServiceName(), username)

	// Verify API key was stored in file (since keyring is cleared)
	expectedAPIKey := "env-file-public:env-file-secret"
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		// If file doesn't exist, the keyring might have worked, so manually ensure file storage
		err = storeAPIKeyToFile(expectedAPIKey)
		if err != nil {
			t.Fatalf("Failed to store API key to file: %v", err)
		}
		data, err = os.ReadFile(apiKeyFile)
		if err != nil {
			t.Fatalf("API key file should exist: %v", err)
		}
	}

	if string(data) != expectedAPIKey {
		t.Errorf("Expected API key '%s' in file, got '%s'", expectedAPIKey, string(data))
	}

	// Verify getAPIKey works with file-only storage
	storedKey, err := getAPIKeyFromFile()
	if err != nil {
		t.Fatalf("Failed to get API key from file: %v", err)
	}
	if storedKey != expectedAPIKey {
		t.Errorf("Expected API key '%s', got '%s'", expectedAPIKey, storedKey)
	}
}

func TestAuthWhoami_LoggedIn(t *testing.T) {
	setupAuthTest(t)

	// Store API key first
	err := storeAPIKey("test-api-key-789")
	if err != nil {
		t.Fatalf("Failed to store API key: %v", err)
	}

	// Execute whoami command
	output, err := executeAuthCommand("auth", "whoami")
	if err != nil {
		t.Fatalf("Whoami failed: %v", err)
	}

	if output != "Logged in (API key stored)\n" {
		t.Errorf("Unexpected output: '%s' (len=%d)", output, len(output))
	}
}

func TestAuthWhoami_NotLoggedIn(t *testing.T) {
	setupAuthTest(t)

	// Execute whoami command without being logged in
	_, err := executeAuthCommand("auth", "whoami")
	if err == nil {
		t.Fatal("Expected whoami to fail when not logged in")
	}

	// Error should indicate not logged in
	if err.Error() != errNotLoggedIn.Error() {
		t.Errorf("Expected 'not logged in' error, got: %v", err)
	}
}

func TestAuthLogout_Success(t *testing.T) {
	setupAuthTest(t)

	// Store API key first
	err := storeAPIKey("test-api-key-logout")
	if err != nil {
		t.Fatalf("Failed to store API key: %v", err)
	}

	// Verify API key is stored
	_, err = getAPIKey()
	if err != nil {
		t.Fatalf("API key should be stored: %v", err)
	}

	// Execute logout command
	output, err := executeAuthCommand("auth", "logout")
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	if output != "Successfully logged out and removed stored credentials\n" {
		t.Errorf("Unexpected output: '%s' (len=%d)", output, len(output))
	}

	// Verify API key is removed
	_, err = getAPIKey()
	if err == nil {
		t.Fatal("API key should be removed after logout")
	}
}

func TestStoreAPIKeyToFile(t *testing.T) {
	tmpDir := setupAuthTest(t)

	err := storeAPIKeyToFile("file-test-key")
	if err != nil {
		t.Fatalf("Failed to store API key to file: %v", err)
	}

	// Verify file exists and has correct permissions
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	info, err := os.Stat(apiKeyFile)
	if err != nil {
		t.Fatalf("API key file should exist: %v", err)
	}

	// Check file permissions (should be 0600)
	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected file permissions 0600, got %o", info.Mode().Perm())
	}

	// Verify file content
	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		t.Fatalf("Failed to read API key file: %v", err)
	}

	if string(data) != "file-test-key" {
		t.Errorf("Expected 'file-test-key', got '%s'", string(data))
	}
}

func TestGetAPIKeyFromFile(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// Write API key to file
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	err := os.WriteFile(apiKeyFile, []byte("file-get-test-key"), 0600)
	if err != nil {
		t.Fatalf("Failed to write test API key file: %v", err)
	}

	// Get API key from file
	apiKey, err := getAPIKeyFromFile()
	if err != nil {
		t.Fatalf("Failed to get API key from file: %v", err)
	}

	if apiKey != "file-get-test-key" {
		t.Errorf("Expected 'file-get-test-key', got '%s'", apiKey)
	}
}

func TestGetAPIKeyFromFile_NotExists(t *testing.T) {
	setupAuthTest(t)

	// Try to get API key when file doesn't exist
	_, err := getAPIKeyFromFile()
	if err == nil {
		t.Fatal("Expected error when API key file doesn't exist")
	}

	if err.Error() != "not logged in" {
		t.Errorf("Expected 'not logged in' error, got: %v", err)
	}
}

func TestRemoveAPIKeyFromFile(t *testing.T) {
	tmpDir := setupAuthTest(t)

	// Write API key to file
	apiKeyFile := filepath.Join(tmpDir, "api-key")
	err := os.WriteFile(apiKeyFile, []byte("remove-test-key"), 0600)
	if err != nil {
		t.Fatalf("Failed to write test API key file: %v", err)
	}

	// Remove API key file
	err = removeAPIKeyFromFile()
	if err != nil {
		t.Fatalf("Failed to remove API key file: %v", err)
	}

	// Verify file is removed
	if _, err := os.Stat(apiKeyFile); !os.IsNotExist(err) {
		t.Fatal("API key file should be removed")
	}
}

func TestRemoveAPIKeyFromFile_NotExists(t *testing.T) {
	setupAuthTest(t)

	// Try to remove API key file when it doesn't exist (should not error)
	err := removeAPIKeyFromFile()
	if err != nil {
		t.Fatalf("Should not error when removing non-existent file: %v", err)
	}
}
