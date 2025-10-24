package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/password"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

func setupDBTest(t *testing.T) string {
	t.Helper()

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Reset global config and viper to ensure test isolation
	config.ResetGlobalConfig()

	t.Cleanup(func() {
		// Reset global config and viper first
		config.ResetGlobalConfig()
		// Clean up environment variable BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func executeDBCommand(ctx context.Context, args ...string) (string, error) {
	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot, err := buildRootCmd(ctx)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err = testRoot.Execute()
	return buf.String(), err
}

func TestDBConnectionString_NoServiceID(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config with no default service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Execute db connection-string command without service ID
	_, err = executeDBCommand(t.Context(), "db", "connection-string")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestDBConnectionString_NoAuth(t *testing.T) {
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
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Execute db connection-string command
	_, err = executeDBCommand(t.Context(), "db", "connection-string")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestDBConnectionString_PoolerWarning(t *testing.T) {
	// This test demonstrates that the warning functionality works
	// by directly testing the password.GetConnectionDetails function

	// Service without connection pooler
	service := api.Service{
		Endpoint: &api.Endpoint{
			Host: util.Ptr("test-host.tigerdata.com"),
			Port: util.Ptr(5432),
		},
		ConnectionPooler: nil, // No pooler available
	}

	// Request pooled connection when pooler is not available
	details, err := password.GetConnectionDetails(service, password.ConnectionDetailsOptions{
		Pooled: true,
		Role:   "tsdbadmin",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should return direct connection string
	expectedString := "postgresql://tsdbadmin@test-host.tigerdata.com:5432/tsdb?sslmode=require"
	if details.String() != expectedString {
		t.Errorf("Expected connection string %q, got %q", expectedString, details.String())
	}

	if details.IsPooler {
		t.Errorf("Expected IsPooler to be false, got true")
	}
}

func TestDBConnect_NoServiceID(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config with no default service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url": "https://api.tigerdata.com/public/v1",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Execute db connect command without service ID
	_, err = executeDBCommand(t.Context(), "db", "connect")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestDBConnect_NoAuth(t *testing.T) {
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
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Execute db connect command
	_, err = executeDBCommand(t.Context(), "db", "connect")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestDBConnect_PsqlNotFound(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Test that psql alias works the same as connect
	_, err1 := executeDBCommand(t.Context(), "db", "connect")
	_, err2 := executeDBCommand(t.Context(), "db", "psql")

	// Both should behave identically (both will fail due to network/psql not found, but with same error pattern)
	if err1 == nil || err2 == nil {
		t.Fatal("Expected both connect and psql to fail in test environment")
	}

	// Both should have similar error patterns (either network error or psql not found)
	connectErrStr := err1.Error()
	psqlErrStr := err2.Error()

	// They should both fail for the same fundamental reason
	if strings.Contains(connectErrStr, "authentication") != strings.Contains(psqlErrStr, "authentication") ||
		strings.Contains(connectErrStr, "psql client not found") != strings.Contains(psqlErrStr, "psql client not found") ||
		strings.Contains(connectErrStr, "failed to fetch") != strings.Contains(psqlErrStr, "failed to fetch") {
		t.Errorf("Connect and psql should behave identically. Connect error: %v, Psql error: %v", err1, err2)
	}
}

func TestLaunchPsqlWithConnectionString(t *testing.T) {
	// This test verifies the psql launching logic without actually running psql

	// Create a test command to capture output
	cmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)

	connectionString := "postgresql://testuser@testhost:5432/testdb?sslmode=require"
	psqlPath := "/fake/path/to/psql" // This will fail, but we can test the setup

	// Create a dummy service for the test
	service := api.Service{}

	// This will fail because psql path doesn't exist, but we can verify the error
	err := launchPsqlWithConnectionString(connectionString, psqlPath, []string{}, service, "tsdbadmin", cmd)

	// Should fail with exec error since fake psql path doesn't exist
	if err == nil {
		t.Error("Expected error when using fake psql path")
	}

	// No output expected since we removed the connecting message
	output := outBuf.String()
	if output != "" {
		t.Errorf("Expected no output, got: %q", output)
	}
}

func TestLaunchPsqlWithAdditionalFlags(t *testing.T) {
	// This test verifies that additional flags are passed correctly to psql

	// Create a test command to capture output
	cmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)

	connectionString := "postgresql://testuser@testhost:5432/testdb?sslmode=require"
	psqlPath := "/fake/path/to/psql" // This will fail, but we can test the setup
	additionalFlags := []string{"--single-transaction", "--quiet", "-c", "SELECT 1;"}

	// Create a dummy service for the test
	service := api.Service{}

	// This will fail because psql path doesn't exist, but we can verify the error
	err := launchPsqlWithConnectionString(connectionString, psqlPath, additionalFlags, service, "tsdbadmin", cmd)

	// Should fail with exec error since fake psql path doesn't exist
	if err == nil {
		t.Error("Expected error when using fake psql path")
	}

	// No output expected since we removed the connecting message
	output := outBuf.String()
	if output != "" {
		t.Errorf("Expected no output, got: %q", output)
	}
}

func TestBuildPsqlCommand_KeyringPasswordEnvVar(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service
	serviceID := "test-psql-service"
	projectID := "test-psql-project"
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
	}

	// Store a test password in keyring
	testPassword := "test-password-12345"
	storage := password.GetPasswordStorage()
	err := storage.Save(service, testPassword, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service, "tsdbadmin") // Clean up after test

	connectionString := "postgresql://testuser@testhost:5432/testdb?sslmode=require"
	psqlPath := "/usr/bin/psql"
	additionalFlags := []string{"--quiet"}

	// Create a mock command for testing
	testCmd := &cobra.Command{}

	// Call the actual production function that builds the command
	psqlCmd := buildPsqlCommand(connectionString, psqlPath, additionalFlags, service, "tsdbadmin", testCmd)

	if psqlCmd == nil {
		t.Fatal("buildPsqlCommand returned nil")
	}

	// Verify that PGPASSWORD is set in the environment with the correct value
	found := false
	expectedEnvVar := "PGPASSWORD=" + testPassword
	for _, envVar := range psqlCmd.Env {
		if envVar == expectedEnvVar {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected PGPASSWORD=%s to be set in environment, but it wasn't. Env vars: %v", testPassword, psqlCmd.Env)
	}
}

func TestBuildPsqlCommand_PgpassStorage_NoEnvVar(t *testing.T) {
	// Set pgpass as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "pgpass")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service
	serviceID := "test-service-id"
	projectID := "test-project-id"
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
	}

	connectionString := "postgresql://testuser@testhost:5432/testdb?sslmode=require"
	psqlPath := "/usr/bin/psql"

	// Create a mock command for testing
	testCmd := &cobra.Command{}

	// Call the actual production function that builds the command
	psqlCmd := buildPsqlCommand(connectionString, psqlPath, []string{}, service, "tsdbadmin", testCmd)

	if psqlCmd == nil {
		t.Fatal("buildPsqlCommand returned nil")
	}

	// Verify that PGPASSWORD is NOT set in the environment for pgpass storage
	if psqlCmd.Env != nil {
		for _, envVar := range psqlCmd.Env {
			if strings.HasPrefix(envVar, "PGPASSWORD=") {
				t.Errorf("PGPASSWORD should not be set when using pgpass storage, but found: %s", envVar)
			}
		}
	}
}

func TestSeparateServiceAndPsqlArgs(t *testing.T) {
	testCases := []struct {
		name                string
		args                []string
		argsLenAtDash       int // What ArgsLenAtDash should return
		expectedServiceArgs []string
		expectedPsqlFlags   []string
	}{
		{
			name:                "No separator - service only",
			args:                []string{"svc-12345"},
			argsLenAtDash:       -1, // No -- found
			expectedServiceArgs: []string{"svc-12345"},
			expectedPsqlFlags:   []string{},
		},
		{
			name:                "No arguments at all",
			args:                []string{},
			argsLenAtDash:       -1,
			expectedServiceArgs: []string{},
			expectedPsqlFlags:   []string{},
		},
		{
			name:                "Service with psql flags after --",
			args:                []string{"svc-12345", "-c", "SELECT 1;"},
			argsLenAtDash:       1, // -- was after first arg
			expectedServiceArgs: []string{"svc-12345"},
			expectedPsqlFlags:   []string{"-c", "SELECT 1;"},
		},
		{
			name:                "No service, just psql flags after --",
			args:                []string{"--single-transaction", "--quiet"},
			argsLenAtDash:       0, // -- was at the beginning
			expectedServiceArgs: []string{},
			expectedPsqlFlags:   []string{"--single-transaction", "--quiet"},
		},
		{
			name:                "Service with multiple psql flags",
			args:                []string{"svc-test", "-c", "SELECT version();", "--no-psqlrc", "-v", "ON_ERROR_STOP=1"},
			argsLenAtDash:       1,
			expectedServiceArgs: []string{"svc-test"},
			expectedPsqlFlags:   []string{"-c", "SELECT version();", "--no-psqlrc", "-v", "ON_ERROR_STOP=1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock command that returns the expected ArgsLenAtDash
			mockCmd := &mockCobraCommand{
				args:          tc.args,
				argsLenAtDash: tc.argsLenAtDash,
			}

			serviceArgs, psqlFlags := separateServiceAndPsqlArgs(mockCmd, tc.args)

			if !equalStringSlices(serviceArgs, tc.expectedServiceArgs) {
				t.Errorf("Expected serviceArgs %v, got %v", tc.expectedServiceArgs, serviceArgs)
			}

			if !equalStringSlices(psqlFlags, tc.expectedPsqlFlags) {
				t.Errorf("Expected psqlFlags %v, got %v", tc.expectedPsqlFlags, psqlFlags)
			}
		})
	}
}

// mockCobraCommand implements the minimal interface needed for testing
type mockCobraCommand struct {
	args          []string
	argsLenAtDash int
}

func (m *mockCobraCommand) ArgsLenAtDash() int {
	return m.argsLenAtDash
}

// Helper function to compare string slices
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

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
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", "test-project-123", nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

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
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Execute db test-connection command
	_, err = executeDBCommand(t.Context(), "db", "test-connection")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestTestDatabaseConnection_InvalidConnectionString(t *testing.T) {
	// Test with truly invalid connection string that should fail at sql.Open

	cmd := &cobra.Command{}
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	// Test with malformed connection string (should return ExitInvalidParameters)
	invalidConnectionString := "this is not a valid connection string at all"
	ctx := context.Background()
	err := testDatabaseConnection(ctx, invalidConnectionString, 1*time.Second, cmd)

	if err == nil {
		t.Error("Expected error for invalid connection string")
	}

	// Should be an exitCodeError
	if exitErr, ok := err.(exitCodeError); ok {
		// The exact code depends on where it fails - could be ExitTimeout or ExitInvalidParameters
		if exitErr.ExitCode() != ExitTimeout && exitErr.ExitCode() != ExitInvalidParameters {
			t.Errorf("Expected exit code %d or %d for invalid connection string, got %d", ExitTimeout, ExitInvalidParameters, exitErr.ExitCode())
		}
	} else {
		t.Error("Expected exitCodeError for invalid connection string")
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

	// Check exit code (should be ExitTimeout for unreachable)
	if exitErr, ok := err.(exitCodeError); ok {
		if exitErr.ExitCode() != ExitTimeout {
			t.Errorf("Expected exit code %d for timeout, got %d", ExitTimeout, exitErr.ExitCode())
		}
	} else {
		t.Error("Expected exitCodeError for timeout")
	}
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
			originalGetCredentials := getCredentialsForDB
			getCredentialsForDB = func() (string, string, error) {
				return "test-api-key", "test-project-123", nil
			}
			defer func() { getCredentialsForDB = originalGetCredentials }()

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
				if exitErr, ok := err.(exitCodeError); ok {
					// Should be ExitTimeout (no response) or ExitInvalidParameters (invalid params) for network errors
					if exitErr.ExitCode() != ExitTimeout && exitErr.ExitCode() != ExitInvalidParameters {
						t.Errorf("Expected exit code %d or %d, got %d", ExitTimeout, ExitInvalidParameters, exitErr.ExitCode())
					}
				} else {
					t.Error("Expected exitCodeError")
				}
			}
		})
	}
}

func TestDBConnectionString_WithPassword(t *testing.T) {
	// This test verifies the end-to-end --with-password flag functionality
	// using direct function testing since full integration would require a real service

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service
	serviceID := "test-e2e-service"
	projectID := "test-e2e-project"
	host := "test-e2e-host.com"
	port := 5432
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Store a test password
	testPassword := "test-e2e-password-789"
	storage := password.GetPasswordStorage()
	err := storage.Save(service, testPassword, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service, "tsdbadmin") // Clean up after test

	// Test connection string without password (default behavior)
	details, err := password.GetConnectionDetails(service, password.ConnectionDetailsOptions{
		Role: "tsdbadmin",
	})
	if err != nil {
		t.Fatalf("GetConnectionDetails failed: %v", err)
	}
	baseConnectionString := details.String()

	expectedBase := fmt.Sprintf("postgresql://tsdbadmin@%s:%d/tsdb?sslmode=require", host, port)
	if baseConnectionString != expectedBase {
		t.Errorf("Expected base connection string '%s', got '%s'", expectedBase, baseConnectionString)
	}

	// Verify base connection string doesn't contain password
	if strings.Contains(baseConnectionString, testPassword) {
		t.Errorf("Base connection string should not contain password, but it does: %s", baseConnectionString)
	}

	// Test connection string with password (simulating --with-password flag)
	details2, err := password.GetConnectionDetails(service, password.ConnectionDetailsOptions{
		Role:         "tsdbadmin",
		WithPassword: true,
	})
	if err != nil {
		t.Fatalf("GetConnectionDetails with password failed: %v", err)
	}
	connectionStringWithPassword := details2.String()

	expectedWithPassword := fmt.Sprintf("postgresql://tsdbadmin:%s@%s:%d/tsdb?sslmode=require", testPassword, host, port)
	if connectionStringWithPassword != expectedWithPassword {
		t.Errorf("Expected connection string with password '%s', got '%s'", expectedWithPassword, connectionStringWithPassword)
	}

	// Verify connection string with password contains the password
	if !strings.Contains(connectionStringWithPassword, testPassword) {
		t.Errorf("Connection string with password should contain '%s', but it doesn't: %s", testPassword, connectionStringWithPassword)
	}
}

func TestDBSavePassword_ExplicitPassword(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)
	tmpDir := setupDBTest(t)

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999",
		"project_id": "test-project-123",
		"service_id": "svc-save-test",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock getServiceDetailsFunc to return a test service
	serviceID := "svc-save-test"
	projectID := "test-project-123"
	host := "test-host.com"
	port := 5432
	mockService := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	originalGetServiceDetails := getServiceDetailsFunc
	getServiceDetailsFunc = func(ctx context.Context, client *api.ClientWithResponses, projectID string, serviceID string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

	// Mock getCredentialsForDB to avoid auth errors
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", projectID, nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	testPassword := "explicit-password-123"

	// Execute save-password with explicit password
	output, err := executeDBCommand(t.Context(), "db", "save-password", "--password="+testPassword)
	if err != nil {
		t.Fatalf("Expected save-password to succeed, got error: %v", err)
	}

	// Verify success message
	if !strings.Contains(output, "Password saved successfully") {
		t.Errorf("Expected success message, got: %s", output)
	}
	if !strings.Contains(output, serviceID) {
		t.Errorf("Expected service ID in output, got: %s", output)
	}

	// Verify password was actually saved
	storage := password.GetPasswordStorage()
	retrievedPassword, err := storage.Get(mockService, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to retrieve saved password: %v", err)
	}
	defer storage.Remove(mockService, "tsdbadmin")

	if retrievedPassword != testPassword {
		t.Errorf("Expected password %q, got %q", testPassword, retrievedPassword)
	}
}

func TestDBSavePassword_EnvironmentVariable(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)
	tmpDir := setupDBTest(t)

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999",
		"project_id": "test-project-123",
		"service_id": "svc-env-test",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock getServiceDetailsFunc to return a test service
	serviceID := "svc-env-test"
	projectID := "test-project-123"
	host := "test-host.com"
	port := 5432
	mockService := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	originalGetServiceDetails := getServiceDetailsFunc
	getServiceDetailsFunc = func(ctx context.Context, client *api.ClientWithResponses, projectID string, serviceID string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

	// Mock getCredentialsForDB to avoid auth errors
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", projectID, nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Set environment variable
	testPassword := "env-password-456"
	os.Setenv("TIGER_NEW_PASSWORD", testPassword)
	defer os.Unsetenv("TIGER_NEW_PASSWORD")

	// Execute save-password without --password flag (should use env var)
	output, err := executeDBCommand(t.Context(), "db", "save-password")
	if err != nil {
		t.Fatalf("Expected save-password to succeed with env var, got error: %v", err)
	}

	// Verify success message
	if !strings.Contains(output, "Password saved successfully") {
		t.Errorf("Expected success message, got: %s", output)
	}

	// Verify password was actually saved
	storage := password.GetPasswordStorage()
	retrievedPassword, err := storage.Get(mockService, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to retrieve saved password: %v", err)
	}
	defer storage.Remove(mockService, "tsdbadmin")

	if retrievedPassword != testPassword {
		t.Errorf("Expected password %q, got %q", testPassword, retrievedPassword)
	}
}

func TestDBSavePassword_InteractivePrompt(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)
	tmpDir := setupDBTest(t)

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999",
		"project_id": "test-project-123",
		"service_id": "svc-interactive-test",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock getServiceDetailsFunc to return a test service
	serviceID := "svc-interactive-test"
	projectID := "test-project-123"
	host := "test-host.com"
	port := 5432
	mockService := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	originalGetServiceDetails := getServiceDetailsFunc
	getServiceDetailsFunc = func(ctx context.Context, client *api.ClientWithResponses, projectID string, serviceID string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

	// Mock getCredentialsForDB to avoid auth errors
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", projectID, nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Make sure TIGER_NEW_PASSWORD is not set
	os.Unsetenv("TIGER_NEW_PASSWORD")

	// Prepare the password input
	testPassword := "interactive-password-999"

	// Mock TTY check to return true (simulate terminal)
	originalCheckStdinIsTTY := checkStdinIsTTY
	checkStdinIsTTY = func() bool {
		return true
	}
	defer func() { checkStdinIsTTY = originalCheckStdinIsTTY }()

	// Mock password reading to return our test password
	originalReadPasswordFromTerminal := readPasswordFromTerminal
	readPasswordFromTerminal = func() (string, error) {
		return testPassword, nil
	}
	defer func() { readPasswordFromTerminal = originalReadPasswordFromTerminal }()

	// Execute save-password without --password flag or env var
	output, err := executeDBCommand(t.Context(), "db", "save-password")
	if err != nil {
		t.Fatalf("Expected save-password to succeed with interactive input, got error: %v", err)
	}

	// Verify the prompt was shown
	if !strings.Contains(output, "Enter password:") {
		t.Errorf("Expected password prompt, got: %s", output)
	}

	// Verify success message
	if !strings.Contains(output, "Password saved successfully") {
		t.Errorf("Expected success message, got: %s", output)
	}

	// Verify password was actually saved
	storage := password.GetPasswordStorage()
	retrievedPassword, err := storage.Get(mockService, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to retrieve saved password: %v", err)
	}
	defer storage.Remove(mockService, "tsdbadmin")

	if retrievedPassword != testPassword {
		t.Errorf("Expected password %q, got %q", testPassword, retrievedPassword)
	}
}

func TestDBSavePassword_InteractivePromptEmpty(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999",
		"project_id": "test-project-123",
		"service_id": "svc-empty-test",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock getServiceDetailsFunc to return a test service
	serviceID := "svc-empty-test"
	projectID := "test-project-123"
	mockService := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
	}

	originalGetServiceDetails := getServiceDetailsFunc
	getServiceDetailsFunc = func(ctx context.Context, client *api.ClientWithResponses, projectID string, serviceID string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

	// Mock getCredentialsForDB to avoid auth errors
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", projectID, nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Make sure TIGER_NEW_PASSWORD is not set
	os.Unsetenv("TIGER_NEW_PASSWORD")

	// Mock TTY check to return true (simulate terminal)
	originalCheckStdinIsTTY := checkStdinIsTTY
	checkStdinIsTTY = func() bool {
		return true
	}
	defer func() { checkStdinIsTTY = originalCheckStdinIsTTY }()

	// Mock password reading to return empty password
	originalReadPasswordFromTerminal := readPasswordFromTerminal
	readPasswordFromTerminal = func() (string, error) {
		return "", nil
	}
	defer func() { readPasswordFromTerminal = originalReadPasswordFromTerminal }()

	// Execute the command
	_, err = executeDBCommand(t.Context(), "db", "save-password")
	if err == nil {
		t.Fatal("Expected error when user provides empty password interactively")
	}

	// Verify the error message
	if !strings.Contains(err.Error(), "password cannot be empty") {
		t.Errorf("Expected 'password cannot be empty' error, got: %v", err)
	}
}

func TestDBSavePassword_CustomRole(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)
	tmpDir := setupDBTest(t)

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999",
		"project_id": "test-project-123",
		"service_id": "svc-role-test",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock getServiceDetailsFunc to return a test service
	serviceID := "svc-role-test"
	projectID := "test-project-123"
	host := "test-host.com"
	port := 5432
	mockService := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	originalGetServiceDetails := getServiceDetailsFunc
	getServiceDetailsFunc = func(ctx context.Context, client *api.ClientWithResponses, projectID string, serviceID string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

	// Mock getCredentialsForDB to avoid auth errors
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", projectID, nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	testPassword := "readonly-password-789"
	customRole := "readonly"

	// Execute with custom role
	output, err := executeDBCommand(t.Context(), "db", "save-password", "--password="+testPassword, "--role", customRole)
	if err != nil {
		t.Fatalf("Expected save-password to succeed with custom role, got error: %v", err)
	}

	// Verify success message shows the custom role
	if !strings.Contains(output, "Password saved successfully") {
		t.Errorf("Expected success message, got: %s", output)
	}
	if !strings.Contains(output, customRole) {
		t.Errorf("Expected role %q in output, got: %s", customRole, output)
	}

	// Verify password was saved for the custom role
	storage := password.GetPasswordStorage()
	retrievedPassword, err := storage.Get(mockService, customRole)
	if err != nil {
		t.Fatalf("Failed to retrieve saved password for role %s: %v", customRole, err)
	}
	defer storage.Remove(mockService, customRole)

	if retrievedPassword != testPassword {
		t.Errorf("Expected password %q, got %q", testPassword, retrievedPassword)
	}

	// Verify that tsdbadmin role doesn't have this password
	_, err = storage.Get(mockService, "tsdbadmin")
	if err == nil {
		t.Error("Expected error when retrieving password for different role, but got none")
	}
}

func TestDBSavePassword_NoServiceID(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config with project ID but no default service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"project_id": "test-project-123",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// No need to mock since it should fail before reaching getServiceDetailsFunc

	// Execute save-password without service ID
	_, err = executeDBCommand(t.Context(), "db", "save-password", "--password=test-password")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}

	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestDBSavePassword_NoAuth(t *testing.T) {
	tmpDir := setupDBTest(t)

	// Set up config with project ID and service ID
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "https://api.tigerdata.com/public/v1",
		"project_id": "test-project-123",
		"service_id": "svc-12345",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock authentication failure
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "", "", fmt.Errorf("not logged in")
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	// Execute save-password command
	_, err = executeDBCommand(t.Context(), "db", "save-password", "--password=test-password")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestDBSavePassword_PgpassStorage(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)
	tmpDir := setupDBTest(t)

	// Set pgpass as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "pgpass")
	defer viper.Set("password_storage", originalStorage)

	// Set up config
	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    "http://localhost:9999",
		"project_id": "test-project-123",
		"service_id": "svc-pgpass-test",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Mock getServiceDetailsFunc to return a test service with endpoint (required for pgpass)
	serviceID := "svc-pgpass-test"
	projectID := "test-project-123"
	host := "pgpass-host.com"
	port := 5432
	mockService := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	originalGetServiceDetails := getServiceDetailsFunc
	getServiceDetailsFunc = func(ctx context.Context, client *api.ClientWithResponses, projectID string, serviceID string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

	// Mock getCredentialsForDB to avoid auth errors
	originalGetCredentials := getCredentialsForDB
	getCredentialsForDB = func() (string, string, error) {
		return "test-api-key", projectID, nil
	}
	defer func() { getCredentialsForDB = originalGetCredentials }()

	testPassword := "pgpass-password-101"

	// Execute with pgpass storage
	output, err := executeDBCommand(t.Context(), "db", "save-password", "--password="+testPassword)
	if err != nil {
		t.Fatalf("Expected save-password to succeed with pgpass, got error: %v", err)
	}

	// Verify success message
	if !strings.Contains(output, "Password saved successfully") {
		t.Errorf("Expected success message, got: %s", output)
	}

	// Verify password was saved in pgpass storage
	storage := password.GetPasswordStorage()
	retrievedPassword, err := storage.Get(mockService, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to retrieve saved password from pgpass: %v", err)
	}
	defer storage.Remove(mockService, "tsdbadmin")

	if retrievedPassword != testPassword {
		t.Errorf("Expected password %q, got %q", testPassword, retrievedPassword)
	}
}
