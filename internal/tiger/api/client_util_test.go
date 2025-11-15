package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

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
	client, err := api.NewTigerClient(cfg, "test-api-key")
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
	client, err := api.NewTigerClient(cfg, apiKey)
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
