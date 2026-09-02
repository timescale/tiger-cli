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

// nextStepsMessage is the message shown after successful login
const nextStepsMessage = `
🎉 Next steps:
• Install MCP server for your favorite AI coding tool: tiger mcp install
• List existing services: tiger service list
• Create a new service: tiger service create
`

// readOnlyNextStep tells the user how to set read_only by hand. Printing it to
// someone who just picked a mode from the menu would read as if their answer
// didn't take, so it's only for logins that left the choice unmade.
const readOnlyNextStep = "• Protect services from writes: tiger config set read_only prod (or all, off)\n"

// nextSteps is the post-login message, picking up readOnlyNextStep when the
// config file holds no read_only value.
func nextSteps(readOnlySet bool) string {
	if readOnlySet {
		return nextStepsMessage
	}
	return nextStepsMessage + readOnlyNextStep
}

var (
	// deviceFallbackGrace is how long an opened browser gets to finish the
	// redirect before the device code is printed too. Overridden in tests.
	deviceFallbackGrace = 10 * time.Second

	// deviceRetryDelay spaces pollDeviceToken's retries. Overridden in tests.
	deviceRetryDelay = 2 * time.Second

	// defaultDeviceCodeTTL bounds polling when the gateway omits expires_in.
	// Overridden in tests.
	defaultDeviceCodeTTL = 15 * time.Minute
)

// errServiceUnavailable is a failure to answer rather than a verdict. Both
// halves of the login redeem their code at the same endpoint, so it sinks the
// redirect as surely as the device code.
var errServiceUnavailable = errors.New("the authorization service is unavailable - try again in a moment")

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
	var headless bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Tiger Cloud API",
		Long: `Authenticate with Tiger Cloud API using predefined keys or an interactive OAuth flow

By default, the command will launch an interactive OAuth flow in your browser to sign in.
The OAuth flow will:
- Open your browser for authentication
- Let you select a project (if you have multiple)
- Store an OAuth session for the selected project

If the browser cannot be opened, or its redirect back to this machine never arrives, the
command prints a short code to enter in a browser on any other machine and carries on from
there. Use --headless to go straight to that flow.

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

  # Login from a machine the browser redirect cannot reach (SSH session, container)
  tiger auth login --headless

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
					cfg:           cfg,
					authURL:       cfg.ConsoleURL + "/oauth/authorize",
					tokenURL:      cfg.GatewayURL + "/idp/external/cli/token",
					deviceCodeURL: cfg.GatewayURL + "/idp/external/cli/device/code",
					successURL:    cfg.ConsoleURL + "/oauth/code/success",
					headless:      headless,
					projectID:     projectID,
					cmd:           cmd,
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
			authInfo, err := common.ValidateAPIKey(cmd.Context(), cfg, client)
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
	cmd.Flags().BoolVar(&headless, "headless", false, "Authorize by entering a code in a browser on any machine, instead of waiting for a redirect back to this one")

	return cmd
}

func finishLogin(cmd *cobra.Command, cfg *config.Config, prevProjectID, projectID string) {
	// An empty prevProjectID (e.g. after a logout) means the default service
	// may belong to any project, so clear it too.
	if prevProjectID != projectID {
		clearStaleDefaultService(cmd, cfg)
	}
	cmd.Printf("Successfully logged in (project: %s)\n", projectID)

	readOnlySet := offerProdProtection(cmd, cfg)
	cmd.Print(nextSteps(readOnlySet))
}

// offerProdProtection asks which services to protect from writes. Every choice
// is recorded, so it asks exactly once — including of users who predate the
// option. Choosing "off" stores read_only=off rather than leaving the key
// absent, which a later login would read as never having been asked.
//
// It reports whether the config file now holds a read_only value, written just
// now or by an earlier login. False means nobody has set one, so nextSteps says
// how to do it by hand.
func offerProdProtection(cmd *cobra.Command, cfg *config.Config) bool {
	// TIGER_READ_ONLY outranks the config file, so a mode chosen here wouldn't
	// take effect.
	if os.Getenv("TIGER_READ_ONLY") != "" {
		return true
	}

	stored, err := config.LoadForOutput(cfg.ConfigDir, false, true)
	if err != nil {
		// Can't tell whether it was ever answered, so don't ask — but do print the
		// bullet, which is the harmless half of the two.
		return false
	}
	if stored.ReadOnly != nil {
		return true
	}

	// Both streams, per the convention: stdin so the answer can be read, stderr so
	// the question can be seen. An unseen prompt reads as a hang.
	if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
		return false
	}

	mode, chose := selectReadOnlyMode(cmd)
	if !chose {
		// Dismissed rather than answered, so record nothing and ask again.
		return false
	}

	if _, err := cfg.Set("read_only", string(mode)); err != nil {
		cmd.PrintErrf("⚠️  Warning: could not set read_only: %v\n", err)
		return false
	}
	if msg := readOnlyConfirmation(mode); msg != "" {
		cmd.PrintErrln(msg)
	}
	return true
}

// readOnlyConfirmation is what to report after a mode is stored.
func readOnlyConfirmation(mode config.ReadOnlyMode) string {
	switch mode {
	case config.ReadOnlyProd:
		return "Services tagged PROD are now protected from writes."
	case config.ReadOnlyAll:
		return "All services are now protected from writes."
	default:
		return ""
	}
}

// readOnlyChoice is one option in the post-login menu.
type readOnlyChoice struct {
	mode  config.ReadOnlyMode
	label string
}

// readOnlyChoices lists every mode, so the menu doubles as the one place a new
// user learns the option exists. Recommended mode first, which is also where
// the cursor starts.
var readOnlyChoices = []readOnlyChoice{
	{config.ReadOnlyProd, "Services tagged PROD only (recommended)"},
	{config.ReadOnlyAll, "Every service"},
	{config.ReadOnlyOff, "Nothing - allow writes everywhere"},
}

// selectReadOnlyMode shows the menu and reports the chosen mode. The bool is
// false when the user dismissed it without choosing, which is not an answer.
var selectReadOnlyMode = selectReadOnlyModeImpl

func selectReadOnlyModeImpl(cmd *cobra.Command) (config.ReadOnlyMode, bool) {
	program := tea.NewProgram(readOnlyModel{},
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.ErrOrStderr()),
		tea.WithContext(cmd.Context()),
		tea.WithoutSignalHandler())

	finalModel, err := program.Run()
	if err != nil {
		// A menu we couldn't show is not a decline: leave the key absent so the
		// next login asks again.
		return "", false
	}

	chosen := finalModel.(readOnlyModel).chosen
	return chosen, chosen != ""
}

// readOnlyModel is the menu. A zero chosen means dismissed, so quitting can't be
// mistaken for picking the mode the cursor happened to rest on.
type readOnlyModel struct {
	cursor int
	chosen config.ReadOnlyMode
}

func (m readOnlyModel) Init() tea.Cmd {
	return nil
}

func (m readOnlyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch key := msg.String(); key {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(readOnlyChoices)-1 {
				m.cursor++
			}
		case "enter", "space":
			m.chosen = readOnlyChoices[m.cursor].mode
			return m, tea.Quit
		default:
			// Number keys jump straight to that option ('1' -> first, etc.).
			if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
				if idx := int(key[0] - '1'); idx < len(readOnlyChoices) {
					m.cursor = idx
					m.chosen = readOnlyChoices[idx].mode
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m readOnlyModel) View() tea.View {
	var s strings.Builder
	s.WriteString("Which services should be protected from writes?\n\n")

	for i, choice := range readOnlyChoices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s.WriteString(fmt.Sprintf("%s %d. %s\n", cursor, i+1, choice.label))
	}

	// No "change it later" hint: dismissing prints the readOnlyNextStep bullet a
	// few lines below, and "q to decide later" already says so anyway.
	s.WriteString("\nUse ↑/↓ arrows or number keys to select, enter to confirm, q to decide later")
	return tea.NewView(s.String())
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
	cfg           *config.Config
	authURL       string
	tokenURL      string
	deviceCodeURL string
	successURL    string
	headless      bool   // go straight to the device flow
	projectID     string // from --project-id; empty means select interactively
	cmd           *cobra.Command
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
	if l.headless {
		oauthCfg, da, err := l.startDeviceAuth(ctx)
		if err != nil {
			return nil, common.ExitWithCode(common.ExitAuthenticationError, err)
		}
		return l.pollDeviceToken(ctx, oauthCfg, da)
	}
	return l.getTokenViaBrowser(ctx)
}

// getTokenViaBrowser runs the redirect flow, falling back to the device flow
// when the browser can't finish it. The listener stays up either way, so
// whichever completes first wins.
func (l *oauthLogin) getTokenViaBrowser(ctx context.Context) (*oauth2.Token, error) {
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
		// Shutdown gives up on a canceled context while a connection is still
		// active -- the callback that just won the race -- so it gets a live one.
		if err := server.server.Shutdown(context.WithoutCancel(ctx)); err != nil {
			l.cmd.PrintErrf("Failed to close local server: %s\n", err)
		}
	}()

	authURL := server.oauthCfg.AuthCodeURL(state, oauth2.S256ChallengeOption(codeVerifier))
	l.cmd.PrintErrf("Auth URL is: %s\n", authURL)
	l.cmd.PrintErrln("Opening browser for authentication...")

	// A browser that never opened can't complete the redirect, and navigating
	// there by hand won't either: it targets a port on this machine.
	browserOpened := true
	grace := deviceFallbackGrace
	if err := openBrowser(authURL); err != nil {
		l.cmd.PrintErrf("Failed to open browser: %s\n", err)
		browserOpened = false
		grace = 0
	}

	select {
	case result := <-server.resultChan:
		return result.token, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(grace):
	}

	oauthCfg, da, err := l.startDeviceAuth(ctx)
	if err != nil {
		// The redirect may still land, so keep waiting on it.
		l.cmd.PrintErrf("Could not fall back to a device code: %s\n", err)
		return l.waitForBrowser(ctx, server.resultChan)
	}

	// The browser is open, just on a page whose redirect can't reach us.
	if browserOpened {
		if err := openBrowser(da.VerificationURI); err != nil {
			l.cmd.PrintErrf("Could not open %s automatically: %s\n", da.VerificationURI, err)
		}
	}

	// The redirect may still complete while the user types the code.
	raceCtx, cancel := context.WithCancel(ctx)

	device := make(chan oauthResult, 1)
	go func() {
		// Closing keeps the drain below from blocking once the race has taken
		// the result.
		defer close(device)
		token, err := l.pollDeviceToken(raceCtx, oauthCfg, da)
		device <- oauthResult{token: token, err: err}
	}()

	// The poll prints to the same stderr as this goroutine, so stop it and let
	// it finish before anything else is written.
	defer func() {
		cancel()
		<-device
	}()

	select {
	case result := <-server.resultChan:
		return result.token, result.err
	case result := <-device:
		// Only a device success ends the race: the redirect may still land --
		// unless nothing is answering, which its code exchange would hit too.
		if result.err == nil {
			return result.token, nil
		}
		if errors.Is(result.err, errServiceUnavailable) {
			return nil, result.err
		}
		l.cmd.PrintErrf("Device authorization failed: %s\n", result.err)
		return l.waitForBrowser(ctx, server.resultChan)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// waitForBrowser waits on the redirect alone, with no device code to race it.
func (l *oauthLogin) waitForBrowser(ctx context.Context, results <-chan oauthResult) (*oauth2.Token, error) {
	select {
	case result := <-results:
		return result.token, result.err
	case <-time.After(5 * time.Minute):
		return nil, errors.New("authorization timeout - no callback received within 5 minutes")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// deviceFlowContext installs api.HTTPClient for the device flow's oauth2
// requests: it stamps the CLI User-Agent and applies our 30s timeout.
func deviceFlowContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, api.HTTPClient)
}

// startDeviceAuth asks the gateway for a code pair and tells the user where
// to enter the short one. The redeemable device_code stays in this process.
func (l *oauthLogin) startDeviceAuth(ctx context.Context) (oauth2.Config, *oauth2.DeviceAuthResponse, error) {
	oauthCfg := oauth2.Config{
		ClientID: config.TigerCLIClientID,
		Endpoint: oauth2.Endpoint{
			DeviceAuthURL: l.deviceCodeURL,
			// Same endpoint as the redirect flow, different grant type.
			TokenURL:  l.tokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	da, err := oauthCfg.DeviceAuth(deviceFlowContext(ctx))
	if err != nil {
		return oauth2.Config{}, nil, fmt.Errorf("failed to start device authorization: %w", err)
	}

	// The gateway floors interval but not expires_in, and x/oauth2 bounds
	// polling only when it sent one -- so supply the bound ourselves.
	if da.Expiry.IsZero() {
		da.Expiry = time.Now().Add(defaultDeviceCodeTTL)
	}

	l.cmd.PrintErrf("\nTo authenticate, visit: %s\nand enter code: %s\n\n", da.VerificationURI, da.UserCode)
	l.cmd.PrintErrln("Waiting for authorization (this can take a few seconds after you enter the code)...")

	return oauthCfg, da, nil
}

// pollDeviceToken polls until the authorization is decided: an OAuth error is
// that verdict, since x/oauth2 polls through authorization_pending and
// slow_down, and a 5xx ends it as a failure to answer. Anything else is
// retried until the codes expire, a deadline DeviceAccessToken takes from
// da.Expiry.
func (l *oauthLogin) pollDeviceToken(ctx context.Context, oauthCfg oauth2.Config, da *oauth2.DeviceAuthResponse) (*oauth2.Token, error) {
	ctx = deviceFlowContext(ctx)

	for {
		token, err := oauthCfg.DeviceAccessToken(ctx, da)
		if err == nil {
			return token, nil
		}

		// A non-2XX with an unparseable body is a RetrieveError too, so the
		// error code, not the type, marks a verdict.
		if retrieveErr, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
			if isServerFailure(retrieveErr) {
				return nil, common.ExitWithCode(common.ExitAuthenticationError, errServiceUnavailable)
			}
			if retrieveErr.ErrorCode != "" {
				return nil, deviceAuthFailure(retrieveErr.ErrorCode, retrieveErr.ErrorDescription)
			}
		}
		// The codes ran out. Asking the clock, rather than reading it off the
		// error: a request that times out on its own reports the same thing,
		// and startDeviceAuth guarantees da.Expiry is set.
		if !time.Now().Before(da.Expiry) {
			return nil, deviceAuthFailure("expired_token", "")
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		l.cmd.PrintErrf("Authorization check failed, retrying: %s\n", err)
		select {
		case <-time.After(deviceRetryDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// isServerFailure reports whether a reply came back 5xx: a failure to answer
// rather than a verdict, whether or not an OAuth error code came with it.
func isServerFailure(err *oauth2.RetrieveError) bool {
	return err.Response != nil && err.Response.StatusCode >= http.StatusInternalServerError
}

// deviceAuthFailure turns the server's OAuth error into what the user sees.
// All of these are terminal, and a fresh code is the fix for every one.
func deviceAuthFailure(code, description string) error {
	var msg string
	switch code {
	case "expired_token":
		msg = "the code expired before it was authorized"
	case "access_denied":
		msg = "the authorization request was denied"
	case "invalid_request", "invalid_grant":
		// FusionAuth's answer for a code already redeemed, or never issued.
		msg = "the code is no longer valid"
	default:
		msg = "authorization failed: " + code
		if description != "" {
			msg = "authorization failed: " + description
		}
	}
	return common.ExitWithCode(common.ExitAuthenticationError,
		errors.New(msg+" - run 'tiger auth login' again for a new code"))
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
