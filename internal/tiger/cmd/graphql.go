package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// We currently use a few GraphQL endpoints as part of the OAuth login flow,
// because they were already available and they accept the OAuth access token
// for authentication (whereas savannah-public only accepts the client
// credentials/API Key).
type GraphQLClient struct {
	URL string
}

// GraphQLResponse represents a generic GraphQL response wrapper
type GraphQLResponse[T any] struct {
	Data   *T             `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

// GraphQLError represents an error returned in a GraphQL response
type GraphQLError struct {
	Message string `json:"message"`
}

// GetAllProjectsData represents the data from the getAllProjects query
type GetAllProjectsData struct {
	GetAllProjects []Project `json:"getAllProjects"`
}

// Project represents a project from the GraphQL API
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// getUserProjects fetches the list of projects the user has access to
func (c *GraphQLClient) getUserProjects(ctx context.Context, accessToken string) ([]Project, error) {
	query := `
		query GetAllProjects {
			getAllProjects {
				id
				name
			}
		}
	`

	response, err := makeGraphQLRequest[GetAllProjectsData](ctx, c.URL, accessToken, query, nil)
	if err != nil {
		return nil, err
	}

	return response.GetAllProjects, nil
}

// GetUserData represents the data from the getUser query
type GetUserData struct {
	GetUser User `json:"getUser"`
}

// User represents a user from the GraphQL API
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// getUser fetches the current user's information
func (c *GraphQLClient) getUser(ctx context.Context, accessToken string) (*User, error) {
	query := `
		query GetUser {
			getUser {
				id
				name
				email
			}
		}
	`

	response, err := makeGraphQLRequest[GetUserData](ctx, c.URL, accessToken, query, nil)
	if err != nil {
		return nil, err
	}

	return &response.GetUser, nil
}

// CreatePATRecordData represents the data from the createPATRecord mutation
type CreatePATRecordData struct {
	CreatePATRecord PATRecordResponse `json:"createPATRecord"`
}

// PATRecordResponse represents the response from creating a PAT record
type PATRecordResponse struct {
	ClientCredentials struct {
		AccessKey string `json:"accessKey"`
		SecretKey string `json:"secretKey"`
	} `json:"clientCredentials"`
}

// createPATRecord creates a new PAT record for the given project
func (c *GraphQLClient) createPATRecord(ctx context.Context, accessToken, projectID, patName string) (*PATRecordResponse, error) {
	query := `
		mutation CreatePATRecord($input: CreatePATRecordInput!) {
			createPATRecord(createPATRecordInput: $input) {
				clientCredentials {
					accessKey
					secretKey
				}
			}
		}
	`

	variables := map[string]any{
		"input": map[string]any{
			"projectId": projectID,
			"name":      patName,
		},
	}

	response, err := makeGraphQLRequest[CreatePATRecordData](ctx, c.URL, accessToken, query, variables)
	if err != nil {
		return nil, err
	}

	return &response.CreatePATRecord, nil
}

// makeGraphQLRequest makes a GraphQL request to the API
func makeGraphQLRequest[T any](ctx context.Context, queryURL, accessToken, query string, variables map[string]any) (*T, error) {
	requestBody := map[string]any{
		"query": query,
	}
	if variables != nil {
		requestBody["variables"] = variables
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", queryURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("User-Agent", fmt.Sprintf("tiger-cli/%s", config.Version))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make GraphQL request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GraphQL response: %w", err)
	}

	var response GraphQLResponse[T]
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GraphQL response: %w", err)
	}

	// Check for GraphQL errors
	if len(response.Errors) > 0 {
		var errorMessages []string
		for _, gqlErr := range response.Errors {
			errorMessages = append(errorMessages, gqlErr.Message)
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(errorMessages, "; "))
	}

	if response.Data == nil {
		return nil, fmt.Errorf("GraphQL response contains no data")
	}

	return response.Data, nil
}
