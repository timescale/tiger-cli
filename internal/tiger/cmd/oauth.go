package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/oauth2"
)

// OAuth parameters
const clientID = "45e1b16d-e435-4049-97b2-8daad150818c"

var (
	// openBrowser can be overridden for testing
	openBrowser = openBrowserImpl

	// selectProjectInteractively can be overridden for testing
	selectProjectInteractively = selectProjectInteractivelyImpl
)

type oauthLogin struct {
	authURL    string
	tokenURL   string
	successURL string
	graphql    *GraphQLClient
	out        io.Writer
}

func (l *oauthLogin) loginWithOAuth(ctx context.Context) (credentials, error) {
	// Get a user access token via OAuth
	accessToken, err := l.getAccessToken(ctx)
	if err != nil {
		return credentials{}, fmt.Errorf("failed to authenticate via OAuth: %w", err)
	}

	// Get the user's project ID (with interactive selection if needed)
	projectID, err := l.selectProjectID(ctx, accessToken)
	if err != nil {
		return credentials{}, fmt.Errorf("failed to select project: %w", err)
	}

	// Create API key for the selected project
	creds, err := l.createCredentials(ctx, accessToken, projectID)
	if err != nil {
		return credentials{}, fmt.Errorf("failed to create credentials: %w", err)
	}

	return creds, nil
}

func (l *oauthLogin) getAccessToken(ctx context.Context) (string, error) {
	// Generate PKCE parameters
	codeVerifier := oauth2.GenerateVerifier()

	// Generate random state string (to guard against CRSF attacks)
	state, err := l.generateRandomState(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate random state: %w", err)
	}

	// Start local HTTP server for handling the OAuth callback
	server, err := l.startOAuthServer(state, codeVerifier)
	if err != nil {
		return "", fmt.Errorf("failed to create local server: %w", err)
	}
	defer func() {
		if err := server.Close(); err != nil {
			fmt.Fprintf(l.out, "Failed to close local server: %s\n", err)
		}
	}()

	// Open browser
	authURL := server.oauthCfg.AuthCodeURL(state, oauth2.S256ChallengeOption(codeVerifier))
	fmt.Fprintf(l.out, "Auth URL is: %s\n", authURL)
	fmt.Fprintln(l.out, "Opening browser for authentication...")
	if err := openBrowser(authURL); err != nil {
		fmt.Fprintf(l.out, "Failed to open browser: %s\nPlease manually navigate to the Auth URL.", err)
	}

	// Wait for callback with timeout
	select {
	case result := <-server.resultChan:
		return result.accessToken, result.err
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("authorization timeout - no callback received within 5 minutes")
	case <-ctx.Done():
		return "", ctx.Err()
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
	listener   net.Listener
	server     *http.Server
	oauthCfg   oauth2.Config
	resultChan <-chan oauthResult
}

type oauthResult struct {
	accessToken string
	err         error
}

func (l *oauthLogin) startOAuthServer(expectedState, codeVerifier string) (*oauthServer, error) {
	// Start listening on an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen on local port: %w", err)
	}

	// Build OAuth config with localhost redirect URI
	port := listener.Addr().(*net.TCPAddr).Port
	oauthCfg := oauth2.Config{
		ClientID: clientID,
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
		listener:   listener,
		oauthCfg:   oauthCfg,
		resultChan: resultChan,
	}, nil
}

func (s *oauthServer) Close() error {
	cls := func(closer io.Closer) error {
		if err := closer.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
		return nil
	}

	return errors.Join(
		cls(s.server),
		cls(s.listener),
	)
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

	// Exchange authorization code for tokens
	token, err := c.oauthCfg.Exchange(r.Context(), code, oauth2.VerifierOption(c.codeVerifier))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to exchange authorization code for tokens")
		c.sendError(fmt.Errorf("failed to exchange code for tokens: %w", err))
		return
	}

	// Redirect to success page
	http.Redirect(w, r, c.successURL, http.StatusTemporaryRedirect)

	c.resultChan <- oauthResult{
		accessToken: token.AccessToken,
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

// selectProjectID prompts the user to select a project if multiple are available
func (l *oauthLogin) selectProjectID(ctx context.Context, accessToken string) (string, error) {
	// First, get the list of projects the user has access to
	projects, err := l.graphql.getUserProjects(ctx, accessToken)
	if err != nil {
		return "", fmt.Errorf("failed to get user projects: %w", err)
	}

	switch len(projects) {
	case 0:
		return "", fmt.Errorf("user has no accessible projects")
	case 1:
		return projects[0].ID, nil
	default:
		return selectProjectInteractively(projects, l.out)
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
	projects     []Project
	cursor       int
	selected     string
	numberBuffer string
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
		case "enter", " ":
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

func (m projectSelectModel) View() string {
	s := "Select a project:\n\n"

	for i, project := range m.projects {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %d. %s (%s)\n", cursor, i+1, project.Name, project.ID)
	}

	// Show the current number buffer if user is typing
	if m.numberBuffer != "" {
		s += fmt.Sprintf("\nTyping: %s", m.numberBuffer)
	}

	s += "\nUse ↑/↓ arrows or number keys to navigate, enter to select, q to quit"
	return s
}

// createCredentials creates client credentials (i.e. a PAT record) for the
// selected project
func (l *oauthLogin) createCredentials(ctx context.Context, accessToken, projectID string) (credentials, error) {
	// Get user information for PAT name
	user, err := l.graphql.getUser(ctx, accessToken)
	if err != nil {
		return credentials{}, fmt.Errorf("failed to get user info: %w", err)
	}

	// Create a PAT record for this project
	patRecord, err := l.graphql.createPATRecord(ctx, accessToken, projectID, l.buildPATName(user))
	if err != nil {
		// Check if error is about reaching maximum token limit
		if strings.Contains(err.Error(), "reached maximum token limit for project") {
			return credentials{}, fmt.Errorf("failed to create API key: %w\n\nYou can delete existing API keys at: https://console.cloud.timescale.com/dashboard/settings", err)
		}
		return credentials{}, fmt.Errorf("failed to create PAT record: %w", err)
	}

	return credentials{
		publicKey: patRecord.ClientCredentials.AccessKey,
		secretKey: patRecord.ClientCredentials.SecretKey,
		projectID: projectID,
	}, nil
}

// Build the PAT/Client Credentials name. This is displayed under "Project settings"
// in the console and helps identify what the credentials are for
func (l *oauthLogin) buildPATName(user *User) string {
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
