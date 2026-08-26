package common

import (
	"fmt"
	"github.com/zalando/go-keyring"
	"net/url"
	"strings"
	"testing"

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
			result, err := GetConnectionDetails(testConfig(""), tc.service, tc.opts)

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
	err := storage.Save(service, testPassword, role)
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service, role) // Clean up after test

	details, err := GetConnectionDetails(cfg, service, ConnectionDetailsOptions{
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
	err := storage.Save(service, testPassword, role)
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service, role) // Clean up after test

	details, err := GetConnectionDetails(cfg, service, ConnectionDetailsOptions{
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

	result, err := GetConnectionDetails(cfg, service, ConnectionDetailsOptions{
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

	result, err := GetConnectionDetails(cfg, service, ConnectionDetailsOptions{
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
	if err := storage.Save(service, testPassword, role); err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service, role)

	details, err := GetConnectionDetails(cfg, service, ConnectionDetailsOptions{
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

	_, err := GetConnectionDetails(cfg, service, ConnectionDetailsOptions{
		Role:         "tsdbadmin",
		WithPassword: true,
	})

	if err == nil {
		t.Fatal("Expected error for invalid service endpoint, but got none")
	}

	// Verify we get an endpoint error
	expectedError := "service endpoint not available"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error message to contain '%s', got: %v", expectedError, err)
	}
}

func TestGetConnectionDetailsFor(t *testing.T) {
	primaryHost := "primary.example.com"
	replicaHost := "replica.example.com"
	poolerHost := "replica-pooler.example.com"
	port := 5432
	poolerPort := 6432

	// credService supplies credentials only; endpoint selection is driven by
	// connService. WithPassword is off here, so credService is not exercised.
	primary := api.Service{
		ServiceID: "svc-primary",
		ProjectID: "proj-1",
		Endpoint: &api.Endpoint{
			Host: &primaryHost,
			Port: &port,
		},
	}

	t.Run("direct endpoint", func(t *testing.T) {
		conn := api.Service{
			ServiceID: "rep-1",
			Name:      "my-replica",
			Endpoint:  &api.Endpoint{Host: &replicaHost, Port: &port},
		}

		details, err := GetConnectionDetailsFor(testConfig(""), conn, primary, ConnectionDetailsOptions{Role: "tsdbadmin"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if details.Host != replicaHost {
			t.Errorf("expected replica host %q, got %q", replicaHost, details.Host)
		}
		if details.Port != port {
			t.Errorf("expected port %d, got %d", port, details.Port)
		}
		if details.IsPooler {
			t.Errorf("expected IsPooler false for direct endpoint")
		}
		if details.Database != "tsdb" {
			t.Errorf("expected database tsdb, got %q", details.Database)
		}
	})

	t.Run("pooled endpoint when available", func(t *testing.T) {
		conn := api.Service{
			ServiceID: "rep-1",
			Name:      "my-replica",
			Endpoint:  &api.Endpoint{Host: &replicaHost, Port: &port},
			ConnectionPooler: &api.ConnectionPooler{
				Endpoint: &api.Endpoint{Host: &poolerHost, Port: &poolerPort},
			},
		}

		details, err := GetConnectionDetailsFor(testConfig(""), conn, primary, ConnectionDetailsOptions{Role: "tsdbadmin", Pooled: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if details.Host != poolerHost || details.Port != poolerPort {
			t.Errorf("expected pooler endpoint %s:%d, got %s:%d", poolerHost, poolerPort, details.Host, details.Port)
		}
		if !details.IsPooler {
			t.Errorf("expected IsPooler true when pooler used")
		}
	})

	t.Run("falls back to direct when pooler requested but unavailable", func(t *testing.T) {
		conn := api.Service{
			ServiceID: "rep-1",
			Endpoint:  &api.Endpoint{Host: &replicaHost, Port: &port},
		}

		details, err := GetConnectionDetailsFor(testConfig(""), conn, primary, ConnectionDetailsOptions{Role: "tsdbadmin", Pooled: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if details.IsPooler {
			t.Errorf("expected IsPooler false when no pooler available")
		}
		if details.Host != replicaHost {
			t.Errorf("expected fallback to direct host %q, got %q", replicaHost, details.Host)
		}
	})

	t.Run("error when endpoint missing", func(t *testing.T) {
		conn := api.Service{ServiceID: "rep-1"}
		if _, err := GetConnectionDetailsFor(testConfig(""), conn, primary, ConnectionDetailsOptions{Role: "tsdbadmin"}); err == nil {
			t.Fatal("expected error for missing connection endpoint")
		}
	})
}
