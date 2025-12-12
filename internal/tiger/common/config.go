package common

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/analytics"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// GetCredentials can be overridden for testing
var GetCredentials = config.GetCredentials

type ConfigAndAPIClient struct {
	*config.Config
	*api.ClientWithResponses
	ProjectID string
}

func LoadConfigAndAPIClient(ctx context.Context) (*ConfigAndAPIClient, error) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Get API key and project ID for authentication
	client, projectID, err := getClientAndProjectID(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &ConfigAndAPIClient{
		Config:              cfg,
		ClientWithResponses: client,
		ProjectID:           projectID,
	}, nil
}

// Cache of validated API Keys. Useful for avoided unnecessary calls to the
// /auth/info and /analytics/identify endpoints when the API client is loaded
// multiple times using credentials provided via the TIGER_PUBLIC_KEY and
// TIGER_SECRET_KEY env vars (e.g. when using the MCP server, which re-fetches
// the API client for each tool call).
var validatedAPIKeyCache = map[string]*api.AuthInfo{}

func getClientAndProjectID(ctx context.Context, cfg *config.Config) (*api.ClientWithResponses, string, error) {
	// Credentials in the environment take priority
	publicKey := os.Getenv("TIGER_PUBLIC_KEY")
	secretKey := os.Getenv("TIGER_SECRET_KEY")

	// If there were no credentials in the environment, try to load stored credentials
	if publicKey == "" && secretKey == "" {
		apiKey, projectID, err := GetCredentials()
		if err != nil {
			return nil, "", ExitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err))
		}

		// Create API client
		client, err := api.NewTigerClient(cfg, apiKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create API client: %w", err)
		}

		// Return immediately. Credentials were already verified and user was
		// already identified for analytics via `tiger auth login`.
		return client, projectID, nil
	}

	// Create API client
	apiKey := fmt.Sprintf("%s:%s", publicKey, secretKey)
	client, err := api.NewTigerClient(cfg, apiKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create API client: %w", err)
	}

	// Check whether this API Key has already been validated, and return the
	// cached auth info if so. Otherwise, validate it
	authInfo, ok := validatedAPIKeyCache[apiKey]
	if !ok {
		// Validate the API key and get auth info by calling the /auth/info endpoint
		authInfo, err = ValidateAPIKey(ctx, cfg, client)
		if err != nil {
			return nil, "", fmt.Errorf("API key validation failed: %w", err)
		}
		validatedAPIKeyCache[apiKey] = authInfo
	}

	return client, authInfo.ApiKey.Project.Id, nil
}

// ValidateAPIKey validates the API key by calling the /auth/info endpoint, and
// returns authentication information. It also identifies the user for the sake
// of analytics.
func ValidateAPIKey(ctx context.Context, cfg *config.Config, client *api.ClientWithResponses) (*api.AuthInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Call the /auth/info endpoint to validate credentials and get auth info
	resp, err := client.GetAuthInfoWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	// Check the response status
	if resp.StatusCode() != 200 {
		if resp.JSON4XX != nil {
			return nil, resp.JSON4XX
		}
		return nil, fmt.Errorf("unexpected API response: %d", resp.StatusCode())
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from API")
	}

	authInfo := resp.JSON200

	// Identify the user with analytics
	a := analytics.New(cfg, client, authInfo.ApiKey.Project.Id)
	a.Identify(
		analytics.Property("userId", authInfo.ApiKey.IssuingUser.Id),
		analytics.Property("email", string(authInfo.ApiKey.IssuingUser.Email)),
		analytics.Property("planType", authInfo.ApiKey.Project.PlanType),
	)

	return authInfo, nil
}
