package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
)

func TestValidateAPIKey(t *testing.T) {
	// Initialize logger for analytics code
	if err := logging.Init(false); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}

	tests := []struct {
		name              string
		setupServer       func() *httptest.Server
		expectedProjectID string
		expectedPublicKey string
		expectedError     string
	}{
		{
			name: "valid API key - returns auth info",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/auth/info" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{
							"type": "apiKey",
							"apiKey": {
								"public_key": "test-access-key",
								"name": "Test Credentials",
								"created": "2025-01-01T00:00:00Z",
								"project": {"id": "proj-12345", "name": "Test Project", "plan_type": "free"},
								"issuing_user": {"id": "user-123", "name": "Test User", "email": "test@example.com"}
							}
						}`))
					} else if r.URL.Path == "/analytics/identify" {
						// Analytics identify endpoint (called after auth info)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"status": "success"}`))
					} else {
						t.Errorf("Unexpected path: %s", r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
					}
				}))
			},
			expectedProjectID: "proj-12345",
			expectedPublicKey: "test-access-key",
			expectedError:     "",
		},
		{
			name: "invalid API key - 401 response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte(`{"message": "Invalid or missing authentication credentials"}`))
				}))
			},
			expectedProjectID: "",
			expectedPublicKey: "",
			expectedError:     "Invalid or missing authentication credentials",
		},
		{
			name: "invalid API key - 403 response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					w.Write([]byte(`{"message": "Invalid or missing authentication credentials"}`))
				}))
			},
			expectedProjectID: "",
			expectedPublicKey: "",
			expectedError:     "Invalid or missing authentication credentials",
		},
		{
			name: "unexpected response - 500",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			expectedProjectID: "",
			expectedPublicKey: "",
			expectedError:     "unexpected API response: 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			// Setup test config with the test server URL
			cfg, err := config.UseTestConfig(t.TempDir(), map[string]any{
				"api_url": server.URL,
			})
			if err != nil {
				t.Fatalf("Failed to setup test config: %v", err)
			}

			client, err := api.NewTigerClient(cfg, "test-api-key")
			require.NoError(t, err)

			authInfo, err := ValidateAPIKey(context.Background(), cfg, client)

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if authInfo == nil {
					t.Fatal("Expected auth info to be returned, got nil")
				}
				if authInfo.ApiKey.Project.Id != tt.expectedProjectID {
					t.Errorf("Expected project ID %q, got %q", tt.expectedProjectID, authInfo.ApiKey.Project.Id)
				}
				if authInfo.ApiKey.PublicKey != tt.expectedPublicKey {
					t.Errorf("Expected access key %q, got %q", tt.expectedPublicKey, authInfo.ApiKey.PublicKey)
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
