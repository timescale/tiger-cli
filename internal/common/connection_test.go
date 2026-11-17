package common

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/timescale/tiger-cli/internal/api"
)

func TestBuildConnectionString_Basic(t *testing.T) {
	testCases := []struct {
		name             string
		service          api.Service
		opts             ConnectionDetailsOptions
		expectedString   string
		expectedIsPooler bool
		expectError      bool
	}{
		{
			name: "Basic connection string without password",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: new("test-host.tigerdata.com"),
					Port: new(5432),
				},
			},
			opts: ConnectionDetailsOptions{
				Role: "tsdbadmin",
			},
			expectedString: "postgresql://tsdbadmin@test-host.tigerdata.com:5432/tsdb?sslmode=require",
		},
		{
			name: "Connection string with custom role",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: new("test-host.tigerdata.com"),
					Port: new(5432),
				},
			},
			opts: ConnectionDetailsOptions{
				Role: "readonly",
			},
			expectedString: "postgresql://readonly@test-host.tigerdata.com:5432/tsdb?sslmode=require",
		},
		{
			name: "Direct connection when pooler is available",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: new("direct-host.tigerdata.com"),
					Port: new(5432),
				},
				ConnectionPooler: &api.ConnectionPooler{
					Endpoint: &api.Endpoint{
						Host: new("pooler-host.tigerdata.com"),
						Port: new(6432),
					},
				},
			},
			opts: ConnectionDetailsOptions{
				Role: "tsdbadmin",
			},
			expectedString: "postgresql://tsdbadmin@direct-host.tigerdata.com:5432/tsdb?sslmode=require",
		},
		{
			name: "Pooled connection string",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: new("direct-host.tigerdata.com"),
					Port: new(5432),
				},
				ConnectionPooler: &api.ConnectionPooler{
					Endpoint: &api.Endpoint{
						Host: new("pooler-host.tigerdata.com"),
						Port: new(6432),
					},
				},
			},
			opts: ConnectionDetailsOptions{
				Pooled: true,
				Role:   "tsdbadmin",
			},
			expectedString:   "postgresql://tsdbadmin@pooler-host.tigerdata.com:6432/tsdb?sslmode=require",
			expectedIsPooler: true,
		},
		{
			name: "Read-only injects tsdb_admin.read_only_connection GUC",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: new("test-host.tigerdata.com"),
					Port: new(5432),
				},
			},
			opts: ConnectionDetailsOptions{
				Role:     "tsdbadmin",
				ReadOnly: true,
			},
			expectedString: "postgresql://tsdbadmin@test-host.tigerdata.com:5432/tsdb?sslmode=require&options=-c%20tsdb_admin.read_only_connection%3Dtrue",
		},
		{
			name: "Pooled connection fallback to direct when pooler unavailable",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: new("direct-host.tigerdata.com"),
					Port: new(5432),
				},
				ConnectionPooler: nil, // No pooler available
			},
			opts: ConnectionDetailsOptions{
				Pooled: true,
				Role:   "tsdbadmin",
			},
			expectedString: "postgresql://tsdbadmin@direct-host.tigerdata.com:5432/tsdb?sslmode=require",
		},
		{
			name: "Error when no endpoint available",
			service: api.Service{
				Endpoint: nil,
			},
			opts: ConnectionDetailsOptions{
				Role: "tsdbadmin",
			},
			expectError: true,
		},
		{
			name: "Error when no host available",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: nil,
					Port: new(5432),
				},
			},
			opts: ConnectionDetailsOptions{
				Role: "tsdbadmin",
			},
			expectError: true,
		},
		{
			name: "Error when host is empty",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: new(""),
					Port: new(5432),
				},
			},
			opts: ConnectionDetailsOptions{
				Role: "tsdbadmin",
			},
			expectError: true,
		},
		{
			name: "Error when no port available",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: new("test-host.tigerdata.com"),
					Port: nil,
				},
			},
			opts: ConnectionDetailsOptions{
				Role: "tsdbadmin",
			},
			expectError: true,
		},
		{
			name: "Error when port is zero",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: new("test-host.tigerdata.com"),
					Port: new(0),
				},
			},
			opts: ConnectionDetailsOptions{
				Role: "tsdbadmin",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := GetConnectionDetails(testConfig(""), NewServiceConnectionTarget(tc.service), tc.opts)

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

			if result.String() != tc.expectedString {
				t.Errorf("Expected connection string %q, got %q", tc.expectedString, result.String())
			}

			if result.IsPooler != tc.expectedIsPooler {
				t.Errorf("Expected IsPooler to be %v, got %v", tc.expectedIsPooler, result.IsPooler)
			}
		})
	}
}

func TestBuildConnectionString_WithPassword_KeyringStorage(t *testing.T) {
	// Give the test a fresh, empty in-memory keyring
	keyring.MockInit()

	cfg := testConfig("keyring")

	// Create a test service
	serviceID := "test-password-service"
	projectID := "test-password-project"
	host := "test-host.com"
	port := 5432
	service := api.Service{
		ServiceID: serviceID,
		ProjectID: projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Store a test password in keyring
	testPassword := "test-password-keyring-123"
	role := "tsdbadmin"
	storage := GetPasswordStorage(cfg)
	err := storage.Save(ServicePasswordTarget(service), testPassword, role)
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(ServicePasswordTarget(service), role) // Clean up after test

	details, err := GetConnectionDetails(cfg, NewServiceConnectionTarget(service), ConnectionDetailsOptions{
		Role:         "tsdbadmin",
		WithPassword: true,
	})
	result := details.String()

	if err != nil {
		t.Fatalf("GetConnectionDetails failed: %v", err)
	}

	// Verify that the password is included in the result
	expectedResult := fmt.Sprintf("postgresql://tsdbadmin:%s@%s:%d/tsdb?sslmode=require", testPassword, host, port)
	if result != expectedResult {
		t.Errorf("Expected connection string with password '%s', got '%s'", expectedResult, result)
	}

	// Verify the password is actually in the connection string
	if !strings.Contains(result, testPassword) {
		t.Errorf("Password '%s' not found in connection string: %s", testPassword, result)
	}
}

func TestBuildConnectionString_WithPassword_PgpassStorage(t *testing.T) {
	cfg := testConfig("pgpass")

	// Create a test service with endpoint information (required for pgpass)
	serviceID := "test-pgpass-service"
	projectID := "test-pgpass-project"
	host := "test-pgpass-host.com"
	port := 5432
	service := api.Service{
		ServiceID: serviceID,
		ProjectID: projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Store a test password in pgpass
	testPassword := "test-password-pgpass-456"
	role := "tsdbadmin"
	storage := GetPasswordStorage(cfg)
	err := storage.Save(ServicePasswordTarget(service), testPassword, role)
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(ServicePasswordTarget(service), role) // Clean up after test

	details, err := GetConnectionDetails(cfg, NewServiceConnectionTarget(service), ConnectionDetailsOptions{
		Role:         "tsdbadmin",
		WithPassword: true,
	})
	result := details.String()

	if err != nil {
		t.Fatalf("GetConnectionDetails failed: %v", err)
	}

	// Verify that the password is included in the result
	expectedResult := fmt.Sprintf("postgresql://tsdbadmin:%s@%s:%d/tsdb?sslmode=require", testPassword, host, port)
	if result != expectedResult {
		t.Errorf("Expected connection string with password '%s', got '%s'", expectedResult, result)
	}

	// Verify the password is actually in the connection string
	if !strings.Contains(result, testPassword) {
		t.Errorf("Password '%s' not found in connection string: %s", testPassword, result)
	}
}

// A password with URL-special characters must still produce a parseable URL.
func TestConnectionDetailsString_EncodesSpecialCharPassword(t *testing.T) {
	details := &ConnectionDetails{
		Role:     "tsdbadmin",
		Password: "p@ss/w:rd? #[x]",
		Host:     "host.example.com",
		Port:     5432,
		Database: "tsdb",
	}

	s := details.String()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("connection string should be a parseable URL, got error: %v (%q)", err, s)
	}
	if got := u.User.Username(); got != details.Role {
		t.Errorf("role did not round-trip: got %q, want %q", got, details.Role)
	}
	if pw, _ := u.User.Password(); pw != details.Password {
		t.Errorf("password did not round-trip: got %q, want %q", pw, details.Password)
	}
}

// TestConnectionDetailsConnConfig is a helper-level test because the behavior
// it protects can't be reached from a command or tool. The per-address bound
// only matters when one of a host's addresses drops packets, and a test can't
// produce that without a real network. pgx itself applies ConnectTimeout to
// each address separately, so the parsed config is the part we own and the
// part this test checks. The cases set or unset PGCONNECT_TIMEOUT explicitly,
// so the developer's environment doesn't leak in.
func TestConnectionDetailsConnConfig(t *testing.T) {
	details := &ConnectionDetails{
		Role:     "tsdbadmin",
		Host:     "host.example.com",
		Port:     5432,
		Database: "tsdb",
	}

	tests := []struct {
		name        string
		env         *string // nil leaves PGCONNECT_TIMEOUT unset
		wantTimeout time.Duration
	}{
		{
			name:        "defaults to ConnectTimeout",
			wantTimeout: ConnectTimeout,
		},
		{
			name:        "PGCONNECT_TIMEOUT wins",
			env:         new("12"),
			wantTimeout: 12 * time.Second,
		},
		{
			name:        "PGCONNECT_TIMEOUT=0 means no limit",
			env:         new("0"),
			wantTimeout: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ConnectTimeoutEnvVar, "") // restores the original value afterwards
			if tt.env != nil {
				t.Setenv(ConnectTimeoutEnvVar, *tt.env)
			} else if err := os.Unsetenv(ConnectTimeoutEnvVar); err != nil {
				t.Fatalf("failed to unset %s: %v", ConnectTimeoutEnvVar, err)
			}

			connConfig, err := details.ConnConfig()
			if err != nil {
				t.Fatalf("ConnConfig() error = %v", err)
			}
			if connConfig.ConnectTimeout != tt.wantTimeout {
				t.Errorf("ConnectTimeout = %v, want %v", connConfig.ConnectTimeout, tt.wantTimeout)
			}
			if connConfig.Host != details.Host || connConfig.Port != uint16(details.Port) || connConfig.User != details.Role || connConfig.Database != details.Database {
				t.Errorf("config doesn't match details: host=%q port=%d user=%q database=%q", connConfig.Host, connConfig.Port, connConfig.User, connConfig.Database)
			}
		})
	}
}

func TestBuildConnectionString_WithPassword_NoStorage(t *testing.T) {
	// Set no storage as the password storage method for this test
	cfg := testConfig("none")

	// Create a test service
	serviceID := "test-nostorage-service"
	projectID := "test-nostorage-project"
	host := "test-host.com"
	port := 5432
	service := api.Service{
		ServiceID: serviceID,
		ProjectID: projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	result, err := GetConnectionDetails(cfg, NewServiceConnectionTarget(service), ConnectionDetailsOptions{
		Role:         "tsdbadmin",
		WithPassword: true,
	})
	if err != nil {
		t.Fatal("Expected no error when password storage is disabled, but got one")
	}

	if result.Password != "" {
		t.Errorf("Expected no password in connection details, but got: %s", result.Password)
	}

	expectedString := "postgresql://tsdbadmin@test-host.com:5432/tsdb?sslmode=require"
	if result.String() != expectedString {
		t.Errorf("Expected connection string %q, got %q", expectedString, result.String())
	}
}

func TestBuildConnectionString_WithPassword_NoPasswordAvailable(t *testing.T) {
	// Give the test a fresh, empty in-memory keyring
	keyring.MockInit()

	cfg := testConfig("keyring")

	// Create a test service (but don't store any password for it)
	serviceID := "test-nopassword-service"
	projectID := "test-nopassword-project"
	host := "test-host.com"
	port := 5432
	service := api.Service{
		ServiceID: serviceID,
		ProjectID: projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	result, err := GetConnectionDetails(cfg, NewServiceConnectionTarget(service), ConnectionDetailsOptions{
		Role:         "tsdbadmin",
		WithPassword: true,
	})
	if err != nil {
		t.Fatal("Expected no error when no password is available, but got one")
	}

	if result.Password != "" {
		t.Errorf("Expected no password in connection details, but got: %s", result.Password)
	}

	expectedString := "postgresql://tsdbadmin@test-host.com:5432/tsdb?sslmode=require"
	if result.String() != expectedString {
		t.Errorf("Expected connection string %q, got %q", expectedString, result.String())
	}
}

func TestBuildConnectionString_ReadOnly_WithPassword(t *testing.T) {
	keyring.MockInit()

	cfg := testConfig("keyring")

	serviceID := "test-readonly-service"
	projectID := "test-readonly-project"
	host := "test-host.com"
	port := 5432
	service := api.Service{
		ServiceID: serviceID,
		ProjectID: projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	testPassword := "test-password-readonly-789"
	role := "tsdbadmin"
	storage := GetPasswordStorage(cfg)
	if err := storage.Save(ServicePasswordTarget(service), testPassword, role); err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(ServicePasswordTarget(service), role)

	details, err := GetConnectionDetails(cfg, NewServiceConnectionTarget(service), ConnectionDetailsOptions{
		Role:         role,
		WithPassword: true,
		ReadOnly:     true,
	})
	if err != nil {
		t.Fatalf("GetConnectionDetails failed: %v", err)
	}

	expected := fmt.Sprintf(
		"postgresql://tsdbadmin:%s@%s:%d/tsdb?sslmode=require&options=-c%%20tsdb_admin.read_only_connection%%3Dtrue",
		testPassword, host, port,
	)
	if got := details.String(); got != expected {
		t.Errorf("Expected connection string %q, got %q", expected, got)
	}
}

func TestBuildConnectionString_WithPassword_InvalidServiceEndpoint(t *testing.T) {
	// Give the test a fresh, empty in-memory keyring
	keyring.MockInit()

	cfg := testConfig("keyring")

	// Create a test service without endpoint (invalid)
	serviceID := "test-invalid-service"
	projectID := "test-invalid-project"
	service := api.Service{
		ServiceID: serviceID,
		ProjectID: projectID,
		Endpoint:  nil, // Invalid - no endpoint
	}

	_, err := GetConnectionDetails(cfg, NewServiceConnectionTarget(service), ConnectionDetailsOptions{
		Role:         "tsdbadmin",
		WithPassword: true,
	})

	if err == nil {
		t.Fatal("Expected error for invalid service endpoint, but got none")
	}

	// Verify we get an endpoint error
	expectedError := "endpoint not available"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error message to contain '%s', got: %v", expectedError, err)
	}
}
