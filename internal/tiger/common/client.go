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

var (
	// GetStoredCredentials loads the stored credentials (PAT or OAuth) from the
	// keyring or fallback file. It's a package var so tests can override it to
	// inject credentials of either shape.
	GetStoredCredentials = config.GetStoredCredentials

	// Cache of validated API Keys. Useful for avoided unnecessary calls to the
	// /auth/info and /analytics/identify endpoints when the API client is
	// loaded multiple times using credentials provided via the TIGER_PUBLIC_KEY
	// and TIGER_SECRET_KEY env vars (e.g. when using the MCP server, which
	// re-fetches the API client for each tool call).
	validatedAPIKeyCache = map[string]*api.AuthInfo{}
)

// NewAPIClient initializes a [api.ClientWithResponses] and returns it along
// with the current project ID. Credentials are pulled from the environment (if
// present), or loaded from storage (either the keyring or fallback file). When
// pulled from the environment, the credentials are first validated by hitting
// the /auth/info endpoint (which also allows us to fetch the project ID), and
// the user is identified for the sake of analytics by hitting the /analytics/identify
// endpoint. When credentials are pulled from storage, those operations should
// have already been performed via `tiger auth login`.
func NewAPIClient(ctx context.Context, cfg *config.Config) (*api.ClientWithResponses, string, error) {
	// Credentials in the environment take priority
	publicKey := os.Getenv("TIGER_PUBLIC_KEY")
	secretKey := os.Getenv("TIGER_SECRET_KEY")

	// If there were no credentials in the environment, try to load stored credentials
	if publicKey == "" && secretKey == "" {
		stored, err := GetStoredCredentials()
		if err != nil {
			return nil, "", ExitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err))
		}

		client, err := api.NewTigerClientForCredentials(cfg, stored)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create API client: %w", err)
		}

		// Credentials were already verified and the user was already identified
		// for analytics via `tiger auth login`.
		return client, stored.ProjectID, nil
	}

	// Create API client
	apiKey := fmt.Sprintf("%s:%s", publicKey, secretKey)
	client, err := api.NewTigerClient(cfg, apiKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create API client: %w", err)
	}

	// Check whether this API Key has already been validated, and use the
	// cached auth info if so. Otherwise, validate it.
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
// returns the caller's identity. It also identifies the user for the sake of
// analytics. Only PAT credentials reach this path, so the response always
// carries the apiKey branch.
func ValidateAPIKey(ctx context.Context, cfg *config.Config, client *api.ClientWithResponses) (*api.AuthInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.GetAuthInfoWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}

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
	if authInfo.ApiKey == nil {
		return nil, fmt.Errorf("expected a PAT credential")
	}
	apiKey := authInfo.ApiKey

	a := analytics.New(cfg, client, apiKey.Project.Id)
	a.Identify(
		analytics.Property("userId", apiKey.IssuingUser.Id),
		analytics.Property("email", string(apiKey.IssuingUser.Email)),
		analytics.Property("planType", apiKey.Project.PlanType),
	)

	return authInfo, nil
}

// IdentifyOAuthUser sends an analytics Identify for an OAuth (PKCE) login,
// using the token-authenticated client built during login. It fetches the
// caller's identity via /auth/info. Best-effort.
func IdentifyOAuthUser(ctx context.Context, cfg *config.Config, client *api.ClientWithResponses, projectID string) {
	// Skip the /auth/info round-trip entirely when analytics is disabled.
	a := analytics.New(cfg, client, projectID)
	if !a.Enabled() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.GetAuthInfoWithResponse(ctx)
	if err != nil || resp.JSON200 == nil || resp.JSON200.Oauth == nil {
		return
	}
	user := resp.JSON200.Oauth.User

	a.Identify(
		analytics.Property("userId", user.Id),
		analytics.Property("email", string(user.Email)),
	)
}
