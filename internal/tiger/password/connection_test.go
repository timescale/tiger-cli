package password

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

func TestBuildConnectionString_Basic(t *testing.T) {
	testCases := []struct {
		name           string
		service        api.Service
		opts           ConnectionDetailsOptions
		expectedString string
		expectError    bool
		expectWarning  bool
	}{
		{
			name: "Basic connection string without password",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: util.Ptr("test-host.tigerdata.com"),
					Port: util.Ptr(5432),
				},
			},
			opts: ConnectionDetailsOptions{
				Pooled:       false,
				Role:         "tsdbadmin",
				PasswordMode: PasswordExclude,
			},
			expectedString: "postgresql://tsdbadmin@test-host.tigerdata.com:5432/tsdb?sslmode=require",
			expectError:    false,
		},
		{
			name: "Connection string with custom role",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: util.Ptr("test-host.tigerdata.com"),
					Port: util.Ptr(5432),
				},
			},
			opts: ConnectionDetailsOptions{
				Pooled:       false,
				Role:         "readonly",
				PasswordMode: PasswordExclude,
			},
			expectedString: "postgresql://readonly@test-host.tigerdata.com:5432/tsdb?sslmode=require",
			expectError:    false,
		},
		{
			name: "Connection string with default port",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: util.Ptr("test-host.tigerdata.com"),
					Port: nil, // Should use default 5432
				},
			},
			opts: ConnectionDetailsOptions{
				Pooled:       false,
				Role:         "tsdbadmin",
				PasswordMode: PasswordExclude,
			},
			expectedString: "postgresql://tsdbadmin@test-host.tigerdata.com:5432/tsdb?sslmode=require",
			expectError:    false,
		},
		{
			name: "Pooled connection string",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: util.Ptr("direct-host.tigerdata.com"),
					Port: util.Ptr(5432),
				},
				ConnectionPooler: &api.ConnectionPooler{
					Endpoint: &api.Endpoint{
						Host: util.Ptr("pooler-host.tigerdata.com"),
						Port: util.Ptr(6432),
					},
				},
			},
			opts: ConnectionDetailsOptions{
				Pooled:       true,
				Role:         "tsdbadmin",
				PasswordMode: PasswordExclude,
			},
			expectedString: "postgresql://tsdbadmin@pooler-host.tigerdata.com:6432/tsdb?sslmode=require",
			expectError:    false,
		},
		{
			name: "Pooled connection fallback to direct when pooler unavailable",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: util.Ptr("direct-host.tigerdata.com"),
					Port: util.Ptr(5432),
				},
				ConnectionPooler: nil, // No pooler available
			},
			opts: ConnectionDetailsOptions{
				Pooled:       true,
				Role:         "tsdbadmin",
				PasswordMode: PasswordExclude,
				WarnWriter:   new(bytes.Buffer), // Enable warnings
			},
			expectedString: "postgresql://tsdbadmin@direct-host.tigerdata.com:5432/tsdb?sslmode=require",
			expectError:    false,
			expectWarning:  true, // Should warn about pooler not available
		},
		{
			name: "Error when no endpoint available",
			service: api.Service{
				Endpoint: nil,
			},
			opts: ConnectionDetailsOptions{
				Pooled:       false,
				Role:         "tsdbadmin",
				PasswordMode: PasswordExclude,
			},
			expectError: true,
		},
		{
			name: "Error when no host available",
			service: api.Service{
				Endpoint: &api.Endpoint{
					Host: nil,
					Port: util.Ptr(5432),
				},
			},
			opts: ConnectionDetailsOptions{
				Pooled:       false,
				Role:         "tsdbadmin",
				PasswordMode: PasswordExclude,
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// If expecting a warning, create a buffer for WarnWriter
			var warnBuf *bytes.Buffer
			if tc.expectWarning && tc.opts.WarnWriter == nil {
				warnBuf = new(bytes.Buffer)
				tc.opts.WarnWriter = warnBuf
			} else if !tc.expectWarning && tc.opts.WarnWriter != nil {
				warnBuf = tc.opts.WarnWriter.(*bytes.Buffer)
			}

			result, err := BuildConnectionString(tc.service, tc.opts)

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
			if warnBuf != nil {
				stderrOutput := warnBuf.String()
				if tc.expectWarning {
					if !strings.Contains(stderrOutput, "Warning: Connection pooler not available") {
						t.Errorf("Expected warning about pooler not available, but got: %q", stderrOutput)
					}
				} else {
					if stderrOutput != "" {
						t.Errorf("Expected no warning, but got: %q", stderrOutput)
					}
				}
			}
		})
	}
}

func TestBuildConnectionString_WithPassword_KeyringStorage(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service
	serviceID := "test-password-service"
	projectID := "test-password-project"
	host := "test-host.com"
	port := 5432
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Store a test password in keyring
	testPassword := "test-password-keyring-123"
	storage := GetPasswordStorage()
	err := storage.Save(service, testPassword)
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service) // Clean up after test

	// Call BuildConnectionString with withPassword=true
	result, err := BuildConnectionString(service, ConnectionDetailsOptions{
		Pooled:       false,
		Role:         "tsdbadmin",
		PasswordMode: PasswordRequired,
	})

	if err != nil {
		t.Fatalf("BuildConnectionString failed: %v", err)
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
	// Set pgpass as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "pgpass")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service with endpoint information (required for pgpass)
	serviceID := "test-pgpass-service"
	projectID := "test-pgpass-project"
	host := "test-pgpass-host.com"
	port := 5432
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Store a test password in pgpass
	testPassword := "test-password-pgpass-456"
	storage := GetPasswordStorage()
	err := storage.Save(service, testPassword)
	if err != nil {
		t.Fatalf("Failed to save test password: %v", err)
	}
	defer storage.Remove(service) // Clean up after test

	// Call BuildConnectionString with withPassword=true
	result, err := BuildConnectionString(service, ConnectionDetailsOptions{
		Pooled:       false,
		Role:         "tsdbadmin",
		PasswordMode: PasswordRequired,
	})

	if err != nil {
		t.Fatalf("BuildConnectionString failed: %v", err)
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

func TestBuildConnectionString_WithPassword_NoStorage(t *testing.T) {
	// Set no storage as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "none")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service
	serviceID := "test-nostorage-service"
	projectID := "test-nostorage-project"
	host := "test-host.com"
	port := 5432
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Call BuildConnectionString with withPassword=true - should fail
	_, err := BuildConnectionString(service, ConnectionDetailsOptions{
		Pooled:       false,
		Role:         "tsdbadmin",
		PasswordMode: PasswordRequired,
	})

	if err == nil {
		t.Fatal("Expected error when password storage is disabled, but got none")
	}

	// Verify we get the expected error message
	expectedError := "password storage is disabled (--password-storage=none)"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error message to contain '%s', got: %v", expectedError, err)
	}
}

func TestBuildConnectionString_WithPassword_NoPasswordAvailable(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service (but don't store any password for it)
	serviceID := "test-nopassword-service"
	projectID := "test-nopassword-project"
	host := "test-host.com"
	port := 5432
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint: &api.Endpoint{
			Host: &host,
			Port: &port,
		},
	}

	// Call BuildConnectionString with withPassword=true - should fail
	_, err := BuildConnectionString(service, ConnectionDetailsOptions{
		Pooled:       false,
		Role:         "tsdbadmin",
		PasswordMode: PasswordRequired,
	})

	if err == nil {
		t.Fatal("Expected error when no password is available, but got none")
	}

	// Verify we get the expected error message
	expectedError := "no password found in keyring for this service"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error message to contain '%s', got: %v", expectedError, err)
	}
}

func TestBuildConnectionString_WithPassword_InvalidServiceEndpoint(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Set keyring as the password storage method for this test
	originalStorage := viper.GetString("password_storage")
	viper.Set("password_storage", "keyring")
	defer viper.Set("password_storage", originalStorage)

	// Create a test service without endpoint (invalid)
	serviceID := "test-invalid-service"
	projectID := "test-invalid-project"
	service := api.Service{
		ServiceId: &serviceID,
		ProjectId: &projectID,
		Endpoint:  nil, // Invalid - no endpoint
	}

	// Call BuildConnectionString with withPassword=true - should fail
	_, err := BuildConnectionString(service, ConnectionDetailsOptions{
		Pooled:       false,
		Role:         "tsdbadmin",
		PasswordMode: PasswordRequired,
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

func TestBuildConnectionString_PoolerWarning(t *testing.T) {
	// Service without connection pooler
	service := api.Service{
		Endpoint: &api.Endpoint{
			Host: util.Ptr("test-host.tigerdata.com"),
			Port: util.Ptr(5432),
		},
		ConnectionPooler: nil, // No pooler available
	}

	// Create a buffer to capture warnings
	warnBuf := new(bytes.Buffer)

	// Request pooled connection when pooler is not available
	connectionString, err := BuildConnectionString(service, ConnectionDetailsOptions{
		Pooled:       true,
		Role:         "tsdbadmin",
		PasswordMode: PasswordExclude,
		WarnWriter:   warnBuf,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should return direct connection string
	expectedString := "postgresql://tsdbadmin@test-host.tigerdata.com:5432/tsdb?sslmode=require"
	if connectionString != expectedString {
		t.Errorf("Expected connection string %q, got %q", expectedString, connectionString)
	}

	// Should have warning message
	stderrOutput := warnBuf.String()
	if !strings.Contains(stderrOutput, "Warning: Connection pooler not available") {
		t.Errorf("Expected warning about pooler not available, but got: %q", stderrOutput)
	}

	// Verify the warning mentions using direct connection
	if !strings.Contains(stderrOutput, "using direct connection") {
		t.Errorf("Expected warning to mention direct connection fallback, but got: %q", stderrOutput)
	}
}
