package common

import (
	"context"
	"fmt"
	"sync"

	"github.com/spf13/pflag"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
)

// App holds shared application state: the config and the API client built from
// it. For CLI commands it is populated once at the start of the wrapped RunE
// (see wrapCommands in internal/cmd) and shared by every command handler. For
// MCP requests, Load is called once per request by the analytics middleware, so
// config changes and logins/logouts made while the session is open take effect
// on the next request; handlers then read the loaded state via GetAll and
// friends.
//
// All state is unexported. Use Load or SetClient to populate it, and
// GetAll/TryGetAll/GetConfig/GetClient to read it. Concurrency is handled
// internally via a sync.RWMutex.
type App struct {
	// Experimental gates preview-stage commands and MCP tools. Read once from
	// TIGER_EXPERIMENTAL at startup; see CLAUDE.md's "Experimental Feature
	// Gating".
	Experimental bool

	flags         *pflag.FlagSet
	config        *config.Config
	client        api.ClientWithResponsesInterface // nil if credentials are unavailable
	projectID     string
	clientErr     error         // returned by GetClient/GetAll when client is nil
	clientFactory ClientFactory // nil in production; set in tests
	lock          sync.RWMutex  // protects config, client, projectID, clientErr
}

// ClientFactory creates an API client from the loaded config. Tests use it to
// inject a client while letting Load run normally, so config resolution and flag
// precedence still go through the real code path.
type ClientFactory func(ctx context.Context, cfg *config.Config) (api.ClientWithResponsesInterface, string, error)

// SetClientFactory sets a custom factory for API client creation.
// When set, Load calls this instead of [NewAPIClient].
func (a *App) SetClientFactory(f ClientFactory) {
	a.clientFactory = f
}

// SetFlags stores the command's flag set for use by [config.Load]. Must be
// called before Load.
func (a *App) SetFlags(flags *pflag.FlagSet) {
	a.flags = flags
}

// Load loads (or reloads) the config and attempts to create the API client.
// Returns the config, API client, and project ID. Config errors are returned;
// API client errors are stored and surfaced by GetClient/GetAll instead (the
// returned client is simply nil), so commands that don't need the client still
// run when the user isn't logged in.
func (a *App) Load(ctx context.Context) (*config.Config, api.ClientWithResponsesInterface, string, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	cfg, err := config.Load(a.flags)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to load config: %w", err)
	}
	a.config = cfg

	a.client, a.projectID, a.clientErr = a.newAPIClient(ctx, a.config)

	return a.config, a.client, a.projectID, nil
}

func (a *App) newAPIClient(ctx context.Context, cfg *config.Config) (api.ClientWithResponsesInterface, string, error) {
	if a.clientFactory != nil {
		return a.clientFactory(ctx, cfg)
	}
	client, projectID, err := NewAPIClient(ctx, cfg)
	if err != nil {
		// Return an untyped nil: wrapping the nil *ClientWithResponses in the
		// interface would make a "client == nil" check false for callers.
		return nil, "", err
	}
	return client, projectID, nil
}

// SetClient stores an existing API client and project ID. Use it when a valid
// client already exists (e.g. after `tiger auth login` builds one to validate
// credentials) so later readers — analytics in particular — see the new
// credentials without re-reading them from storage.
func (a *App) SetClient(client api.ClientWithResponsesInterface, projectID string) {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.client = client
	a.projectID = projectID
	a.clientErr = nil
}

// getAll is the locked primitive behind the exported accessors. It returns a
// snapshot of the App state: the returned values stay valid even if Load is
// called concurrently, because Load replaces pointers rather than mutating the
// objects they point to. The returned error is the stored client-creation
// error, nil when the client is available.
//
// Panics if the App has never been loaded. That's a programmer error: every
// code path that reads the App (commands, MCP request handlers, completion
// functions) must arrange for Load to run first.
func (a *App) getAll() (*config.Config, api.ClientWithResponsesInterface, string, error) {
	a.lock.RLock()
	defer a.lock.RUnlock()

	if a.config == nil {
		panic("App.Load must be called before accessing the config or API client")
	}
	return a.config, a.client, a.projectID, a.clientErr
}

// GetAll returns a snapshot of the config, API client, and project ID. Returns
// an error (and zero values) if the client is unavailable, e.g. because the
// user isn't logged in. Panics if the App has never been loaded (see getAll).
//
// Callers that only read the config but do reach the API later still use GetAll
// (discarding the client) so that a missing credential fails fast, before any
// prompting or other work.
func (a *App) GetAll() (*config.Config, api.ClientWithResponsesInterface, string, error) {
	cfg, client, projectID, err := a.getAll()
	if err != nil {
		return nil, nil, "", err
	}
	return cfg, client, projectID, nil
}

// TryGetAll returns a snapshot of the config, API client, and project ID like
// GetAll, but tolerates an unavailable client: the returned client is simply
// nil. Use it for best-effort work where the API call is optional (e.g.
// analytics). Panics if the App has never been loaded (see getAll).
func (a *App) TryGetAll() (*config.Config, api.ClientWithResponsesInterface, string) {
	cfg, client, projectID, _ := a.getAll() // error dropped: best-effort access
	return cfg, client, projectID
}

// GetConfig returns a snapshot of the config. Panics if the App has never been
// loaded (see getAll).
func (a *App) GetConfig() *config.Config {
	cfg, _, _, _ := a.getAll()
	return cfg
}

// GetClient returns a snapshot of the API client and project ID. Returns an
// error if the client is unavailable, e.g. because the user isn't logged in.
// Panics if the App has never been loaded (see getAll).
func (a *App) GetClient() (api.ClientWithResponsesInterface, string, error) {
	_, client, projectID, err := a.getAll()
	return client, projectID, err
}
