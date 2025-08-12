package cmd

import (
	"fmt"
	"testing"
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
