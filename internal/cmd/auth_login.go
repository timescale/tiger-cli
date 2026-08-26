package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// validateAPIKey can be overridden for testing
var validateAPIKey = common.ValidateAPIKey

// nextStepsMessage is the message shown after successful login
const nextStepsMessage = `
🎉 Next steps:
• Install MCP server for your favorite AI coding tool: tiger mcp install
• List existing services: tiger service list
• Create a new service: tiger service create
• Enable read-only mode: tiger config set read_only true
`

var (
	// openBrowser can be overridden for testing
	openBrowser = openBrowserImpl

	// selectProjectInteractively can be overridden for testing
	selectProjectInteractively = selectProjectInteractivelyImpl
)

type credentials struct {
	publicKey string
	secretKey string
}

func buildLoginCmd(app *common.App) *cobra.Command {
	var flags credentials
	var projectID string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Tiger Cloud API",
		Long: `Authenticate with Tiger Cloud API using predefined keys or an interactive OAuth flow

By default, the command will launch an interactive OAuth flow in your browser to sign in.
The OAuth flow will:
- Open your browser for authentication
- Let you select a project (if you have multiple)
- Store an OAuth session for the selected project

Use --project-id to pick the project up front and skip the interactive selection. After
logging in, you can switch projects with 'tiger project use'.

The credentials and project ID will be stored securely in the system keyring, or in a fallback file with
restricted permissions. Unless the login lands on the same project as the previous login, the default
service (config key service_id) is cleared, since it belongs to the project it was set in.

You may also provide API keys via flags or environment variables, in which case they will be used
directly. The CLI will prompt for any missing information.

You can find your API credentials at: https://console.cloud.tigerdata.com/dashboard/settings

Examples:
  # Interactive login with OAuth (opens browser)
  tiger auth login

  # OAuth login without the interactive project selection
  tiger auth login --project-id my-project-id

  # Login with keys (project ID will be auto-detected)
  tiger auth login --public-key your-public-key --secret-key your-secret-key

  # Login using environment variables
  export TIGER_PUBLIC_KEY="your-public-key"
  export TIGER_SECRET_KEY="your-secret-key"
  tiger auth login`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.GetConfig()

			var err error
			creds := credentials{
				publicKey: flagOrEnvVar(flags.publicKey, "TIGER_PUBLIC_KEY"),
				secretKey: flagOrEnvVar(flags.secretKey, "TIGER_SECRET_KEY"),
			}

			// Captured before the login replaces it; used to decide whether
			// the default service is stale.
			prevProjectID := ""
			if stored, err := cfg.GetStoredCredentials(); err == nil {
				prevProjectID = stored.ProjectID
			}

			if creds.publicKey == "" && creds.secretKey == "" {
				l := &oauthLogin{
					cfg:        cfg,
					authURL:    cfg.ConsoleURL + "/oauth/authorize",
					tokenURL:   cfg.GatewayURL + "/idp/external/cli/token",
					successURL: cfg.ConsoleURL + "/oauth/code/success",
					projectID:  projectID,
					cmd:        cmd,
				}

				token, client, projectID, err := l.loginWithOAuth(cmd.Context())
				if err != nil {
					return err
				}
				if err := cfg.StoreOAuthCredentials(token, projectID); err != nil {
					return fmt.Errorf("failed to store credentials: %w", err)
				}
				// Hand the freshly authenticated client to the App so later
				// readers — analytics in particular — use the new credentials
				// instead of the pre-login state.
				app.SetClient(client, projectID)
				// Identify the user for analytics.
				common.IdentifyOAuthUser(cmd.Context(), cfg, client, projectID)
				finishLogin(cmd, cfg, prevProjectID, projectID)
				return nil
			} else if creds.publicKey == "" || creds.secretKey == "" {
				creds, err = promptForCredentials(cmd, cfg.ConsoleURL, creds)
				if err != nil {
					return fmt.Errorf("failed to get credentials: %w", err)
				}
				if creds.publicKey == "" || creds.secretKey == "" {
					return fmt.Errorf("both public key and secret key are required")
				}
			}

			apiKey := fmt.Sprintf("%s:%s", creds.publicKey, creds.secretKey)
			client, err := api.NewTigerClient(cfg, apiKey)
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			cmd.PrintErrln("Validating API key...")
			authInfo, err := validateAPIKey(cmd.Context(), cfg, client)
			if err != nil {
				return fmt.Errorf("API key validation failed: %w", err)
			}
			// An API key carries its own project; a mismatched --project-id is
			// an error.
			if projectID != "" && projectID != authInfo.APIKey.Project.ID {
				return common.ExitWithCode(common.ExitInvalidParameters,
					errors.New("API key is scoped to a different project than the one requested with --project-id"))
			}
			if err := cfg.StoreCredentials(apiKey, authInfo.APIKey.Project.ID); err != nil {
				return fmt.Errorf("failed to store credentials: %w", err)
			}
			// See the OAuth branch above: keep the App's client in sync with the
			// credentials we just stored.
			app.SetClient(client, authInfo.APIKey.Project.ID)
			finishLogin(cmd, cfg, prevProjectID, authInfo.APIKey.Project.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&flags.publicKey, "public-key", "", "Public key for authentication")
	cmd.Flags().StringVar(&flags.secretKey, "secret-key", "", "Secret key for authentication")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project ID to log in to (skips interactive project selection)")

	return cmd
}

func finishLogin(cmd *cobra.Command, cfg *config.Config, prevProjectID, projectID string) {
	// An empty prevProjectID (e.g. after a logout) means the default service
	// may belong to any project, so clear it too.
	if prevProjectID != projectID {
		clearStaleDefaultService(cmd, cfg)
	}
	cmd.Printf("Successfully logged in (project: %s)\n", projectID)
	cmd.Print(nextStepsMessage)
}

func flagOrEnvVar(flagVal, envVarName string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(envVarName)
}

func promptForCredentials(cmd *cobra.Command, consoleURL string, creds credentials) (credentials, error) {
	if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
		return credentials{}, fmt.Errorf("TTY not detected - credentials required. Use flags (--public-key, --secret-key) or environment variables (TIGER_PUBLIC_KEY, TIGER_SECRET_KEY)")
	}

	ctx := cmd.Context()
	cmd.PrintErrf("You can find your API credentials at: %s/dashboard/settings\n\n", consoleURL)

	if creds.publicKey == "" {
		cmd.PrintErr("Enter your public key: ")
		publicKey, err := util.ReadLine(ctx, cmd.InOrStdin())
		if err != nil {
			return credentials{}, err
		}
		creds.publicKey = publicKey
	}

	if creds.secretKey == "" {
		cmd.PrintErr("Enter your secret key: ")
		password, err := util.ReadPassword(ctx, cmd.InOrStdin())
		if err != nil {
			return credentials{}, err
		}
		cmd.PrintErrln()
		creds.secretKey = password
	}

	return creds, nil
}

type oauthLogin struct {
	cfg        *config.Config
	authURL    string
	tokenURL   string
	successURL string
	projectID  string // from --project-id; empty means select interactively
	cmd        *cobra.Command
}

func (l *oauthLogin) loginWithOAuth(ctx context.Context) (*oauth2.Token, *api.ClientWithResponses, string, error) {
	token, err := l.getOAuthToken(ctx)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to authenticate via OAuth: %w", err)
	}

	// Build the token-authenticated client once and reuse it for the
	// subsequent authenticated requests.
	client, err := api.NewTigerClientWithToken(l.cfg, token, nil)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to create API client: %w", err)
	}

	projectID, err := l.selectProjectID(ctx, client)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to select project: %w", err)
	}

	return token, client, projectID, nil
}

func (l *oauthLogin) getOAuthToken(ctx context.Context) (*oauth2.Token, error) {
	codeVerifier := oauth2.GenerateVerifier()

	// Random state guards against CSRF on the OAuth callback.
	state, err := l.generateRandomState(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random state: %w", err)
	}

	server, err := l.startOAuthServer(state, codeVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to create local server: %w", err)
	}
	defer func() {
		if err := server.server.Shutdown(ctx); err != nil {
			l.cmd.PrintErrf("Failed to close local server: %s\n", err)
		}
	}()

	authURL := server.oauthCfg.AuthCodeURL(state, oauth2.S256ChallengeOption(codeVerifier))
	l.cmd.PrintErrf("Auth URL is: %s\n", authURL)
	l.cmd.PrintErrln("Opening browser for authentication...")
	if err := openBrowser(authURL); err != nil {
		l.cmd.PrintErrf("Failed to open browser: %s\nPlease manually navigate to the Auth URL.", err)
	}

	select {
	case result := <-server.resultChan:
		return result.token, result.err
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authorization timeout - no callback received within 5 minutes")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *oauthLogin) generateRandomState(length int) (string, error) {
	data := make([]byte, length)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(data)[:length], nil
}

type oauthServer struct {
	server     *http.Server
	oauthCfg   oauth2.Config
	resultChan <-chan oauthResult
}

type oauthResult struct {
	token *oauth2.Token
	err   error
}

func (l *oauthLogin) startOAuthServer(expectedState, codeVerifier string) (*oauthServer, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen on local port: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	oauthCfg := oauth2.Config{
		ClientID: config.TigerCLIClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:   l.authURL,
			TokenURL:  l.tokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: fmt.Sprintf("http://localhost:%d/callback", port),
	}

	// Start local HTTP server for callback
	resultChan := make(chan oauthResult, 1)
	mux := http.NewServeMux()
	mux.Handle("GET /callback", &oauthCallback{
		oauthCfg:      oauthCfg,
		expectedState: expectedState,
		codeVerifier:  codeVerifier,
		successURL:    l.successURL,
		resultChan:    resultChan,
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			resultChan <- oauthResult{
				err: fmt.Errorf("failed to serve requests: %w", err),
			}
		}
	}()

	return &oauthServer{
		server:     server,
		oauthCfg:   oauthCfg,
		resultChan: resultChan,
	}, nil
}

type oauthCallback struct {
	oauthCfg      oauth2.Config
	expectedState string
	codeVerifier  string
	successURL    string
	resultChan    chan<- oauthResult
}

func (c *oauthCallback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Validate state parameter
	state := query.Get("state")
	if state != c.expectedState {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid state parameter")
		c.sendError(fmt.Errorf("invalid state parameter"))
		return
	}

	// Get authorization code
	code := query.Get("code")
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Missing authorization code")
		c.sendError(fmt.Errorf("missing authorization code in callback"))
		return
	}

	// api.HTTPClient already stamps the CLI's User-Agent (recorded server-side
	// as the session's user_agent) and applies our 30s timeout.
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, api.HTTPClient)
	token, err := c.oauthCfg.Exchange(ctx, code, oauth2.VerifierOption(c.codeVerifier))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to exchange authorization code for tokens")
		c.sendError(fmt.Errorf("failed to exchange code for tokens: %w", err))
		return
	}

	// Redirect to success page
	http.Redirect(w, r, c.successURL, http.StatusTemporaryRedirect)

	c.resultChan <- oauthResult{
		token: token,
	}
}

func (c *oauthCallback) sendError(err error) {
	c.resultChan <- oauthResult{err: err}
}

func openBrowserImpl(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Escape '&' so cmd.exe doesn't treat it as a command separator
		cmd = exec.Command("cmd", "/c", "start", strings.ReplaceAll(url, "&", "^&"))
	case "darwin":
		cmd = exec.Command("open", url)
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}

func (l *oauthLogin) selectProjectID(ctx context.Context, client *api.ClientWithResponses) (string, error) {
	resp, err := client.GetProjectsWithResponse(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get user projects: %w", err)
	}
	if resp.JSON200 == nil {
		return "", common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	projects := *resp.JSON200

	if l.projectID != "" {
		if err := requireProjectAccess(l.cmd, projects, l.projectID); err != nil {
			return "", err
		}
		return l.projectID, nil
	}

	switch len(projects) {
	case 0:
		return "", fmt.Errorf("user has no accessible projects")
	case 1:
		return projects[0].ID, nil
	default:
		if !util.IsTerminal(l.cmd.InOrStdin()) || !util.IsTerminal(l.cmd.ErrOrStderr()) {
			return "", fmt.Errorf("TTY not detected - cannot select between %d projects. Log in with API keys instead (--public-key, --secret-key)", len(projects))
		}
		return selectProjectInteractively(l.cmd, projects)
	}
}

// selectProjectInteractivelyImpl is the default implementation for project selection using Bubble Tea
func selectProjectInteractivelyImpl(cmd *cobra.Command, projects []api.Project) (string, error) {
	model := projectSelectModel{
		projects: projects,
		cursor:   0,
	}

	program := tea.NewProgram(model,
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.ErrOrStderr()),
		tea.WithContext(cmd.Context()),
		tea.WithoutSignalHandler())
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

type projectSelectModel struct {
	projects     []api.Project
	cursor       int
	selected     string
	numberBuffer string
}

func (m projectSelectModel) Init() tea.Cmd {
	return nil
}

func (m projectSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			// Clear buffer when using arrows
			m.numberBuffer = ""
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			// Clear buffer when using arrows
			m.numberBuffer = ""
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case "enter", "space":
			m.selected = m.projects[m.cursor].ID
			return m, tea.Quit
		case "backspace":
			// Handle backspace to remove last character from buffer
			if len(m.numberBuffer) > 0 {
				m.updateNumberBuffer(m.numberBuffer[:len(m.numberBuffer)-1])
			}
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Add digit to buffer and update cursor position
			m.updateNumberBuffer(m.numberBuffer + msg.String())
		case "ctrl+w", "esc":
			// Clear buffer on escape
			m.numberBuffer = ""
		}
	}
	return m, nil
}

// updateNumberBuffer moves the cursor to the project matching the number buffer
func (m *projectSelectModel) updateNumberBuffer(newBuffer string) {
	if newBuffer == "" {
		m.numberBuffer = newBuffer
		return
	}

	// Parse the buffer as a number
	num, err := strconv.Atoi(newBuffer)
	if err != nil {
		return
	}

	// Convert from 1-based to 0-based index and validate bounds
	index := num - 1
	if index >= 0 && index < len(m.projects) {
		m.numberBuffer = newBuffer
		m.cursor = index
	}
}

func (m projectSelectModel) View() tea.View {
	var s strings.Builder
	s.WriteString("Select a project:\n\n")

	for i, project := range m.projects {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s.WriteString(fmt.Sprintf("%s %d. %s (%s)\n", cursor, i+1, project.Name, project.ID))
	}

	// Show the current number buffer if user is typing
	if m.numberBuffer != "" {
		s.WriteString(fmt.Sprintf("\nTyping: %s", m.numberBuffer))
	}

	s.WriteString("\nUse ↑/↓ arrows or number keys to navigate, enter to select, q to quit")
	return tea.NewView(s.String())
}
