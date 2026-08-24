package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/config"
)

// HTTPClient is the shared HTTP client all outgoing requests should use,
// giving every request a 30-second timeout and the CLI's User-Agent by
// default. A call that needs a shorter bound should layer a
// context.WithTimeout over the call rather than build a separate client; a
// call that needs a longer one (e.g. downloading a large release archive) is
// a legitimate reason to use a dedicated client instead.
var HTTPClient = &http.Client{
	Timeout:   30 * time.Second,
	Transport: userAgentTransport{},
}

// userAgentTransport stamps the CLI's User-Agent onto every outgoing request.
// It's HTTPClient's Transport, so anything built on HTTPClient gets it for
// free — including a Bearer-authenticated client's requests and its token
// refreshes, both of which route through this same Transport (see
// NewTigerClientWithToken).
type userAgentTransport struct{}

func (userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", userAgent())
	return http.DefaultTransport.RoundTrip(req)
}

// userAgent returns the User-Agent the CLI sends on HTTP requests.
func userAgent() string {
	return fmt.Sprintf("tiger-cli/%s (%s/%s)", config.Version, runtime.GOOS, runtime.GOARCH)
}

// apiKey must be in "publicKey:secretKey" format.
func NewTigerClient(cfg *config.Config, apiKey string) (*ClientWithResponses, error) {
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(apiKey))
	client, err := NewClientWithResponses(cfg.APIURL, WithHTTPClient(HTTPClient), WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", authHeader)
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

	// Stash our shared client in the context: oauth2 uses it both for token
	// refresh requests and to seed the Base transport (and Timeout) of the
	// *http.Client returned by oauth2.NewClient below, so the returned
	// client's ordinary API requests get our 30s timeout and User-Agent too.
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, HTTPClient)

	var src oauth2.TokenSource = oauthCfg.TokenSource(ctx, token)
	if persist != nil {
		src = &persistingTokenSource{base: src, persist: persist, last: token.AccessToken}
	}

	httpClient := oauth2.NewClient(ctx, src)

	client, err := NewClientWithResponses(cfg.APIURL, WithHTTPClient(httpClient))
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
			return cfg.StoreOAuthCredentials(t, creds.ProjectID)
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
