package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestDBTestConnection_NoServiceID(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config with no default service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	mockTestPAT(t)

	// Execute db test-connection command without service ID
	_, err = executeDBCommand(t.Context(), "db", "test-connection")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestDBTestConnection_NoAuth(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config with service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	mockNotLoggedIn(t)

	// Execute db test-connection command
	_, err = executeDBCommand(t.Context(), "db", "test-connection")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestDBTestConnection_TimeoutParsing(t *testing.T) {
	testCases := []struct {
		name           string
		timeoutFlag    string
		expectError    bool
		expectedOutput string
	}{
		{
			name:        "Valid duration - seconds",
			timeoutFlag: "30s",
			expectError: true, // Will fail due to unreachable server
		},
		{
			name:        "Valid duration - minutes",
			timeoutFlag: "5m",
			expectError: true, // Will fail due to unreachable server
		},
		{
			name:        "Valid duration - hours",
			timeoutFlag: "1h",
			expectError: true, // Will fail due to unreachable server
		},
		{
			name:        "Valid duration - mixed",
			timeoutFlag: "1h30m45s",
			expectError: true, // Will fail due to unreachable server
		},
		{
			name:        "Zero timeout (no timeout)",
			timeoutFlag: "0",
			expectError: true, // Will fail due to unreachable server
		},
		{
			name:           "Invalid duration format",
			timeoutFlag:    "invalid",
			expectError:    true,
			expectedOutput: "invalid duration",
		},
		{
			name:        "Negative duration",
			timeoutFlag: "-5s",
			expectError: true,
			// Note: API call fails before validation, so we don't get the validation error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := setupDBTest(t)

			// Set up config
			_, err := config.UseTestConfig(tmpDir, map[string]any{
				"api_url":    "http://localhost:9999", // Non-existent server
				"service_id": "svc-12345",
			})
			if err != nil {
				t.Fatalf("Failed to save test config: %v", err)
			}

			// Mock authentication
			mockTestPAT(t)

			// Execute db test-connection command with timeout flag
			_, err = executeDBCommand(t.Context(), "db", "test-connection", "--timeout", tc.timeoutFlag)

			if !tc.expectError {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				return
			}

			// All test cases expect errors due to invalid duration or unreachable server
			if err == nil {
				t.Error("Expected error but got none")
				return
			}

			// Check if error message contains expected content for invalid format
			if tc.expectedOutput != "" && !strings.Contains(err.Error(), tc.expectedOutput) {
				t.Errorf("Expected error to contain '%s', got: %v", tc.expectedOutput, err)
			}

			// For valid durations that fail due to server unreachable, check exit code
			if tc.expectedOutput == "" {
				var codeErr common.ExitCodeError
				if errors.As(err, &codeErr) {
					// Network errors map to ExitTimeout (no response) or ExitInvalidParameters
					if codeErr.ExitCode() != common.ExitTimeout && codeErr.ExitCode() != common.ExitInvalidParameters {
						t.Errorf("Expected exit code %d or %d, got %d", common.ExitTimeout, common.ExitInvalidParameters, codeErr.ExitCode())
					}
				} else {
					t.Error("Expected common.ExitCodeError")
				}
			}
		})
	}
}

func TestTestDatabaseConnection_InvalidConnectionString(t *testing.T) {
	// Test with truly invalid connection string that should fail at sql.Open

	cmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	// Test with malformed connection string (should return common.ExitInvalidParameters)
	invalidConnectionString := "this is not a valid connection string at all"
	ctx := context.Background()
	err := testDatabaseConnection(ctx, invalidConnectionString, 1*time.Second, cmd)

	if err == nil {
		t.Error("Expected error for invalid connection string")
	}

	// The exact code depends on where it fails
	var codeErr common.ExitCodeError
	if errors.As(err, &codeErr) {
		if codeErr.ExitCode() != common.ExitTimeout && codeErr.ExitCode() != common.ExitInvalidParameters {
			t.Errorf("Expected exit code %d or %d for invalid connection string, got %d", common.ExitTimeout, common.ExitInvalidParameters, codeErr.ExitCode())
		}
	} else {
		t.Error("Expected common.ExitCodeError for invalid connection string")
	}
}

func TestTestDatabaseConnection_Timeout(t *testing.T) {
	// Test timeout functionality with a connection to a non-existent server
	cmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	// Use a connection string to a non-routable IP to test timeout
	timeoutConnectionString := "postgresql://user:pass@192.0.2.1:5432/db?sslmode=disable&connect_timeout=1"

	ctx := context.Background()
	start := time.Now()
	err := testDatabaseConnection(ctx, timeoutConnectionString, 1*time.Second, cmd) // 1 second timeout
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error for timeout connection")
	}

	// Should complete within reasonable time (not hang)
	if duration > 3*time.Second {
		t.Errorf("Connection test took too long: %v", duration)
	}

	assertExitCode(t, err, common.ExitTimeout)
}

func TestIsConnectionRejected(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name: "PostgreSQL error code 57P03 (ERRCODE_CANNOT_CONNECT_NOW)",
			err: &pgconn.PgError{
				Code:    "57P03",
				Message: "the database system is starting up",
			},
			expected: true,
		},
		{
			name: "PostgreSQL authentication error (28P01)",
			err: &pgconn.PgError{
				Code:    "28P01",
				Message: "password authentication failed for user \"test\"",
			},
			expected: false,
		},
		{
			name: "PostgreSQL invalid authorization error (28000)",
			err: &pgconn.PgError{
				Code:    "28000",
				Message: "role \"nonexistent\" does not exist",
			},
			expected: false,
		},
		{
			name: "PostgreSQL database does not exist (3D000)",
			err: &pgconn.PgError{
				Code:    "3D000",
				Message: "database \"nonexistent\" does not exist",
			},
			expected: false,
		},
		{
			name:     "Non-PostgreSQL error (connection refused)",
			err:      fmt.Errorf("dial tcp: connection refused"),
			expected: false,
		},
		{
			name:     "Non-PostgreSQL error (network unreachable)",
			err:      fmt.Errorf("dial tcp: network is unreachable"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isConnectionRejected(tc.err)

			if result != tc.expected {
				t.Errorf("Expected isConnectionRejected to return %v for error %v, got %v",
					tc.expected, tc.err, result)
			}
		})
	}
}
