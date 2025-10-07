package cmd

import (
	"fmt"
	"testing"

	"github.com/timescale/tiger-cli/internal/tiger/api"
)

func TestExitCodeError(t *testing.T) {
	// Test the exitCodeError type
	originalErr := fmt.Errorf("test error")
	exitErr := exitWithCode(42, originalErr)

	if exitErr.Error() != "test error" {
		t.Errorf("Expected error message 'test error', got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(exitCodeError); ok {
		if exitCodeErr.ExitCode() != 42 {
			t.Errorf("Expected exit code 42, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("exitWithCode should return exitCodeError")
	}
}

func TestExitCodeError_NilError(t *testing.T) {
	exitErr := exitWithCode(1, nil)

	if exitErr.Error() != "" {
		t.Errorf("Expected empty error message for nil error, got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(interface{ ExitCode() int }); ok {
		if exitCodeErr.ExitCode() != 1 {
			t.Errorf("Expected exit code 1, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("exitWithCode should return exitCodeError")
	}
}

func TestExitAuthenticationError(t *testing.T) {
	originalErr := fmt.Errorf("authentication failed: invalid API key")
	exitErr := exitWithCode(ExitAuthenticationError, originalErr)

	if exitErr.Error() != "authentication failed: invalid API key" {
		t.Errorf("Expected error message 'authentication failed: invalid API key', got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(interface{ ExitCode() int }); ok {
		if exitCodeErr.ExitCode() != ExitAuthenticationError {
			t.Errorf("Expected exit code %d (ExitAuthenticationError), got %d", ExitAuthenticationError, exitCodeErr.ExitCode())
		}
		if exitCodeErr.ExitCode() != 4 {
			t.Errorf("Expected exit code 4 for authentication error, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("exitWithCode should return exitCodeError with ExitCode method")
	}
}

func TestExitPermissionDenied(t *testing.T) {
	originalErr := fmt.Errorf("permission denied: insufficient access to service")
	exitErr := exitWithCode(ExitPermissionDenied, originalErr)

	if exitErr.Error() != "permission denied: insufficient access to service" {
		t.Errorf("Expected error message 'permission denied: insufficient access to service', got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(interface{ ExitCode() int }); ok {
		if exitCodeErr.ExitCode() != ExitPermissionDenied {
			t.Errorf("Expected exit code %d (ExitPermissionDenied), got %d", ExitPermissionDenied, exitCodeErr.ExitCode())
		}
		if exitCodeErr.ExitCode() != 5 {
			t.Errorf("Expected exit code 5 for permission denied, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("exitWithCode should return exitCodeError with ExitCode method")
	}
}

func TestFormatAPIError(t *testing.T) {
	tests := []struct {
		name     string
		apiErr   *api.Error
		fallback string
		expected string
	}{
		{
			name: "API error with message",
			apiErr: &api.Error{
				Code:    strPtr("ENTITLEMENT_ERROR"),
				Message: strPtr("Unauthorized. Entitlement check has failed."),
			},
			fallback: "fallback message",
			expected: "Unauthorized. Entitlement check has failed.",
		},
		{
			name:     "nil API error",
			apiErr:   nil,
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name: "API error with nil message",
			apiErr: &api.Error{
				Code:    strPtr("ERROR_CODE"),
				Message: nil,
			},
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name: "API error with empty message",
			apiErr: &api.Error{
				Code:    strPtr("ERROR_CODE"),
				Message: strPtr(""),
			},
			fallback: "fallback message",
			expected: "fallback message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := formatAPIError(tt.apiErr, tt.fallback)
			if err.Error() != tt.expected {
				t.Errorf("Expected error message '%s', got '%s'", tt.expected, err.Error())
			}
		})
	}
}

func TestFormatAPIErrorFromBody(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		fallback string
		expected string
	}{
		{
			name:     "valid JSON with error message",
			body:     []byte(`{"code":"ENTITLEMENT_ERROR","message":"Unauthorized. Entitlement check has failed."}`),
			fallback: "fallback message",
			expected: "Unauthorized. Entitlement check has failed.",
		},
		{
			name:     "empty body",
			body:     []byte{},
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name:     "nil body",
			body:     nil,
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name:     "invalid JSON",
			body:     []byte(`{invalid json}`),
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name:     "valid JSON with empty message",
			body:     []byte(`{"code":"ERROR_CODE","message":""}`),
			fallback: "fallback message",
			expected: "fallback message",
		},
		{
			name:     "valid JSON with null message",
			body:     []byte(`{"code":"ERROR_CODE","message":null}`),
			fallback: "fallback message",
			expected: "fallback message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := formatAPIErrorFromBody(tt.body, tt.fallback)
			if err.Error() != tt.expected {
				t.Errorf("Expected error message '%s', got '%s'", tt.expected, err.Error())
			}
		})
	}
}

// Helper function for creating string pointers
func strPtr(s string) *string {
	return &s
}
