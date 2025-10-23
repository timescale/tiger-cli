package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestGraphQLUserAgent(t *testing.T) {
	// Set up a test server that captures the User-Agent header
	var capturedUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserAgent = r.Header.Get("User-Agent")

		// Return a valid GraphQL response
		response := GraphQLResponse[GetUserData]{
			Data: &GetUserData{
				GetUser: User{
					ID:    "test-user-id",
					Name:  "Test User",
					Email: "test@example.com",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create a GraphQL client pointing to our test server
	client := &GraphQLClient{
		URL: server.URL,
	}

	// Make a request
	_, err := client.getUser(context.Background(), "test-access-token")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify the User-Agent header was set correctly
	expectedUserAgent := "tiger-cli/" + config.Version
	if capturedUserAgent != expectedUserAgent {
		t.Errorf("Expected User-Agent %q, got %q", expectedUserAgent, capturedUserAgent)
	}
}

func TestGraphQLUserAgentInAllRequests(t *testing.T) {
	tests := []struct {
		name        string
		requestFunc func(*GraphQLClient, string) (interface{}, error)
	}{
		{
			name: "getUserProjects",
			requestFunc: func(c *GraphQLClient, token string) (interface{}, error) {
				return c.getUserProjects(context.Background(), token)
			},
		},
		{
			name: "getUser",
			requestFunc: func(c *GraphQLClient, token string) (interface{}, error) {
				return c.getUser(context.Background(), token)
			},
		},
		{
			name: "createPATRecord",
			requestFunc: func(c *GraphQLClient, token string) (interface{}, error) {
				return c.createPATRecord(context.Background(), token, "test-project-id", "test-pat-name")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedUserAgent string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedUserAgent = r.Header.Get("User-Agent")

				// Return appropriate response based on the query
				var response interface{}
				if tt.name == "getUserProjects" {
					response = GraphQLResponse[GetAllProjectsData]{
						Data: &GetAllProjectsData{
							GetAllProjects: []Project{
								{ID: "test-id", Name: "test-project"},
							},
						},
					}
				} else if tt.name == "getUser" {
					response = GraphQLResponse[GetUserData]{
						Data: &GetUserData{
							GetUser: User{ID: "test-id", Name: "Test User", Email: "test@example.com"},
						},
					}
				} else if tt.name == "createPATRecord" {
					response = GraphQLResponse[CreatePATRecordData]{
						Data: &CreatePATRecordData{
							CreatePATRecord: PATRecordResponse{
								ClientCredentials: struct {
									AccessKey string `json:"accessKey"`
									SecretKey string `json:"secretKey"`
								}{
									AccessKey: "test-access-key",
									SecretKey: "test-secret-key",
								},
							},
						},
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client := &GraphQLClient{URL: server.URL}
			_, err := tt.requestFunc(client, "test-token")
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			expectedUserAgent := "tiger-cli/" + config.Version
			if capturedUserAgent != expectedUserAgent {
				t.Errorf("Expected User-Agent %q, got %q", expectedUserAgent, capturedUserAgent)
			}
		})
	}
}
