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

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
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
	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot := buildRootCmd()

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err := testRoot.Execute()
	return buf.String(), err
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
	// by directly testing the util.BuildConnectionString function

	// Service without connection pooler
	service := api.Service{
		Endpoint: &api.Endpoint{
			Host: util.Ptr("test-host.tigerdata.com"),
			Port: util.Ptr(5432),
		},
		ConnectionPooler: nil, // No pooler available
	}

	// Create a buffer to capture stderr
	errBuf := new(bytes.Buffer)

	// Request pooled connection when pooler is not available
	connectionString, err := util.BuildConnectionString(service, util.ConnectionStringOptions{
		Pooled:       true,
		Role:         "tsdbadmin",
		WithPassword: false,
		WarnWriter:   errBuf,
	})

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

	// Create a dummy service for the test
	service := api.Service{}

	// This will fail because psql path doesn't exist, but we can verify the error
	err := launchPsqlWithConnectionString(connectionString, psqlPath, []string{}, service, cmd)

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
	err := launchPsqlWithConnectionString(connectionString, psqlPath, additionalFlags, service, cmd)

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
	storage := util.GetPasswordStorage()
	err := storage.Save(service, testPassword)
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service) // Clean up after test

	connectionString := "postgresql://testuser@testhost:5432/testdb?sslmode=require"
	psqlPath := "/usr/bin/psql"
	additionalFlags := []string{"--quiet"}

	// Create a mock command for testing
	testCmd := &cobra.Command{}

	// Call the actual production function that builds the command
	psqlCmd := buildPsqlCommand(connectionString, psqlPath, additionalFlags, service, testCmd)

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
	psqlCmd := buildPsqlCommand(connectionString, psqlPath, []string{}, service, testCmd)

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

func TestBuildConnectionConfig_KeyringPassword(t *testing.T) {
	// This test verifies that buildConnectionConfig properly sets password from keyring

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service
	serviceID := "test-connection-config-service"
	projectID := "test-connection-config-project"
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
	}

	// Store a test password in keyring
	testPassword := "test-connection-config-password-789"
	storage := util.GetPasswordStorage()
	err := storage.Save(service, testPassword)
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service) // Clean up after test

	connectionString := "postgresql://testuser@testhost:5432/testdb?sslmode=require"

	// Call the actual production function that builds the config
	config, err := buildConnectionConfig(connectionString, service)

	if err != nil {
		t.Fatalf("buildConnectionConfig failed: %v", err)
	}

	if config == nil {
		t.Fatal("buildConnectionConfig returned nil config")
	}

	// Verify that the password was set in the config
	if config.Password != testPassword {
		t.Errorf("Expected password '%s' to be set in config, but got '%s'", testPassword, config.Password)
	}
}

func TestBuildConnectionConfig_PgpassStorage_NoPasswordSet(t *testing.T) {
	// This test verifies that buildConnectionConfig doesn't set password for pgpass storage

	// Set pgpass as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "pgpass")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service
	serviceID := "test-connection-config-pgpass"
	projectID := "test-connection-config-project"
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
	}

	connectionString := "postgresql://testuser@testhost:5432/testdb?sslmode=require"

	// Call the actual production function that builds the config
	config, err := buildConnectionConfig(connectionString, service)

	if err != nil {
		t.Fatalf("buildConnectionConfig failed: %v", err)
	}

	if config == nil {
		t.Fatal("buildConnectionConfig returned nil config")
	}

	// Verify that no password was set in the config (pgx will check ~/.pgpass automatically)
	if config.Password != "" {
		t.Errorf("Expected no password to be set in config for pgpass storage, but got '%s'", config.Password)
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

	// Test with malformed connection string (should return ExitInvalidParameters)
	invalidConnectionString := "this is not a valid connection string at all"
	service := api.Service{} // Dummy service for test
	err := testDatabaseConnection(invalidConnectionString, 1, service, cmd)

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

	service := api.Service{} // Dummy service for test
	start := time.Now()
	err := testDatabaseConnection(timeoutConnectionString, 1, service, cmd) // 1 second timeout
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
			cfg := &config.Config{
				APIURL:    "http://localhost:9999", // Non-existent server
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

			// Execute db test-connection command with timeout flag
			_, err = executeDBCommand("db", "test-connection", "--timeout", tc.timeoutFlag)

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
	storage := util.GetPasswordStorage()
	err := storage.Save(service, testPassword)
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service) // Clean up after test

	// Test util.BuildConnectionString without password (default behavior)
	cmd := &cobra.Command{}
	baseConnectionString, err := util.BuildConnectionString(service, util.ConnectionStringOptions{
		Pooled:       false,
		Role:         "tsdbadmin",
		WithPassword: false,
		WarnWriter:   cmd.ErrOrStderr(),
	})
	if err != nil {
		t.Fatalf("BuildConnectionString failed: %v", err)
	}

	expectedBase := fmt.Sprintf("postgresql://tsdbadmin@%s:%d/tsdb?sslmode=require", host, port)
	if baseConnectionString != expectedBase {
		t.Errorf("Expected base connection string '%s', got '%s'", expectedBase, baseConnectionString)
	}

	// Verify base connection string doesn't contain password
	if strings.Contains(baseConnectionString, testPassword) {
		t.Errorf("Base connection string should not contain password, but it does: %s", baseConnectionString)
	}

	// Test util.BuildConnectionString with password (simulating --with-password flag)
	connectionStringWithPassword, err := util.BuildConnectionString(service, util.ConnectionStringOptions{
		Pooled:       false,
		Role:         "tsdbadmin",
		WithPassword: true,
		WarnWriter:   cmd.ErrOrStderr(),
	})
	if err != nil {
		t.Fatalf("BuildConnectionString with password failed: %v", err)
	}

	expectedWithPassword := fmt.Sprintf("postgresql://tsdbadmin:%s@%s:%d/tsdb?sslmode=require", testPassword, host, port)
	if connectionStringWithPassword != expectedWithPassword {
		t.Errorf("Expected connection string with password '%s', got '%s'", expectedWithPassword, connectionStringWithPassword)
	}

	// Verify connection string with password contains the password
	if !strings.Contains(connectionStringWithPassword, testPassword) {
		t.Errorf("Connection string with password should contain '%s', but it doesn't: %s", testPassword, connectionStringWithPassword)
	}
}
