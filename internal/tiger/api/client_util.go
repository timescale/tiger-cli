package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/tigerdata/tiger-cli/internal/tiger/config"
)

// NewTigerClient creates a new API client with the given API key
func NewTigerClient(apiKey string) (*ClientWithResponses, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	
	// Create HTTP client with reasonable timeout
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	// Create the API client
	client, err := NewClientWithResponses(cfg.APIURL, WithHTTPClient(httpClient), WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		// Add API key to Authorization header
		req.Header.Set("Authorization", "Bearer "+apiKey)
		return nil
	}))
	
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}
	
	return client, nil
}

// ValidateAPIKey validates the API key by making a test API call
func ValidateAPIKey(apiKey string) error {
	client, err := NewTigerClient(apiKey)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	
	return ValidateAPIKeyWithClient(client)
}

// ValidateAPIKeyWithClient validates the API key using the provided client interface
func ValidateAPIKeyWithClient(client ClientWithResponsesInterface) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// We need a project ID to make most API calls, but we don't have one yet during login.
	// For now, let's try to get any project's services which should fail with 400/404 if the API key is invalid
	// and succeed (even if empty) if the API key is valid.
	// TODO: Find a better endpoint for API key validation that doesn't require project ID
	
	// Try to call a simple endpoint - we'll use a dummy project ID
	// The API should return 401/403 for invalid API key, and 404 for non-existent project
	dummyProjectID := "00000000-0000-0000-0000-000000000000"
	resp, err := client.GetProjectsProjectIdServicesWithResponse(ctx, dummyProjectID)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}
	
	// Check the response status
	switch resp.StatusCode() {
	case 401, 403:
		return fmt.Errorf("invalid API key: authentication failed")
	case 404:
		// Project not found is OK - it means the API key is valid but project doesn't exist
		return nil
	case 200:
		// Success - API key is valid
		return nil
	default:
		return fmt.Errorf("unexpected API response: %d", resp.StatusCode())
	}
}