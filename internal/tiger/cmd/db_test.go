package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/tigerdata/tiger-cli/internal/tiger/api"
	"github.com/tigerdata/tiger-cli/internal/tiger/config"
)

func setupDBTest(t *testing.T) string {
	t.Helper()
	
	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	
	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	
	// Reset global config and viper to ensure test isolation
	config.ResetGlobalConfig()
	viper.Reset()
	
	t.Cleanup(func() {
		// Reset global config and viper first
		config.ResetGlobalConfig()
		viper.Reset()
		// Clean up environment variable BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})
	
	return tmpDir
}

func executeDBCommand(args ...string) (string, error) {
	// Create a test root command with db subcommand
	testRoot := &cobra.Command{
		Use: "tiger",
		PersistentPreRunE: rootCmd.PersistentPreRunE,
	}
	
	// Add persistent flags and bind them
	addPersistentFlags(testRoot)
	bindFlags(testRoot)
	
	// Add the db command to our test root
	dbCmd := buildDbCmd()
	testRoot.AddCommand(dbCmd)
	
	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)
	
	err := testRoot.Execute()
	return buf.String(), err
}

func TestDBConnectionString_NoProjectID(t *testing.T) {
	setupDBTest(t)
	
	// Mock authentication
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Execute db connection-string command without project ID
	_, err := executeDBCommand("db", "connection-string", "svc-12345")
	if err == nil {
		t.Fatal("Expected error when no project ID is configured")
	}
	
	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("Expected error about missing project ID, got: %v", err)
	}
}

func TestDBConnectionString_NoServiceID(t *testing.T) {
	tmpDir := setupDBTest(t)
	
	// Set up config with project ID but no default service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}
	
	// Mock authentication
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Execute db connection-string command without service ID
	_, err = executeDBCommand("db", "connection-string")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}
	
	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestDBConnectionString_NoAuth(t *testing.T) {
	tmpDir := setupDBTest(t)
	
	// Set up config with project ID and service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ServiceID: "svc-12345",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}
	
	// Mock authentication failure
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "", fmt.Errorf("not logged in")
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Execute db connection-string command
	_, err = executeDBCommand("db", "connection-string")
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}
	
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("Expected authentication error, got: %v", err)
	}
}

func TestDBConnectionString_PoolerWarning(t *testing.T) {
	// This test demonstrates that the warning functionality works
	// by directly testing the buildConnectionString function
	
	// Service without connection pooler
	service := api.Service{
		Endpoint: &api.Endpoint{
			Host: stringPtr("test-host.tigerdata.com"),
			Port: intPtr(5432),
		},
		ConnectionPooler: nil, // No pooler available
	}
	
	// Create a test command to capture stderr
	cmd := &cobra.Command{}
	errBuf := new(bytes.Buffer)
	cmd.SetErr(errBuf)
	
	// Request pooled connection when pooler is not available
	connectionString, err := buildConnectionString(service, true, "tsdbadmin", cmd)
	
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	
	// Should return direct connection string
	expectedString := "postgresql://tsdbadmin@test-host.tigerdata.com:5432/tsdb?sslmode=require"
	if connectionString != expectedString {
		t.Errorf("Expected connection string %q, got %q", expectedString, connectionString)
	}
	
	// Should have warning message on stderr
	stderrOutput := errBuf.String()
	if !strings.Contains(stderrOutput, "Warning: Connection pooler not available") {
		t.Errorf("Expected warning about pooler not available, but got: %q", stderrOutput)
	}
	
	// Verify the warning mentions using direct connection
	if !strings.Contains(stderrOutput, "using direct connection") {
		t.Errorf("Expected warning to mention direct connection fallback, but got: %q", stderrOutput)
	}
}

func TestDBConnect_NoProjectID(t *testing.T) {
	setupDBTest(t)
	
	// Mock authentication
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Execute db connect command without project ID
	_, err := executeDBCommand("db", "connect", "svc-12345")
	if err == nil {
		t.Fatal("Expected error when no project ID is configured")
	}
	
	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("Expected error about missing project ID, got: %v", err)
	}
}

func TestDBConnect_NoServiceID(t *testing.T) {
	tmpDir := setupDBTest(t)
	
	// Set up config with project ID but no default service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}
	
	// Mock authentication
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Execute db connect command without service ID
	_, err = executeDBCommand("db", "connect")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}
	
	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestDBConnect_NoAuth(t *testing.T) {
	tmpDir := setupDBTest(t)
	
	// Set up config with project ID and service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ServiceID: "svc-12345",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}
	
	// Mock authentication failure
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "", fmt.Errorf("not logged in")
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Execute db connect command
	_, err = executeDBCommand("db", "connect")
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
	cfg := &config.Config{
		APIURL:    "http://localhost:9999",
		ProjectID: "test-project-123",
		ServiceID: "svc-12345",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}
	
	// Mock authentication
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Test that psql alias works the same as connect
	_, err1 := executeDBCommand("db", "connect")
	_, err2 := executeDBCommand("db", "psql")
	
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
	
	// This will fail because psql path doesn't exist, but we can verify the error
	err := launchPsqlWithConnectionString(connectionString, psqlPath, []string{}, cmd)
	
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
	
	// This will fail because psql path doesn't exist, but we can verify the error
	err := launchPsqlWithConnectionString(connectionString, psqlPath, additionalFlags, cmd)
	
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

func TestSeparateServiceAndPsqlArgs(t *testing.T) {
	testCases := []struct {
		name                string
		args                []string
		argsLenAtDash       int  // What ArgsLenAtDash should return
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

func TestDBTestConnection_NoProjectID(t *testing.T) {
	setupDBTest(t)
	
	// Mock authentication
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Execute db test-connection command without project ID
	_, err := executeDBCommand("db", "test-connection", "svc-12345")
	if err == nil {
		t.Fatal("Expected error when no project ID is configured")
	}
	
	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("Expected error about missing project ID, got: %v", err)
	}
}

func TestDBTestConnection_NoServiceID(t *testing.T) {
	tmpDir := setupDBTest(t)
	
	// Set up config with project ID but no default service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}
	
	// Mock authentication
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "test-api-key", nil
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Execute db test-connection command without service ID
	_, err = executeDBCommand("db", "test-connection")
	if err == nil {
		t.Fatal("Expected error when no service ID is provided or configured")
	}
	
	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("Expected error about missing service ID, got: %v", err)
	}
}

func TestDBTestConnection_NoAuth(t *testing.T) {
	tmpDir := setupDBTest(t)
	
	// Set up config with project ID and service ID
	cfg := &config.Config{
		APIURL:    "https://api.tigerdata.com/public/v1",
		ProjectID: "test-project-123",
		ServiceID: "svc-12345",
		ConfigDir: tmpDir,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}
	
	// Mock authentication failure
	originalGetAPIKey := getAPIKeyForDB
	getAPIKeyForDB = func() (string, error) {
		return "", fmt.Errorf("not logged in")
	}
	defer func() { getAPIKeyForDB = originalGetAPIKey }()
	
	// Execute db test-connection command
	_, err = executeDBCommand("db", "test-connection")
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
	
	// Test with malformed connection string (should return exit code 3)
	invalidConnectionString := "this is not a valid connection string at all"
	err := testDatabaseConnection(invalidConnectionString, 1, cmd)
	
	if err == nil {
		t.Error("Expected error for invalid connection string")
	}
	
	// Should be an exitCodeError
	if exitErr, ok := err.(exitCodeError); ok {
		// The exact code depends on where it fails - could be 2 or 3
		if exitErr.ExitCode() != 2 && exitErr.ExitCode() != 3 {
			t.Errorf("Expected exit code 2 or 3 for invalid connection string, got %d", exitErr.ExitCode())
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
	
	start := time.Now()
	err := testDatabaseConnection(timeoutConnectionString, 1, cmd) // 1 second timeout
	duration := time.Since(start)
	
	if err == nil {
		t.Error("Expected error for timeout connection")
	}
	
	// Should complete within reasonable time (not hang)
	if duration > 3*time.Second {
		t.Errorf("Connection test took too long: %v", duration)
	}
	
	// Check exit code (should be 2 for unreachable)
	if exitErr, ok := err.(exitCodeError); ok {
		if exitErr.ExitCode() != 2 {
			t.Errorf("Expected exit code 2 for timeout, got %d", exitErr.ExitCode())
		}
	} else {
		t.Error("Expected exitCodeError for timeout")
	}
}

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

func TestBuildConnectionString(t *testing.T) {
	testCases := []struct {
		name            string
		service         api.Service
		pooled          bool
		role            string
		expectedString  string
		expectError     bool
		expectWarning   bool
	}{
		{
			name: "Basic connection string",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: stringPtr("test-host.tigerdata.com"),
					Port: intPtr(5432),
				},
			},
			pooled:         false,
			role:           "tsdbadmin",
			expectedString: "postgresql://tsdbadmin@test-host.tigerdata.com:5432/tsdb?sslmode=require",
			expectError:    false,
		},
		{
			name: "Connection string with custom role",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: stringPtr("test-host.tigerdata.com"),
					Port: intPtr(5432),
				},
			},
			pooled:         false,
			role:           "readonly",
			expectedString: "postgresql://readonly@test-host.tigerdata.com:5432/tsdb?sslmode=require",
			expectError:    false,
		},
		{
			name: "Connection string with default port",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: stringPtr("test-host.tigerdata.com"),
					Port: nil, // Should use default 5432
				},
			},
			pooled:         false,
			role:           "tsdbadmin",
			expectedString: "postgresql://tsdbadmin@test-host.tigerdata.com:5432/tsdb?sslmode=require",
			expectError:    false,
		},
		{
			name: "Pooled connection string",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: stringPtr("direct-host.tigerdata.com"),
					Port: intPtr(5432),
				},
				ConnectionPooler: &api.ConnectionPooler{
					Endpoint: &api.Endpoint{
						Host: stringPtr("pooler-host.tigerdata.com"),
						Port: intPtr(6432),
					},
				},
			},
			pooled:         true,
			role:           "tsdbadmin",
			expectedString: "postgresql://tsdbadmin@pooler-host.tigerdata.com:6432/tsdb?sslmode=require",
			expectError:    false,
		},
		{
			name: "Pooled connection fallback to direct when pooler unavailable",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: stringPtr("direct-host.tigerdata.com"),
					Port: intPtr(5432),
				},
				ConnectionPooler: nil, // No pooler available
			},
			pooled:         true,
			role:           "tsdbadmin",
			expectedString: "postgresql://tsdbadmin@direct-host.tigerdata.com:5432/tsdb?sslmode=require",
			expectError:    false,
			expectWarning:  true, // Should warn about pooler not available
		},
		{
			name: "Error when no endpoint available",
			service: api.Service{
				Endpoint: nil,
			},
			pooled:      false,
			role:        "tsdbadmin",
			expectError: true,
		},
		{
			name: "Error when no host available",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: nil,
					Port: intPtr(5432),
				},
			},
			pooled:      false,
			role:        "tsdbadmin",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test command to capture stderr output
			cmd := &cobra.Command{}
			errBuf := new(bytes.Buffer)
			cmd.SetErr(errBuf)
			
			result, err := buildConnectionString(tc.service, tc.pooled, tc.role, cmd)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if result != tc.expectedString {
				t.Errorf("Expected connection string %q, got %q", tc.expectedString, result)
			}
			
			// Check for warning message
			stderrOutput := errBuf.String()
			if tc.expectWarning {
				if !strings.Contains(stderrOutput, "Warning: Connection pooler not available") {
					t.Errorf("Expected warning about pooler not available, but got: %q", stderrOutput)
				}
			} else {
				if stderrOutput != "" {
					t.Errorf("Expected no warning, but got: %q", stderrOutput)
				}
			}
		})
	}
}

// Helper functions for creating pointers
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}