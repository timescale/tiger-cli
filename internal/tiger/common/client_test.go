package common

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
						authInfo := api.AuthInfo{
							Type: api.ApiKey,
						}
						authInfo.ApiKey.PublicKey = "test-access-key"
						authInfo.ApiKey.Project.Id = "proj-12345"
						authInfo.ApiKey.Project.Name = "Test Project"
						authInfo.ApiKey.Project.PlanType = "FREE"
						authInfo.ApiKey.Name = "Test Credentials"
						authInfo.ApiKey.IssuingUser.Name = "Test User"
						authInfo.ApiKey.IssuingUser.Email = "test@example.com"
						authInfo.ApiKey.IssuingUser.Id = "user-123"
						authInfo.ApiKey.Created = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(authInfo)
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
