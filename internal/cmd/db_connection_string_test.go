package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

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
	mockTestPAT(t)

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
	mockNotLoggedIn(t)

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
	details, err := common.GetConnectionDetails(testConfig(t), service, common.ConnectionDetailsOptions{
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

func TestDBConnectionString_WithPassword(t *testing.T) {
	// This test verifies the end-to-end --with-password flag functionality
	// using direct function testing since full integration would require a real service

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Set keyring as the password storage method for this test
	t.Setenv("TIGER_PASSWORD_STORAGE", "keyring")

	// Create a test service
	serviceID := "test-e2e-service"
	projectID := "test-e2e-project"
	host := "test-e2e-host.com"
	port := 5432
	service := api.Service{
		ServiceID: &serviceID,
		ProjectID: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Store a test password
	testPassword := "test-e2e-password-789"
	storage := common.GetPasswordStorage(testConfig(t))
	err := storage.Save(service, testPassword, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service, "tsdbadmin") // Clean up after test

	// Test connection string without password (default behavior)
	details, err := common.GetConnectionDetails(testConfig(t), service, common.ConnectionDetailsOptions{
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
	details2, err := common.GetConnectionDetails(testConfig(t), service, common.ConnectionDetailsOptions{
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

// TestDBConnectionString_ReadOnlyConfig verifies that the global read_only
// config option forces the read-only GUC into the emitted connection string
// even when --read-only is not passed on the command line.
func TestDBConnectionString_ReadOnlyConfig(t *testing.T) {
	const readOnlyMarker = "tsdb_admin.read_only_connection"

	cases := []struct {
		name      string
		readOnly  bool
		extraArgs []string
		want      bool
	}{
		{"flag off, config off", false, nil, false},
		{"flag on, config off", false, []string{"--read-only"}, true},
		{"flag off, config on", true, nil, true},
		{"flag on, config on", true, []string{"--read-only"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := setupDBTest(t)

			_, err := config.UseTestConfig(tmpDir, map[string]any{
				"api_url":    "http://localhost:9999",
				"project_id": "test-project-123",
				"service_id": "svc-ro-test",
				"read_only":  tc.readOnly,
			})
			if err != nil {
				t.Fatalf("Failed to save test config: %v", err)
			}

			mockTestPAT(t)

			originalGetServiceDetails := getServiceDetailsFunc
			getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
				host := "test-host.com"
				port := 5432
				return api.Service{
					Endpoint: &api.Endpoint{Host: &host, Port: &port},
				}, nil
			}
			t.Cleanup(func() { getServiceDetailsFunc = originalGetServiceDetails })

			args := append([]string{"db", "connection-string"}, tc.extraArgs...)
			out, err := executeDBCommand(t.Context(), args...)
			if err != nil {
				t.Fatalf("executeDBCommand failed: %v", err)
			}

			got := strings.Contains(out, readOnlyMarker)
			if got != tc.want {
				t.Errorf("read-only marker present = %v, want %v\noutput: %s", got, tc.want, out)
			}
		})
	}
}
