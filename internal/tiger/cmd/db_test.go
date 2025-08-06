package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

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
	// Reset global flag variables to ensure clean state for each test
	dbConnectionStringPooled = false
	dbConnectionStringRole = "tsdbadmin"
	
	// Create a test root command with db subcommand
	testRoot := &cobra.Command{
		Use: "tiger",
		PersistentPreRunE: rootCmd.PersistentPreRunE,
	}
	
	// Add persistent flags and bind them
	addPersistentFlags(testRoot)
	bindFlags(testRoot)
	
	// Add the db command to our test root
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