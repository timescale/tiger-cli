package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"go.uber.org/zap"
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

// TigerClient wraps the generated API client with configuration and convenience methods
type TigerClient struct {
	*ClientWithResponses
	Config    *config.Config
	ProjectID string
}

// NewTigerClient creates a new API client with the given config, API key, and project ID
func NewTigerClient(cfg *config.Config, apiKey string, projectID string) (*TigerClient, error) {
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

	return &TigerClient{
		ClientWithResponses: client,
		Config:              cfg,
		ProjectID:           projectID,
	}, nil
}

// TryInitTigerClient tries to load credentials and initialize a [TigerClient].
// It returns nil if credentials do not exist or it otherwise fails to create a
// new client. This function is intended to be used when the caller does not
// need an API client to function, but would use one if available (e.g. to
// track analytics events).
func TryInitTigerClient(cfg *config.Config) *TigerClient {
	apiKey, projectID, err := config.GetCredentials()
	if err != nil {
		return nil
	}

	client, err := NewTigerClient(cfg, apiKey, projectID)
	if err != nil {
		return nil
	}

	return client
}

// Track sends an analytics event with the provided event name and properties.
// It automatically includes common properties like ProjectID, OS, and
// architecture. Events are only sent if the client is initialized and
// analytics are enabled in the config, otherwise they are skipped.
func (c *TigerClient) Track(event string, properties map[string]any) {
	logger := logging.GetLogger().With(
		zap.String("event", event),
		zap.Any("properties", properties),
	)

	// Check for cases where the client was not initialized
	// (e.g. because API credentials are not available)
	if c == nil {
		if c.Config.Debug {
			logger.Debug("Analytics event skipped (client not initialized)")
		}
		return
	}

	// Check if analytics is disabled
	if !c.Config.Analytics {
		if c.Config.Debug {
			logger.Debug("Analytics event skipped (analytics disabled)")
		}
		return
	}

	// Build properties map with common properties
	allProperties := map[string]any{
		"project_id": c.ProjectID,
		"version":    config.Version,
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	}

	// Merge in user-provided properties (they can override common properties if needed)
	for k, v := range properties {
		allProperties[k] = v
	}

	// Send the event
	// NOTE: We intentionally use context.Background() here so we can
	// track analytics events even if a command times out or is canceled.
	resp, err := c.PostTrackWithResponse(context.Background(), PostTrackJSONRequestBody{
		Event:      event,
		Properties: &allProperties,
	})
	if err != nil {
		// Log error but don't fail the operation - analytics should never block user actions
		if c.Config.Debug {
			logger.Debug("Failed to send analytics event", zap.Error(err))
		}
		return
	}

	// Check if the API layer skipped the event
	if resp.JSON200 != nil && resp.JSON200.Status != nil && *resp.JSON200.Status == "skipped" {
		if c.Config.Debug {
			logger.Debug("Analytics event skipped by API")
		}
	}
}

func (c *TigerClient) TrackErr(event string, err error, properties map[string]any) {
	if properties == nil {
		properties = map[string]any{}
	}
	if err == nil {
		properties["success"] = true
	} else {
		properties["success"] = false
		properties["error"] = err.Error()
	}
	c.Track(event, properties)
}

// ValidateAPIKey validates the API key by making a test API call
func ValidateAPIKey(cfg *config.Config, apiKey string, projectID string) error {
	client, err := NewTigerClient(cfg, apiKey, projectID)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	return ValidateAPIKeyWithClient(client.ClientWithResponses, projectID)
}

// ValidateAPIKeyWithClient validates the API key using the provided client interface
func ValidateAPIKeyWithClient(client ClientWithResponsesInterface, projectID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use provided project ID if available, otherwise use a dummy one
	targetProjectID := projectID
	if targetProjectID == "" {
		// Use a dummy project ID for validation when none is provided
		targetProjectID = "00000000-0000-0000-0000-000000000000"
	}

	// Try to call a simple endpoint
	// The API should return 401/403 for invalid API key, and 404 for non-existent project
	resp, err := client.GetProjectsProjectIdServicesWithResponse(ctx, targetProjectID)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	// Check the response status
	if resp.StatusCode() != 200 {
		if resp.StatusCode() == 404 {
			// Project not found, but API key is valid
			return nil
		}
		if resp.JSON4XX != nil {
			return resp.JSON4XX
		} else {
			return errors.New("unexpected API response: 500")
		}
	} else {
		return nil
	}
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
