package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// Shared HTTP client with resource limits to prevent resource exhaustion under load
var (
	httpClientOnce   sync.Once
	sharedHTTPClient *http.Client
)

// getHTTPClient returns a singleton HTTP client with essential resource limits
// Focuses on preventing resource leaks while using reasonable Go defaults elsewhere
func getHTTPClient() *http.Client {
	httpClientOnce.Do(func() {
		// Clone default transport to inherit sensible defaults, then customize key settings
		transport := http.DefaultTransport.(*http.Transport).Clone()

		// Essential resource limits to prevent exhaustion
		transport.MaxIdleConns = 100                 // Limit total idle connections
		transport.MaxIdleConnsPerHost = 10           // Limit per-host idle connections
		transport.IdleConnTimeout = 90 * time.Second // Clean up idle connections

		sharedHTTPClient = &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second, // Overall request timeout
		}
	})
	return sharedHTTPClient
}

// NewTigerClient creates a new API client with the given API key
func NewTigerClient(apiKey string) (*ClientWithResponses, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Use shared HTTP client with resource limits
	httpClient := getHTTPClient()

	// Create the API client
	client, err := NewClientWithResponses(cfg.APIURL, WithHTTPClient(httpClient), WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		// Add API key to Authorization header
		encodedKey := base64.StdEncoding.EncodeToString([]byte(apiKey))
		req.Header.Set("Authorization", "Basic "+encodedKey)
		return nil
	}))

	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return client, nil
}

// ValidateAPIKey validates the API key by making a test API call
func ValidateAPIKey(apiKey string, projectID string) error {
	client, err := NewTigerClient(apiKey)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	return ValidateAPIKeyWithClient(client, projectID)
}

// ValidateAPIKeyWithClient validates the API key using the provided client interface
func ValidateAPIKeyWithClient(client ClientWithResponsesInterface, projectID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use provided project ID if available, otherwise use a dummy one
	targetProjectID := projectID
	if targetProjectID == "" {
		// Use a dummy project ID for validation when none is provided
		targetProjectID = "00000000-0000-0000-0000-000000000000"
	}

	// Try to call a simple endpoint
	// The API should return 401/403 for invalid API key, and 404 for non-existent project
	resp, err := client.GetProjectsProjectIdServicesWithResponse(ctx, targetProjectID)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	// Check the response status
	switch resp.StatusCode() {
	case 401, 403:
		return FormatAPIErrorFromBody(resp.Body, "invalid API key: authentication failed")
	case 404:
		// Project not found is OK - it means the API key is valid but project doesn't exist
		return nil
	case 200:
		// Success - API key is valid
		return nil
	default:
		statusCode := resp.StatusCode()
		if statusCode >= 400 && statusCode < 500 {
			return FormatAPIErrorFromBody(resp.Body, fmt.Sprintf("unexpected API response: %d", statusCode))
		}
		return fmt.Errorf("unexpected API response: %d", statusCode)
	}
}

// FormatAPIError creates an error message from an API error response.
// If the API error contains a message, it will be used; otherwise the fallback message is returned.
func FormatAPIError(apiErr *Error, fallback string) error {
	if apiErr != nil && apiErr.Message != nil && *apiErr.Message != "" {
		return errors.New(*apiErr.Message)
	}
	return errors.New(fallback)
}

// FormatAPIErrorFromBody attempts to parse an API error from a response body.
// If the body contains a valid API error with a message, it will be used; otherwise the fallback message is returned.
func FormatAPIErrorFromBody(body []byte, fallback string) error {
	if len(body) > 0 {
		var apiErr Error
		if err := json.Unmarshal(body, &apiErr); err == nil {
			if apiErr.Message != nil && *apiErr.Message != "" {
				return errors.New(*apiErr.Message)
			}
		}
	}
	return errors.New(fallback)
}

// Error implements the error interface for the Error type.
// This allows Error values to be used directly as Go errors.
func (e *Error) Error() string {
	if e == nil {
		return "unknown error"
	}
	if e.Message != nil && *e.Message != "" {
		return *e.Message
	}
	return "unknown error"
}

// ResponseError wraps an API response with an error, preserving the status code
// for proper exit code mapping in CLI commands
type ResponseError struct {
	statusCode int
	response   *http.Response
	err        error
}

func (r *ResponseError) Error() string {
	return r.err.Error()
}

func (r *ResponseError) StatusCode() int {
	return r.statusCode
}

func (r *ResponseError) HTTPResponse() *http.Response {
	return r.response
}

func (r *ResponseError) Unwrap() error {
	return r.err
}

// NewResponseError creates an error from an API response
// It attempts to extract the error message from the response body
func NewResponseError(response *http.Response) error {
	if response == nil {
		return errors.New("unknown error")
	}

	var err error
	bodyBytes, readErr := io.ReadAll(response.Body)
	err = errors.New(string(bodyBytes))
	return &ResponseError{
		statusCode: response.StatusCode,
		response:   response,
		err:        err,
	}
	if readErr == nil && len(bodyBytes) > 0 {
		var apiErr Error
		if unmarshalErr := json.Unmarshal(bodyBytes, &apiErr); unmarshalErr == nil {
			err = &apiErr
		} else {
			err = fmt.Errorf("API error: status %d", unmarshalErr.Error())
		}
	} else {
		err = fmt.Errorf("API error: status %d", response.StatusCode)
	}

	return &ResponseError{
		statusCode: response.StatusCode,
		response:   response,
		err:        err,
	}
}
