package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

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

// openBrowser can be overridden for testing
var openBrowser = openBrowserImpl

type credentials struct {
	publicKey string
	secretKey string
}

func buildLoginCmd(app *common.App) *cobra.Command {
	var flags credentials
	var projectIDFlag string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Tiger Cloud API",
		Long: `Authenticate with Tiger Cloud API using predefined keys or an interactive OAuth flow

By default, the command will launch an interactive OAuth flow in your browser to sign in.
The OAuth flow will:
- Open your browser for authentication
- Let you select a project (if you have multiple)
- Store an OAuth session for the selected project

Use --project-id (or the TIGER_PROJECT_ID environment variable) to pick the project up front
and skip the interactive selection, which is what you want when no terminal is available. After
logging in, switch between projects with 'tiger project'.

The credentials and project ID will be stored securely in the system keyring, or in a fallback file with
restricted permissions. Logging in to a different project than the previous login clears the default
service (config key service_id), since a default service belongs to the project it was set in.

You may also provide API keys via flags or environment variables, in which case they will be used
directly. The CLI will prompt for any missing information. An API key is scoped to a single project,
so --project-id only confirms which project that is.

You can find your API credentials at: https://console.cloud.tigerdata.com/dashboard/settings

Examples:
  # Interactive login with OAuth (opens browser)
  tiger auth login

  # OAuth login without the interactive project picker
  tiger auth login --project-id rp1pz7uyae

  # Login with keys (project ID will be auto-detected)
  tiger auth login --public-key your-public-key --secret-key your-secret-key

  # Login using environment variables
  export TIGER_PUBLIC_KEY="your-public-key"
  export TIGER_SECRET_KEY="your-secret-key"
  tiger auth login`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg := app.GetConfig()

			var err error
			creds := credentials{
				publicKey: flagOrEnvVar(flags.publicKey, "TIGER_PUBLIC_KEY"),
				secretKey: flagOrEnvVar(flags.secretKey, "TIGER_SECRET_KEY"),
			}
			requestedProjectID := flagOrEnvVar(projectIDFlag, "TIGER_PROJECT_ID")

			// Captured before the login replaces it: landing on a different
			// project clears the default service, like `tiger project` does.
			prevProjectID := ""
			if stored, err := cfg.GetStoredCredentials(); err == nil {
				prevProjectID = stored.ProjectID
			}

			if creds.publicKey == "" && creds.secretKey == "" {
				l := &oauthLogin{
					cfg:               cfg,
					authURL:           cfg.ConsoleURL + "/oauth/authorize",
					tokenURL:          cfg.GatewayURL + "/idp/external/cli/token",
					successURL:        cfg.ConsoleURL + "/oauth/code/success",
					projectID:         requestedProjectID,
					projectIDFromFlag: projectIDFlag != "",
					cmd:               cmd,
				}

				token, client, projectID, err := l.loginWithOAuth(cmd.Context())
				if err != nil {
					return err
				}
				if err := cfg.StoreOAuthCredentials(token, projectID); err != nil {
					return fmt.Errorf("failed to store credentials: %w", err)
				}
				// Identify the user for analytics.
				common.IdentifyOAuthUser(cmd.Context(), cfg, client, projectID)
				completeLogin(cmd, app, cfg, client, prevProjectID, projectID)
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
			// An API key carries its own project: a mismatched --project-id is
			// an error, while the ambient TIGER_PROJECT_ID only warns. An empty
			// flag counts as unset (projectIDFlag, not cobra's Changed), like
			// flagOrEnvVar above.
			if requestedProjectID != "" && requestedProjectID != authInfo.APIKey.Project.ID {
				if projectIDFlag != "" {
					// No project IDs in the message — analytics records error
					// text verbatim; `tiger auth status` shows the key's project.
					return common.ExitWithCode(common.ExitInvalidParameters,
						errors.New("API key is scoped to a different project than the requested one"))
				}
				cmd.PrintErrf("Warning: ignoring TIGER_PROJECT_ID (%s) - this API key is scoped to project %s\n", requestedProjectID, authInfo.APIKey.Project.ID)
			}
			if err := cfg.StoreCredentials(apiKey, authInfo.APIKey.Project.ID); err != nil {
				return fmt.Errorf("failed to store credentials: %w", err)
			}
			completeLogin(cmd, app, cfg, client, prevProjectID, authInfo.APIKey.Project.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&flags.publicKey, "public-key", "", "Public key for authentication")
	cmd.Flags().StringVar(&flags.secretKey, "secret-key", "", "Secret key for authentication")
	cmd.Flags().StringVar(&projectIDFlag, "project-id", "", "Project ID to log in to (skips interactive project selection)")

	// Suppress cobra's default file completion; filenames are never project IDs.
	_ = cmd.RegisterFlagCompletionFunc("project-id", cobra.NoFileCompletions)

	return cmd
}

// completeLogin runs the post-store steps shared by both login flavors: update
// the App's client so later readers (analytics) use the new credentials, clear
// a default service left from a different project, and print the success message.
func completeLogin(cmd *cobra.Command, app *common.App, cfg *config.Config, client *api.ClientWithResponses, prevProjectID, projectID string) {
	app.SetClient(client, projectID)
	clearStaleDefaultService(cmd, cfg, prevProjectID, projectID)
	finishLogin(cmd, projectID)
}

func finishLogin(cmd *cobra.Command, projectID string) {
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
	// projectID comes from --project-id or TIGER_PROJECT_ID; empty means
	// select one after authenticating. projectIDFromFlag records which: the
	// flag is authoritative, the ambient env var only warns.
	projectID         string
	projectIDFromFlag bool
	cmd               *cobra.Command
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

	projects, err := fetchProjects(ctx, client)
	if err != nil {
		return nil, nil, "", err
	}
	project, err := resolveProjectID(l.cmd, projects, l.projectID,
		"pass --project-id or set TIGER_PROJECT_ID")
	if err != nil && l.projectID != "" && !l.projectIDFromFlag {
		// TIGER_PROJECT_ID is ambient and may belong to a different login, so
		// an inaccessible project it names only warns, like the API-key branch.
		l.cmd.PrintErrf("Warning: ignoring TIGER_PROJECT_ID (%s) - project not found or not accessible\n", l.projectID)
		project, err = resolveProjectID(l.cmd, projects, "", "pass --project-id")
	}
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to select project: %w", err)
	}

	return token, client, project.ID, nil
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

// userAgentTransport sets the CLI User-Agent on outgoing requests.
type userAgentTransport struct{}

func (userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", config.UserAgent())
	return http.DefaultTransport.RoundTrip(req)
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

	// The exchange's User-Agent is recorded as the CLI session's user_agent.
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient,
		&http.Client{Transport: userAgentTransport{}})
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
