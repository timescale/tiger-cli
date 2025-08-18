package cmd

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"

	"github.com/tigerdata/tiger-cli/internal/tiger/api"
	"github.com/tigerdata/tiger-cli/internal/tiger/config"
)

// Keyring parameters
const (
	serviceName = "tiger-cli"
	username    = "api-key"
)

// getServiceName returns the appropriate service name for keyring operations
// Uses a test-specific service name when running in test mode to avoid polluting the real keyring
func getServiceName() string {
	// Check if we're running in a test - look for .test suffix in the binary name
	if strings.HasSuffix(os.Args[0], ".test") {
		return "tiger-cli-test"
	}
	// Also check for test-specific arguments
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") {
			return "tiger-cli-test"
		}
	}
	return serviceName
}

// OAuth parameters
// TODO: Currently unused, but should probably support
const clientID = "65183398-ece9-40c4-84e7-84974c51255b"

var (
	// validateAPIKeyForLogin can be overridden for testing
	validateAPIKeyForLogin = api.ValidateAPIKey

	// openBrowser can be overridden for testing
	openBrowser = openBrowserImpl

	// selectProjectInteractively can be overridden for testing
	selectProjectInteractively = selectProjectInteractivelyImpl
)

func buildLoginCmd() *cobra.Command {
	var publicKeyFlag string
	var secretKeyFlag string
	var projectIDFlag string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with TigerData API",
		Long: `Authenticate with TigerData API using predefined keys or an interactive OAuth flow

By default, the command will launch an interactive OAuth flow in your browser to create new API keys.
The OAuth flow will:
- Open your browser for authentication
- Let you select a project (if you have multiple)
- Create API keys automatically for the selected project

The keys will be combined and stored securely in the system keyring or as a fallback file.
The project ID will be stored in the configuration file.

You may also provide API keys via flags or environment variables, in which case
they will be used directly. The CLI will prompt for any missing information.

You can find your API credentials and project ID at: https://console.timescale.com/dashboard/settings

Examples:
  # Interactive login with OAuth (opens browser, creates API keys automatically)
  tiger auth login

  # Login with project ID (will prompt for keys if not provided)
  tiger auth login --project-id your-project-id

  # Login with keys and project ID
  tiger auth login --public-key your-public-key --secret-key your-secret-key --project-id your-project-id

  # Login using environment variables
  export TIGER_PUBLIC_KEY="your-public-key"
  export TIGER_SECRET_KEY="your-secret-key"
  export TIGER_PROJECT_ID="proj-123"
  tiger auth login`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			c := &loginCmd{
				cfg: cfg,
				out: cmd.OutOrStdout(),
			}

			publicKey := publicKeyFlag
			if publicKey == "" {
				publicKey = os.Getenv("TIGER_PUBLIC_KEY")
			}

			secretKey := secretKeyFlag
			if secretKey == "" {
				secretKey = os.Getenv("TIGER_SECRET_KEY")
			}

			// TODO: It should be possible to get the projectID corresponding to the
			// API keys programmatically, making the project-id flag unnecessary
			projectID := projectIDFlag
			if projectID == "" {
				projectID = os.Getenv("TIGER_PROJECT_ID")
			}

			apiKey := fmt.Sprintf("%s:%s", publicKey, secretKey)
			if publicKey == "" && secretKey == "" && projectID == "" {
				apiKey, projectID, err = c.loginWithOAuth()
				if err != nil {
					return err
				}
			} else if publicKey == "" || secretKey == "" || projectID == "" {
				// If any credentials are missing, prompt for them all at once
				publicKey, secretKey, projectID, err = promptForCredentials(publicKey, secretKey, projectID)
				if err != nil {
					return fmt.Errorf("failed to get credentials: %w", err)
				}

				if publicKey == "" || secretKey == "" {
					return fmt.Errorf("both public key and secret key are required")
				}

				if projectID == "" {
					return fmt.Errorf("project ID is required")
				}

				// Combine the keys in the format "public:secret" for storage
				apiKey = fmt.Sprintf("%s:%s", publicKey, secretKey)
			}

			// Validate the API key by making a test API call
			fmt.Fprintln(c.out, "Validating API key...")
			if err := validateAPIKeyForLogin(apiKey, projectID); err != nil {
				return fmt.Errorf("API key validation failed: %w", err)
			}

			// Store the API key securely
			if err := storeAPIKey(apiKey); err != nil {
				return fmt.Errorf("failed to store API key: %w", err)
			}
			fmt.Fprintln(c.out, "Successfully logged in and stored API key")

			// Store project ID in config if provided
			if err := storeProjectID(projectID); err != nil {
				return fmt.Errorf("failed to store project ID: %w", err)
			}
			fmt.Fprintf(c.out, "Set default project ID to: %s\n", projectID)

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&publicKeyFlag, "public-key", "", "Public key for authentication")
	cmd.Flags().StringVar(&secretKeyFlag, "secret-key", "", "Secret key for authentication")
	cmd.Flags().StringVar(&projectIDFlag, "project-id", "", "Default project ID to set in configuration")

	return cmd
}

func buildLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials",
		Long:  `Remove stored API key and clear authentication credentials.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			if err := removeAPIKey(); err != nil {
				return fmt.Errorf("failed to remove API key: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Successfully logged out and removed stored credentials")
			return nil
		},
	}
}

func buildWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current user information",
		Long:  `Show information about the currently authenticated user.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			if _, err := getAPIKey(); err != nil {
				return err
			}

			// TODO: Make API call to get user information
			fmt.Fprintln(cmd.OutOrStdout(), "Logged in (API key stored)")

			return nil
		},
	}
}

func buildAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication and credentials",
		Long:  `Manage authentication and credentials for TigerData Cloud Platform.`,
	}

	cmd.AddCommand(buildLoginCmd())
	cmd.AddCommand(buildLogoutCmd())
	cmd.AddCommand(buildWhoamiCmd())

	return cmd
}

type loginCmd struct {
	cfg *config.Config
	out io.Writer
}

func (c *loginCmd) loginWithOAuth() (string, string, error) {
	// Get a user access token via OAuth
	accessToken, err := c.oauthFlow()
	if err != nil {
		return "", "", fmt.Errorf("failed to authenticate via OAuth: %w", err)
	}

	// Get the user's project ID (with interactive selection if needed)
	projectID, err := c.selectProjectID(accessToken)
	if err != nil {
		return "", "", fmt.Errorf("failed to select project: %w", err)
	}

	// Create API key for the selected project
	apiKey, err := c.createAPIKey(accessToken, projectID)
	if err != nil {
		return "", "", fmt.Errorf("failed to create API key: %w", err)
	}

	return apiKey, projectID, nil

}

func (c *loginCmd) oauthFlow() (string, error) {
	// Generate PKCE parameters
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return "", fmt.Errorf("failed to generate PKCE code verifier: %w", err)
	}
	codeChallenge := generateCodeChallenge(codeVerifier)

	// Generate random state string (to guard against CRSF attacks)
	state, err := generateRandomState(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate random state: %w", err)
	}

	// Start local HTTP server for handling the OAuth callback
	server, err := c.startOAuthServer(state, codeVerifier)
	if err != nil {
		return "", fmt.Errorf("failed to create local server: %w", err)
	}
	defer func() {
		if err := server.Close(); err != nil {
			fmt.Fprintf(c.out, "Failed to close local server: %s\n", err)
		}
	}()

	// Construct authorization URL
	authParams := url.Values{}
	authParams.Set("clientId", clientID)
	authParams.Set("redirectUri", server.redirectURI)
	authParams.Set("responseType", "code")
	authParams.Set("codeChallenge", codeChallenge)
	authParams.Set("codeChallengeMethod", "S256")
	authParams.Set("state", state)

	authURL := c.cfg.ConsoleURL + "/oauth/authorize" + "?" + authParams.Encode()

	// Open browser
	fmt.Fprintln(c.out, "Opening browser for authentication...")
	if err := openBrowser(authURL); err != nil {
		fmt.Fprintf(c.out, "Failed to open browser: %s\nPlease manually navigate to: %s\n", err, authURL)
	}

	// Wait for callback with timeout
	select {
	case result := <-server.resultChan:
		return result.accessToken, result.err
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("authorization timeout - no callback received within 5 minutes")
	}
}

func generateCodeVerifier() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(data), nil
}

func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
}

func generateRandomState(length int) (string, error) {
	data := make([]byte, length)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(data)[:length], nil
}

type oauthServer struct {
	listener    net.Listener
	server      *http.Server
	redirectURI string
	resultChan  <-chan oauthResult
}

type oauthResult struct {
	accessToken string
	err         error
}

func (s *oauthServer) Close() error {
	doClose := func(closer io.Closer) error {
		if err := closer.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
		return nil
	}

	return errors.Join(
		doClose(s.server),
		doClose(s.listener),
	)
}

func (c *loginCmd) startOAuthServer(state, codeVerifier string) (*oauthServer, error) {
	// Start listening on an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen on local port: %w", err)
	}

	// Build redirect URI
	port := listener.Addr().(*net.TCPAddr).Port
	redirectUri := fmt.Sprintf("http://localhost:%d/callback", port)

	// Start local HTTP server for callback
	resultChan := make(chan oauthResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", c.createOAuthCallbackHandler(state, codeVerifier, resultChan))

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			resultChan <- oauthResult{
				err: fmt.Errorf("failed to serve requests: %w", err),
			}
		}
	}()

	return &oauthServer{
		server:      server,
		listener:    listener,
		redirectURI: redirectUri,
		resultChan:  resultChan,
	}, nil
}

func (c *loginCmd) createOAuthCallbackHandler(expectedState, codeVerifier string, resultChan chan<- oauthResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		// Validate state parameter
		state := query.Get("state")
		if state != expectedState {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Invalid state parameter")
			resultChan <- oauthResult{err: fmt.Errorf("invalid state parameter")}
			return
		}

		// Get authorization code
		code := query.Get("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Missing authorization code")
			resultChan <- oauthResult{err: fmt.Errorf("missing authorization code in callback")}
			return
		}

		// Exchange authorization code for tokens
		accessToken, err := c.exchangeCodeForAccessToken(code, codeVerifier, clientID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed to exchange authorization code for tokens")
			resultChan <- oauthResult{err: fmt.Errorf("failed to exchange code for tokens: %w", err)}
			return
		}

		// Redirect to success page
		successURL := c.cfg.ConsoleURL + "/oauth/code/success"
		http.Redirect(w, r, successURL, http.StatusTemporaryRedirect)

		resultChan <- oauthResult{
			accessToken: accessToken,
		}
	}
}

func (c *loginCmd) exchangeCodeForAccessToken(code, codeVerifier, clientID string) (string, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("code", code)
	data.Set("code_verifier", codeVerifier)

	tokenURL := c.cfg.GatewayURL + "/idp/external/cli/token"
	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("failed to make token exchange request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var tokenResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int    `json:"expires_at"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response body: %w", err)
	}

	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResponse.AccessToken == "" {
		return "", fmt.Errorf("no access token received")
	}

	return tokenResponse.AccessToken, nil
}

// selectProjectID prompts the user to select a project if multiple are available
func (c *loginCmd) selectProjectID(accessToken string) (string, error) {
	// First, get the list of projects the user has access to
	projects, err := c.getUserProjects(accessToken)
	if err != nil {
		return "", fmt.Errorf("failed to get user projects: %w", err)
	}

	switch len(projects) {
	case 0:
		return "", fmt.Errorf("user has no accessible projects")
	case 1:
		return projects[0].ID, nil
	default:
		return selectProjectInteractively(projects, c.out)
	}
}

// selectProjectInteractivelyImpl is the default implementation for project selection using Bubble Tea
func selectProjectInteractivelyImpl(projects []Project, out io.Writer) (string, error) {
	model := projectSelectModel{
		projects: projects,
		cursor:   0,
	}

	program := tea.NewProgram(model, tea.WithOutput(out))
	finalModel, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run project selection: %w", err)
	}

	result := finalModel.(projectSelectModel)
	if result.selected == "" {
		return "", fmt.Errorf("no project selected")
	}

	return result.selected, nil
}

// projectSelectModel represents the Bubble Tea model for project selection
type projectSelectModel struct {
	projects []Project
	cursor   int
	selected string
}

func (m projectSelectModel) Init() tea.Cmd {
	return nil
}

func (m projectSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.selected = m.projects[m.cursor].ID
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m projectSelectModel) View() string {
	s := "Select a project:\n\n"

	for i, project := range m.projects {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %s (%s)\n", cursor, project.Name, project.ID)
	}

	s += "\nUse ↑/↓ arrows to navigate, Enter to select, q to quit"
	return s
}

// createAPIKey creates an API key for the selected project
func (c *loginCmd) createAPIKey(accessToken, projectID string) (string, error) {
	// Get user information for PAT name
	user, err := c.getUser(accessToken)
	if err != nil {
		return "", fmt.Errorf("failed to get user info: %w", err)
	}

	// Create a PAT record for this project
	patRecord, err := c.createPATRecord(accessToken, projectID, c.buildPATName(user))
	if err != nil {
		return "", fmt.Errorf("failed to create PAT record: %w", err)
	}

	// Combine access key and secret key with colon
	return fmt.Sprintf("%s:%s", patRecord.ClientCredentials.AccessKey, patRecord.ClientCredentials.SecretKey), nil
}

// Build the PAT/Client Credentials name. This is displayed under "Project settings"
// in the console and helps identify what the credentials are for.
func (c *loginCmd) buildPATName(user *User) string {
	// Use user name, fallback to email if name is empty,
	// fallback to hostname as last resort.
	if user.Name != "" {
		return "TigerCLI - " + user.Name
	} else if user.Email != "" {
		return "TigerCLI - " + user.Email
	} else if hostname, _ := os.Hostname(); hostname != "" {
		return "TigerCLI - " + hostname
	} else {
		return "TigerCLI"
	}
}

// GetAllProjectsResponse represents the response from the getAllProjects query
type GetAllProjectsResponse struct {
	Data struct {
		GetAllProjects []Project `json:"getAllProjects"`
	} `json:"data"`
}

// Project represents a project from the GraphQL API
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// getUserProjects fetches the list of projects the user has access to
func (c *loginCmd) getUserProjects(accessToken string) ([]Project, error) {
	query := `
		query GetAllProjects {
			getAllProjects {
				id
				name
			}
		}
	`

	response, err := makeGraphQLRequest[GetAllProjectsResponse](c.cfg.GatewayURL, accessToken, query, nil)
	if err != nil {
		return nil, err
	}

	return response.Data.GetAllProjects, nil
}

// GetUserResponse represents the response from the getUser query
type GetUserResponse struct {
	Data struct {
		GetUser User `json:"getUser"`
	} `json:"data"`
}

// User represents a user from the GraphQL API
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// getUser fetches the current user's information
func (c *loginCmd) getUser(accessToken string) (*User, error) {
	query := `
		query GetUser {
			getUser {
				id
				name
				email
			}
		}
	`

	response, err := makeGraphQLRequest[GetUserResponse](c.cfg.GatewayURL, accessToken, query, nil)
	if err != nil {
		return nil, err
	}

	return &response.Data.GetUser, nil
}

// CreatePATRecordResponse represents the response from the createPATRecord mutation
type CreatePATRecordResponse struct {
	Data struct {
		CreatePATRecord PATRecordResponse `json:"createPATRecord"`
	} `json:"data"`
}

// PATRecordResponse represents the response from creating a PAT record
type PATRecordResponse struct {
	ClientCredentials struct {
		AccessKey string `json:"accessKey"`
		SecretKey string `json:"secretKey"`
	} `json:"clientCredentials"`
}

// createPATRecord creates a new PAT record for the given project
func (c *loginCmd) createPATRecord(accessToken, projectID, patName string) (*PATRecordResponse, error) {
	query := `
		mutation CreatePATRecord($input: CreatePATRecordInput!) {
			createPATRecord(createPATRecordInput: $input) {
				clientCredentials {
					accessKey
					secretKey
				}
			}
		}
	`

	variables := map[string]any{
		"input": map[string]any{
			"projectId": projectID,
			"name":      patName,
		},
	}

	response, err := makeGraphQLRequest[CreatePATRecordResponse](c.cfg.GatewayURL, accessToken, query, variables)
	if err != nil {
		return nil, err
	}

	return &response.Data.CreatePATRecord, nil
}

// makeGraphQLRequest makes a GraphQL request to the API using generics
func makeGraphQLRequest[T any](gatewayURL, accessToken, query string, variables map[string]any) (*T, error) {
	requestBody := map[string]any{
		"query": query,
	}
	if variables != nil {
		requestBody["variables"] = variables
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	queryURL := gatewayURL + "/query"
	req, err := http.NewRequest("POST", queryURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make GraphQL request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GraphQL response: %w", err)
	}

	var response T
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GraphQL response: %w", err)
	}

	return &response, nil
}

func openBrowserImpl(url string) error {
	var cmd string
	var args []string

	// TODO: Do all of these work correctly?
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// storeAPIKey stores the API key using keyring with file fallback
func storeAPIKey(apiKey string) error {
	// Try keyring first
	err := keyring.Set(getServiceName(), username, apiKey)
	if err == nil {
		return nil
	}

	// Fallback to file storage
	return storeAPIKeyToFile(apiKey)
}

// getAPIKey retrieves the API key from keyring or file fallback
func getAPIKey() (string, error) {
	// Try keyring first
	apiKey, err := keyring.Get(getServiceName(), username)
	if err == nil && apiKey != "" {
		return apiKey, nil
	}

	// Fallback to file storage
	return getAPIKeyFromFile()
}

// removeAPIKey removes the API key from keyring and file fallback
func removeAPIKey() error {
	// Try to remove from keyring (ignore errors as it might not exist)
	keyring.Delete(getServiceName(), username)

	// Remove from file fallback
	return removeAPIKeyFromFile()
}

// storeAPIKeyToFile stores API key to ~/.config/tiger/api-key with restricted permissions
func storeAPIKeyToFile(apiKey string) error {
	configDir := config.GetConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)
	file, err := os.OpenFile(apiKeyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create API key file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(apiKey); err != nil {
		return fmt.Errorf("failed to write API key to file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	return nil
}

var errNotLoggedIn = errors.New("not logged in")

// getAPIKeyFromFile retrieves API key from ~/.config/tiger/api-key
func getAPIKeyFromFile() (string, error) {
	configDir := config.GetConfigDir()
	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)

	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		// If the file does not exist, treat as not logged in
		if os.IsNotExist(err) {
			return "", errNotLoggedIn
		}
		return "", fmt.Errorf("failed to read API key file: %w", err)
	}

	apiKey := strings.TrimSpace(string(data))

	// If file exists but is empty, treat as not logged in
	if apiKey == "" {
		return "", errNotLoggedIn
	}

	return apiKey, nil
}

// removeAPIKeyFromFile removes the API key file
func removeAPIKeyFromFile() error {
	configDir := config.GetConfigDir()
	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)

	err := os.Remove(apiKeyFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove API key file: %w", err)
	}

	return nil
}

// promptForCredentials prompts the user to enter any missing credentials
func promptForCredentials(publicKey, secretKey, projectID string) (string, string, string, error) {
	// Check if we're in a terminal for interactive input
	if !term.IsTerminal(int(syscall.Stdin)) {
		return "", "", "", fmt.Errorf("TTY not detected - credentials required. Use flags (--public-key, --secret-key, --project-id) or environment variables (TIGER_PUBLIC_KEY, TIGER_SECRET_KEY, TIGER_PROJECT_ID)")
	}

	fmt.Println("You can find your API credentials and project ID at: https://console.timescale.com/dashboard/settings")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// Prompt for public key if missing
	if publicKey == "" {
		fmt.Print("Enter your public key: ")
		var err error
		publicKey, err = reader.ReadString('\n')
		if err != nil {
			return "", "", "", err
		}
		publicKey = strings.TrimSpace(publicKey)
	}

	// Prompt for secret key if missing
	if secretKey == "" {
		fmt.Print("Enter your secret key: ")
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return "", "", "", err
		}
		fmt.Println() // Print newline after hidden input
		secretKey = strings.TrimSpace(string(bytePassword))
	}

	// Prompt for project ID if missing
	if projectID == "" {
		fmt.Print("Enter your project ID: ")
		var err error
		projectID, err = reader.ReadString('\n')
		if err != nil {
			return "", "", "", err
		}
		projectID = strings.TrimSpace(projectID)
	}

	return publicKey, secretKey, projectID, nil
}

// storeProjectID stores the project ID in the configuration file
func storeProjectID(projectID string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	return cfg.Set("project_id", projectID)
}
