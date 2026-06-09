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
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

var (
	// openBrowser can be overridden for testing
	openBrowser = openBrowserImpl

	// selectProjectInteractively can be overridden for testing
	selectProjectInteractively = selectProjectInteractivelyImpl
)

type oauthLogin struct {
	cfg        *config.Config
	authURL    string
	tokenURL   string
	successURL string
	out        io.Writer
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
			fmt.Fprintf(l.out, "Failed to close local server: %s\n", err)
		}
	}()

	authURL := server.oauthCfg.AuthCodeURL(state, oauth2.S256ChallengeOption(codeVerifier))
	fmt.Fprintf(l.out, "Auth URL is: %s\n", authURL)
	fmt.Fprintln(l.out, "Opening browser for authentication...")
	if err := openBrowser(authURL); err != nil {
		fmt.Fprintf(l.out, "Failed to open browser: %s\nPlease manually navigate to the Auth URL.", err)
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

	switch len(projects) {
	case 0:
		return "", fmt.Errorf("user has no accessible projects")
	case 1:
		return projects[0].Id, nil
	default:
		return selectProjectInteractively(projects, l.out)
	}
}

// selectProjectInteractivelyImpl is the default implementation for project selection using Bubble Tea
func selectProjectInteractivelyImpl(projects []api.Project, out io.Writer) (string, error) {
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
			m.selected = m.projects[m.cursor].Id
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
		s += fmt.Sprintf("%s %d. %s (%s)\n", cursor, i+1, project.Name, project.Id)
	}

	// Show the current number buffer if user is typing
	if m.numberBuffer != "" {
		s += fmt.Sprintf("\nTyping: %s", m.numberBuffer)
	}

	s += "\nUse ↑/↓ arrows or number keys to navigate, enter to select, q to quit"
	return s
}
