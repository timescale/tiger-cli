package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/api/mocks"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
	"go.uber.org/mock/gomock"
)

func TestValidateAPIKeyWithClient(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*mocks.MockClientWithResponsesInterface)
		expectedError string
	}{
		{
			name: "valid API key - 200 response",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 200},
					}, nil)
			},
			expectedError: "",
		},
		{
			name: "valid API key - 404 response (project not found)",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 404},
					}, nil)
			},
			expectedError: "",
		},
		{
			name: "invalid API key - 401 response",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 401},
						JSON4XX:      &api.ClientError{Message: util.Ptr("Invalid or missing authentication credentials")},
					}, nil)
			},
			expectedError: "Invalid or missing authentication credentials",
		},
		{
			name: "invalid API key - 403 response",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 403},
						JSON4XX:      &api.ClientError{Message: util.Ptr("Invalid or missing authentication credentials")},
					}, nil)
			},
			expectedError: "Invalid or missing authentication credentials",
		},
		{
			name: "unexpected response - 500",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(&api.GetProjectsProjectIdServicesResponse{
						HTTPResponse: &http.Response{StatusCode: 500},
					}, nil)
			},
			expectedError: "unexpected API response: 500",
		},
		{
			name: "network error",
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().
					GetProjectsProjectIdServicesWithResponse(gomock.Any(), "00000000-0000-0000-0000-000000000000").
					Return(nil, context.DeadlineExceeded)
			},
			expectedError: "API call failed: context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockClientWithResponsesInterface(ctrl)
			tt.setupMock(mockClient)

			err := api.ValidateAPIKeyWithClient(context.Background(), mockClient, "")

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
				} else if err.Error() != tt.expectedError {
					t.Errorf("Expected error %q, got %q", tt.expectedError, err.Error())
				}
			}
		})
	}
}

// TestValidateAPIKey_Integration would be an integration test that actually calls the API
// This should be run with a real API key for integration testing
func TestValidateAPIKey_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test would require a real API key and network connectivity
	t.Skip("Integration test requires real API key - implement when needed")
}

func TestNewTigerClientUserAgent(t *testing.T) {
	// Create a test server that captures the User-Agent header
	var capturedUserAgent string
	var requestReceived bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		capturedUserAgent = r.Header.Get("User-Agent")
		// Return a valid JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) // Empty array for services list
	}))
	defer server.Close()

	// Setup test config with the test server URL
	cfg, err := config.UseTestConfig(t.TempDir(), map[string]any{
		"api_url": server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to setup test config: %v", err)
	}

	// Create a new Tiger client
	client, err := api.NewTigerClient(cfg, "test-api-key", "test-project-id")
	if err != nil {
		t.Fatalf("Failed to create Tiger client: %v", err)
	}

	// Make a request to trigger the User-Agent header
	ctx := context.Background()
	_, err = client.GetProjectsProjectIdServicesWithResponse(ctx, "test-project-id")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if !requestReceived {
		t.Fatal("Request was not received by test server")
	}

	// Verify the User-Agent header was set correctly
	expectedUserAgent := "tiger-cli/" + config.Version
	if capturedUserAgent != expectedUserAgent {
		t.Errorf("Expected User-Agent %q, got %q", expectedUserAgent, capturedUserAgent)
	}
}

func TestNewTigerClientAuthorizationHeader(t *testing.T) {
	// Create a test server that captures the Authorization header
	var capturedAuthHeader string
	var requestReceived bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		capturedAuthHeader = r.Header.Get("Authorization")
		// Return a valid JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) // Empty array for services list
	}))
	defer server.Close()

	// Setup test config with the test server URL
	cfg, err := config.UseTestConfig(t.TempDir(), map[string]any{
		"api_url": server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to setup test config: %v", err)
	}

	// Create a new Tiger client with a test API key
	apiKey := "test-api-key:test-secret-key"
	client, err := api.NewTigerClient(cfg, apiKey, "test-project-id")
	if err != nil {
		t.Fatalf("Failed to create Tiger client: %v", err)
	}

	// Make a request to trigger the Authorization header
	ctx := context.Background()
	_, err = client.GetProjectsProjectIdServicesWithResponse(ctx, "test-project-id")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if !requestReceived {
		t.Fatal("Request was not received by test server")
	}

	// Verify the Authorization header was set correctly (should be Base64 encoded)
	if capturedAuthHeader == "" {
		t.Error("Expected Authorization header to be set, but it was empty")
	}
	if len(capturedAuthHeader) < 6 || capturedAuthHeader[:6] != "Basic " {
		t.Errorf("Expected Authorization header to start with 'Basic ', got: %s", capturedAuthHeader)
	}
}
