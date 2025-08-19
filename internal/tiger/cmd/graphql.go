package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// We currently use a few GraphQL endpoints as part of the OAuth login flow,
// because they were already available and they accept the OAuth access token
// for authentication (whereas savannah-public only accepts the client
// credentials/API Key).
type GraphQLClient struct {
	URL string
}

// GetAllProjectsResponse represents the response from the getAllProjects query
type GetAllProjectsResponse struct {
	Data struct {
		GetAllProjects []Project `json:"getAllProjects"`
	} `json:"data"`
}

// Project represents a project from the GraphQL API
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// getUserProjects fetches the list of projects the user has access to
func (c *GraphQLClient) getUserProjects(accessToken string) ([]Project, error) {
	query := `
		query GetAllProjects {
			getAllProjects {
				id
				name
			}
		}
	`

	response, err := makeGraphQLRequest[GetAllProjectsResponse](c.URL, accessToken, query, nil)
	if err != nil {
		return nil, err
	}

	return response.Data.GetAllProjects, nil
}

// GetUserResponse represents the response from the getUser query
type GetUserResponse struct {
	Data struct {
		GetUser User `json:"getUser"`
	} `json:"data"`
}

// User represents a user from the GraphQL API
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// getUser fetches the current user's information
func (c *GraphQLClient) getUser(accessToken string) (*User, error) {
	query := `
		query GetUser {
			getUser {
				id
				name
				email
			}
		}
	`

	response, err := makeGraphQLRequest[GetUserResponse](c.URL, accessToken, query, nil)
	if err != nil {
		return nil, err
	}

	return &response.Data.GetUser, nil
}

// CreatePATRecordResponse represents the response from the createPATRecord mutation
type CreatePATRecordResponse struct {
	Data struct {
		CreatePATRecord PATRecordResponse `json:"createPATRecord"`
	} `json:"data"`
}

// PATRecordResponse represents the response from creating a PAT record
type PATRecordResponse struct {
	ClientCredentials struct {
		AccessKey string `json:"accessKey"`
		SecretKey string `json:"secretKey"`
	} `json:"clientCredentials"`
}

// createPATRecord creates a new PAT record for the given project
func (c *GraphQLClient) createPATRecord(accessToken, projectID, patName string) (*PATRecordResponse, error) {
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

	response, err := makeGraphQLRequest[CreatePATRecordResponse](c.URL, accessToken, query, variables)
	if err != nil {
		return nil, err
	}

	return &response.Data.CreatePATRecord, nil
}

// makeGraphQLRequest makes a GraphQL request to the API
func makeGraphQLRequest[T any](queryURL, accessToken, query string, variables map[string]any) (*T, error) {
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

	req, err := http.NewRequest("POST", queryURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

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

	var response T
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GraphQL response: %w", err)
	}

	return &response, nil
}
