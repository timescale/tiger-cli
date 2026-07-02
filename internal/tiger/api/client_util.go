package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// Shared HTTP client with resource limits to prevent resource exhaustion under load
var (
	httpClientOnce   sync.Once
	sharedHTTPClient *http.Client
)

// getHTTPClient returns a singleton HTTP client with essential resource limits
// Focuses on preventing resource leaks while using reasonable Go defaults elsewhere
func getHTTPClient() *http.Client {
	httpClientOnce.Do(func() {
		// Clone default transport to inherit sensible defaults, then customize key settings
		transport := http.DefaultTransport.(*http.Transport).Clone()

		// Essential resource limits to prevent exhaustion
		transport.MaxIdleConns = 100                 // Limit total idle connections
		transport.MaxIdleConnsPerHost = 10           // Limit per-host idle connections
		transport.IdleConnTimeout = 90 * time.Second // Clean up idle connections

		sharedHTTPClient = &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second, // Overall request timeout
		}
	})
	return sharedHTTPClient
}

// apiKey must be in "publicKey:secretKey" format.
func NewTigerClient(cfg *config.Config, apiKey string) (*ClientWithResponses, error) {
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(apiKey))
	client, err := NewClientWithResponses(cfg.APIURL, WithHTTPClient(getHTTPClient()), WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", authHeader)
		req.Header.Set("User-Agent", config.UserAgent())
		return nil
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}
	return client, nil
}

// NewTigerClientWithToken builds a Bearer-authenticated client that
// auto-refreshes via the gateway's /idp/external/cli/token endpoint. Rotated
// tokens are handed to persist (typically a keyring write); pass nil for
// short-lived callers that don't need to update storage (e.g. logout).
func NewTigerClientWithToken(cfg *config.Config, token *oauth2.Token, persist func(*oauth2.Token) error) (*ClientWithResponses, error) {
	if token == nil || token.AccessToken == "" {
		return nil, fmt.Errorf("oauth token is empty")
	}

	oauthCfg := &oauth2.Config{
		ClientID: config.TigerCLIClientID,
		Endpoint: oauth2.Endpoint{
			TokenURL:  cfg.GatewayURL + "/idp/external/cli/token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	// Stash our pooled client in the context.
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, getHTTPClient())

	var src oauth2.TokenSource = oauthCfg.TokenSource(ctx, token)
	if persist != nil {
		src = &persistingTokenSource{base: src, persist: persist, last: token.AccessToken}
	}

	httpClient := oauth2.NewClient(ctx, src)
	httpClient.Timeout = 30 * time.Second

	client, err := NewClientWithResponses(cfg.APIURL, WithHTTPClient(httpClient), WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Set("User-Agent", config.UserAgent())
		return nil
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}
	return client, nil
}

// NewTigerClientForCredentials dispatches on credential shape. For OAuth,
// rotated tokens are persisted back to storage automatically.
func NewTigerClientForCredentials(cfg *config.Config, creds *config.Credentials) (*ClientWithResponses, error) {
	if creds.OAuth != nil {
		persist := func(t *oauth2.Token) error {
			return config.StoreOAuthCredentials(t, creds.ProjectID)
		}
		return NewTigerClientWithToken(cfg, creds.OAuth, persist)
	}
	return NewTigerClient(cfg, creds.APIKey)
}

// persistingTokenSource wraps a TokenSource and invokes persist on each
// rotation. Persist failures are swallowed: the in-memory token is still
// valid; the next CLI invocation re-mints anyway.
type persistingTokenSource struct {
	base    oauth2.TokenSource
	persist func(*oauth2.Token) error
	last    string
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	if tok.AccessToken != p.last {
		_ = p.persist(tok)
		p.last = tok.AccessToken
	}
	return tok, nil
}

func (e *Error) Error() string {
	if e == nil {
		return "unknown error"
	}
	msg := ""
	if e.Message != nil {
		msg = *e.Message
	}
	if e.Details != nil && *e.Details != "" {
		if msg != "" {
			return msg + ": " + *e.Details
		}
		return *e.Details
	}
	if msg != "" {
		return msg
	}
	return "unknown error"
}
