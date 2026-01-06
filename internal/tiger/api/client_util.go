package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

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

// NewTigerClient creates a new API client with the given config, API key, and project ID
func NewTigerClient(cfg *config.Config, apiKey string) (*ClientWithResponses, error) {
	// Use shared HTTP client with resource limits
	httpClient := getHTTPClient()

	// Create the API client
	client, err := NewClientWithResponses(cfg.APIURL, WithHTTPClient(httpClient), WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		// Add API key to Authorization header
		encodedKey := base64.StdEncoding.EncodeToString([]byte(apiKey))
		req.Header.Set("Authorization", "Basic "+encodedKey)
		// Add User-Agent header to identify CLI version
		req.Header.Set("User-Agent", fmt.Sprintf("tiger-cli/%s", config.Version))
		return nil
	}))

	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return client, nil
}

// Error implements the error interface for the Error type.
// This allows Error values to be used directly as Go errors.
func (e *Error) Error() string {
	if e == nil {
		return "unknown error"
	}
	if e.Message != nil && *e.Message != "" {
		return *e.Message
	}
	return "unknown error"
}
