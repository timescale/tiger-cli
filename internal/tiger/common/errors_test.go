package common

import (
	"fmt"
	"testing"
)

func TestExitCodeError(t *testing.T) {
	// Test the ExitCodeError type
	originalErr := fmt.Errorf("test error")
	exitErr := ExitWithCode(42, originalErr)

	if exitErr.Error() != "test error" {
		t.Errorf("Expected error message 'test error', got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(ExitCodeError); ok {
		if exitCodeErr.ExitCode() != 42 {
			t.Errorf("Expected exit code 42, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("ExitWithCode should return ExitCodeError")
	}
}

func TestExitCodeError_NilError(t *testing.T) {
	exitErr := ExitWithCode(1, nil)

	if exitErr.Error() != "" {
		t.Errorf("Expected empty error message for nil error, got '%s'", exitErr.Error())
	}

	if exitCodeErr, ok := exitErr.(interface{ ExitCode() int }); ok {
		if exitCodeErr.ExitCode() != 1 {
			t.Errorf("Expected exit code 1, got %d", exitCodeErr.ExitCode())
		}
	} else {
		t.Error("ExitWithCode should return ExitCodeError")
	}
}

func TestExitAuthenticationError(t *testing.T) {
	originalErr := fmt.Errorf("authentication failed: invalid API key")
	exitErr := ExitWithCode(ExitAuthenticationError, originalErr)

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
		t.Error("ExitWithCode should return ExitCodeError with ExitCode method")
	}
}

func TestExitPermissionDenied(t *testing.T) {
	originalErr := fmt.Errorf("permission denied: insufficient access to service")
	exitErr := ExitWithCode(ExitPermissionDenied, originalErr)

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
		t.Error("ExitWithCode should return ExitCodeError with ExitCode method")
	}
}
