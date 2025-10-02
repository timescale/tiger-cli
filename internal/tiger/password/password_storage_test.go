package password

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// Helper function to create a test service
func createTestService(serviceID string) api.Service {
	projectID := "test-project-123"
	return api.Service{
		ProjectId: &projectID,
		ServiceId: &serviceID,
		Endpoint: &api.Endpoint{
			Host: util.Ptr("test-host.tigerdata.com"),
			Port: util.Ptr(5432),
		},
	}
}

func TestNoStorage_Save(t *testing.T) {
	storage := &NoStorage{}
	service := createTestService("test-service-123")

	err := storage.Save(service, "test-password")
	if err != nil {
		t.Errorf("NoStorage.Save() should never return an error, got: %v", err)
	}
}

func TestNoStorage_Get(t *testing.T) {
	storage := &NoStorage{}
	service := createTestService("test-service-123")

	password, err := storage.Get(service)
	if err == nil {
		t.Error("NoStorage.Get() should return an error")
	}
	if password != "" {
		t.Errorf("NoStorage.Get() should return empty password, got: %s", password)
	}
	if !strings.Contains(err.Error(), "password storage disabled") {
		t.Errorf("NoStorage.Get() error should mention storage disabled, got: %v", err)
	}
}

func TestNoStorage_Remove(t *testing.T) {
	storage := &NoStorage{}
	service := createTestService("test-service-123")

	err := storage.Remove(service)
	if err != nil {
		t.Errorf("NoStorage.Remove() should never return an error, got: %v", err)
	}
}

func TestKeyringStorage_Save_NoServiceId(t *testing.T) {
	storage := &KeyringStorage{}
	service := api.Service{} // No ServiceId

	err := storage.Save(service, "test-password")
	if err == nil {
		t.Error("KeyringStorage.Save() should return error when ServiceId is nil")
	}
	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("KeyringStorage.Save() should mention service ID required, got: %v", err)
	}
}

func TestKeyringStorage_Save_NoProjectId(t *testing.T) {
	storage := &KeyringStorage{}
	serviceID := "test-service-123"
	service := api.Service{
		ServiceId: &serviceID,
		// No ProjectId
	}

	err := storage.Save(service, "test-password")
	if err == nil {
		t.Error("KeyringStorage.Save() should return error when ProjectId is nil")
	}
	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("KeyringStorage.Save() should mention project ID required, got: %v", err)
	}
}

func TestKeyringStorage_Get_NoServiceId(t *testing.T) {
	storage := &KeyringStorage{}
	service := api.Service{} // No ServiceId

	password, err := storage.Get(service)
	if err == nil {
		t.Error("KeyringStorage.Get() should return error when ServiceId is nil")
	}
	if password != "" {
		t.Errorf("KeyringStorage.Get() should return empty password on error, got: %s", password)
	}
	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("KeyringStorage.Get() should mention service ID required, got: %v", err)
	}
}

func TestKeyringStorage_Get_NoProjectId(t *testing.T) {
	storage := &KeyringStorage{}
	serviceID := "test-service-123"
	service := api.Service{
		ServiceId: &serviceID,
		// No ProjectId
	}

	password, err := storage.Get(service)
	if err == nil {
		t.Error("KeyringStorage.Get() should return error when ProjectId is nil")
	}
	if password != "" {
		t.Errorf("KeyringStorage.Get() should return empty password on error, got: %s", password)
	}
	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("KeyringStorage.Get() should mention project ID required, got: %v", err)
	}
}

func TestKeyringStorage_Remove_NoServiceId(t *testing.T) {
	storage := &KeyringStorage{}
	service := api.Service{} // No ServiceId

	err := storage.Remove(service)
	if err == nil {
		t.Error("KeyringStorage.Remove() should return error when ServiceId is nil")
	}
	if !strings.Contains(err.Error(), "service ID is required") {
		t.Errorf("KeyringStorage.Remove() should mention service ID required, got: %v", err)
	}
}

func TestKeyringStorage_Remove_NoProjectId(t *testing.T) {
	storage := &KeyringStorage{}
	serviceID := "test-service-123"
	service := api.Service{
		ServiceId: &serviceID,
		// No ProjectId
	}

	err := storage.Remove(service)
	if err == nil {
		t.Error("KeyringStorage.Remove() should return error when ProjectId is nil")
	}
	if !strings.Contains(err.Error(), "project ID is required") {
		t.Errorf("KeyringStorage.Remove() should mention project ID required, got: %v", err)
	}
}

func TestPgpassStorage_Remove_NoEndpoint(t *testing.T) {
	storage := &PgpassStorage{}
	service := api.Service{
		ServiceId: util.Ptr("test-service-123"),
		// No Endpoint
	}

	err := storage.Remove(service)
	if err == nil {
		t.Error("PgpassStorage.Remove() should return error when endpoint is nil")
	}
	if !strings.Contains(err.Error(), "service endpoint not available") {
		t.Errorf("PgpassStorage.Remove() should mention endpoint not available, got: %v", err)
	}
}

func TestPgpassStorage_Get_NoEndpoint(t *testing.T) {
	storage := &PgpassStorage{}
	service := api.Service{
		ServiceId: util.Ptr("test-service-123"),
		// No Endpoint
	}

	password, err := storage.Get(service)
	if err == nil {
		t.Error("PgpassStorage.Get() should return error when endpoint is nil")
	}
	if password != "" {
		t.Errorf("PgpassStorage.Get() should return empty password on error, got: %s", password)
	}
	if !strings.Contains(err.Error(), "service endpoint not available") {
		t.Errorf("PgpassStorage.Get() should mention endpoint not available, got: %v", err)
	}
}

func TestPgpassStorage_Get_NoFile(t *testing.T) {
	// Create a temporary directory with no .pgpass file
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	storage := &PgpassStorage{}
	service := createTestService("test-service-123")

	password, err := storage.Get(service)
	if err == nil {
		t.Error("PgpassStorage.Get() should return error when .pgpass file doesn't exist")
	}
	if password != "" {
		t.Errorf("PgpassStorage.Get() should return empty password on error, got: %s", password)
	}
	if !strings.Contains(err.Error(), "no .pgpass file found") {
		t.Errorf("PgpassStorage.Get() should mention file not found, got: %v", err)
	}
}

func TestGetPasswordStorage(t *testing.T) {
	tests := []struct {
		name          string
		storageMethod string
		expectedType  string
	}{
		{"keyring", "keyring", "*password.KeyringStorage"},
		{"pgpass", "pgpass", "*password.PgpassStorage"},
		{"none", "none", "*password.NoStorage"},
		{"default", "", "*password.KeyringStorage"},        // Default case
		{"invalid", "invalid", "*password.KeyringStorage"}, // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up viper for this test
			viper.Set("password_storage", tt.storageMethod)

			storage := GetPasswordStorage()
			actualType := fmt.Sprintf("%T", storage)

			if actualType != tt.expectedType {
				t.Errorf("GetPasswordStorage() with %s = %v, want %v", tt.storageMethod, actualType, tt.expectedType)
			}

			// Clean up
			viper.Set("password_storage", "")
		})
	}
}

// Test pgpass storage with a temporary directory
func TestPgpassStorage_Integration(t *testing.T) {
	// Create a temporary directory for this test
	tempDir := t.TempDir()

	// Override the home directory for this test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", originalHome)

	storage := &PgpassStorage{}
	service := createTestService("test-service-123")
	password := "test-password"

	// Test Save
	err := storage.Save(service, password)
	if err != nil {
		t.Fatalf("PgpassStorage.Save() failed: %v", err)
	}

	// Verify the .pgpass file was created
	pgpassPath := filepath.Join(tempDir, ".pgpass")
	if _, err := os.Stat(pgpassPath); os.IsNotExist(err) {
		t.Fatal("Expected .pgpass file to be created")
	}

	// Read the file contents
	content, err := os.ReadFile(pgpassPath)
	if err != nil {
		t.Fatalf("Failed to read .pgpass file: %v", err)
	}

	expectedEntry := "test-host.tigerdata.com:5432:tsdb:tsdbadmin:test-password\n"
	if string(content) != expectedEntry {
		t.Errorf("Expected .pgpass entry %q, got %q", expectedEntry, string(content))
	}

	// Test Get
	retrievedPassword, err := storage.Get(service)
	if err != nil {
		t.Fatalf("PgpassStorage.Get() failed: %v", err)
	}
	if retrievedPassword != password {
		t.Errorf("PgpassStorage.Get() = %q, want %q", retrievedPassword, password)
	}

	// Test Remove
	err = storage.Remove(service)
	if err != nil {
		t.Fatalf("PgpassStorage.Remove() failed: %v", err)
	}

	// Verify the entry was removed (file should be empty or not exist)
	if _, err := os.Stat(pgpassPath); err == nil {
		content, err := os.ReadFile(pgpassPath)
		if err != nil {
			t.Fatalf("Failed to read .pgpass file after removal: %v", err)
		}
		if len(content) > 0 {
			t.Errorf("Expected .pgpass file to be empty after removal, got: %q", string(content))
		}
	}
}

// Test GetStorageResult methods for all storage types
func TestNoStorage_GetStorageResult(t *testing.T) {
	storage := &NoStorage{}
	testPassword := "test-password"

	// NoStorage should always return a "none" result regardless of error
	result := storage.GetStorageResult(nil, testPassword)
	if result.Success != false {
		t.Errorf("NoStorage.GetStorageResult() success should be false, got: %t", result.Success)
	}
	if result.Method != "none" {
		t.Errorf("NoStorage.GetStorageResult() method should be 'none', got: %s", result.Method)
	}
	if !strings.Contains(result.Message, "Password not saved (--password-storage=none)") {
		t.Errorf("NoStorage.GetStorageResult() should mention no storage, got: %s", result.Message)
	}

	// Test with error (should still return same message)
	result = storage.GetStorageResult(fmt.Errorf("some error"), testPassword)
	if result.Method != "none" {
		t.Errorf("NoStorage.GetStorageResult() with error should still have method 'none', got: %s", result.Method)
	}

	// Verify no password is ever in the message
	if strings.Contains(result.Message, testPassword) {
		t.Errorf("NoStorage.GetStorageResult() should not include actual passwords, got: %s", result.Message)
	}
}

func TestKeyringStorage_GetStorageResult_Success(t *testing.T) {
	storage := &KeyringStorage{}
	testPassword := "test-password"

	// Test success case
	result := storage.GetStorageResult(nil, testPassword)
	if !result.Success {
		t.Errorf("KeyringStorage.GetStorageResult() success should be true, got: %t", result.Success)
	}
	if result.Method != "keyring" {
		t.Errorf("KeyringStorage.GetStorageResult() method should be 'keyring', got: %s", result.Method)
	}
	if !strings.Contains(result.Message, "Password saved to system keyring") {
		t.Errorf("KeyringStorage.GetStorageResult() success should mention keyring, got: %s", result.Message)
	}
	if strings.Contains(result.Message, testPassword) {
		t.Errorf("KeyringStorage.GetStorageResult() should never include actual passwords, got: %s", result.Message)
	}
}

func TestKeyringStorage_GetStorageResult_Error(t *testing.T) {
	storage := &KeyringStorage{}
	testPassword := "test-password"

	// Test error case
	testErr := fmt.Errorf("keyring service not available")
	result := storage.GetStorageResult(testErr, testPassword)
	if result.Success {
		t.Errorf("KeyringStorage.GetStorageResult() error should have success=false, got: %t", result.Success)
	}
	if result.Method != "keyring" {
		t.Errorf("KeyringStorage.GetStorageResult() method should be 'keyring', got: %s", result.Method)
	}
	if !strings.Contains(result.Message, "Failed to save password to keyring") {
		t.Errorf("KeyringStorage.GetStorageResult() error should mention keyring failure, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "keyring service not available") {
		t.Errorf("KeyringStorage.GetStorageResult() should include error details, got: %s", result.Message)
	}
	if strings.Contains(result.Message, testPassword) {
		t.Errorf("KeyringStorage.GetStorageResult() should never include actual passwords, got: %s", result.Message)
	}
}

func TestPgpassStorage_GetStorageResult_Success(t *testing.T) {
	storage := &PgpassStorage{}
	testPassword := "test-password"

	// Test success case
	result := storage.GetStorageResult(nil, testPassword)
	if !result.Success {
		t.Errorf("PgpassStorage.GetStorageResult() success should be true, got: %t", result.Success)
	}
	if result.Method != "pgpass" {
		t.Errorf("PgpassStorage.GetStorageResult() method should be 'pgpass', got: %s", result.Method)
	}
	if !strings.Contains(result.Message, "Password saved to ~/.pgpass") {
		t.Errorf("PgpassStorage.GetStorageResult() success should mention pgpass, got: %s", result.Message)
	}
	if strings.Contains(result.Message, testPassword) {
		t.Errorf("PgpassStorage.GetStorageResult() should never include actual passwords, got: %s", result.Message)
	}
}

func TestPgpassStorage_GetStorageResult_Error(t *testing.T) {
	storage := &PgpassStorage{}
	testPassword := "test-password"

	// Test error case
	testErr := fmt.Errorf("permission denied")
	result := storage.GetStorageResult(testErr, testPassword)
	if result.Success {
		t.Errorf("PgpassStorage.GetStorageResult() error should have success=false, got: %t", result.Success)
	}
	if result.Method != "pgpass" {
		t.Errorf("PgpassStorage.GetStorageResult() method should be 'pgpass', got: %s", result.Method)
	}
	if !strings.Contains(result.Message, "Failed to save password to ~/.pgpass") {
		t.Errorf("PgpassStorage.GetStorageResult() error should mention pgpass failure, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "permission denied") {
		t.Errorf("PgpassStorage.GetStorageResult() should include error details, got: %s", result.Message)
	}
	if strings.Contains(result.Message, testPassword) {
		t.Errorf("PgpassStorage.GetStorageResult() should never include actual passwords, got: %s", result.Message)
	}
}

// Test SavePasswordWithResult function
func TestSavePasswordWithResult_EmptyPassword(t *testing.T) {
	service := createTestService("test-service-123")

	result, err := SavePasswordWithResult(service, "")
	if err != nil {
		t.Errorf("SavePasswordWithResult() with empty password should not return error, got: %v", err)
	}
	if result.Success {
		t.Errorf("SavePasswordWithResult() with empty password should not be successful, got: %t", result.Success)
	}
	if result.Method != "none" {
		t.Errorf("SavePasswordWithResult() with empty password should have method 'none', got: %s", result.Method)
	}
	if !strings.Contains(result.Message, "No password provided") {
		t.Errorf("SavePasswordWithResult() should mention no password, got: %s", result.Message)
	}
}

func TestSavePasswordWithResult_WithPassword(t *testing.T) {
	service := createTestService("test-service-123")

	// Set up viper to use NoStorage for predictable behavior
	viper.Set("password_storage", "none")
	defer viper.Set("password_storage", "")

	result, err := SavePasswordWithResult(service, "test-password")
	if err != nil {
		t.Errorf("SavePasswordWithResult() should not return error with NoStorage, got: %v", err)
	}
	if result.Success {
		t.Errorf("SavePasswordWithResult() with NoStorage should not be successful, got: %t", result.Success)
	}
	if result.Method != "none" {
		t.Errorf("SavePasswordWithResult() should have method 'none', got: %s", result.Method)
	}
	if !strings.Contains(result.Message, "Password not saved (--password-storage=none)") {
		t.Errorf("SavePasswordWithResult() should mention no storage, got: %s", result.Message)
	}
	// Make sure password is never in the result
	if strings.Contains(result.Message, "test-password") {
		t.Errorf("SavePasswordWithResult() should never include actual passwords, got: %s", result.Message)
	}
}

// Security test: Ensure no storage type ever includes passwords in messages
func TestGetStorageResult_SecurityTest_NoPasswordInResults(t *testing.T) {
	testPassword := "super-secret-password-123"
	testError := fmt.Errorf("failed to save %s", testPassword)

	storages := []PasswordStorage{
		&NoStorage{},
		&KeyringStorage{},
		&PgpassStorage{},
	}

	for _, storage := range storages {
		t.Run(fmt.Sprintf("%T", storage), func(t *testing.T) {
			// Test both success and error cases
			successResult := storage.GetStorageResult(nil, testPassword)
			errorResult := storage.GetStorageResult(testError, testPassword)

			// Verify password is never in any result message
			if strings.Contains(successResult.Message, testPassword) {
				t.Errorf("%T.GetStorageResult() success should never include password, got: %s", storage, successResult.Message)
			}
			if strings.Contains(errorResult.Message, testPassword) {
				t.Errorf("%T.GetStorageResult() error should never include password, got: %s", storage, errorResult.Message)
			}

			// For errors containing passwords, verify they are masked
			if strings.Contains(errorResult.Message, "***") {
				t.Logf("%T.GetStorageResult() correctly masked password in error message: %s", storage, errorResult.Message)
			}
		})
	}
}

// Test the buildPasswordKeyringUsername helper function
func TestBuildPasswordKeyringUsername(t *testing.T) {
	tests := []struct {
		name        string
		service     api.Service
		expected    string
		expectError bool
	}{
		{
			name: "valid service with both IDs",
			service: api.Service{
				ProjectId: util.Ptr("project-123"),
				ServiceId: util.Ptr("service-456"),
			},
			expected:    "password-project-123-service-456",
			expectError: false,
		},
		{
			name: "missing service ID",
			service: api.Service{
				ProjectId: util.Ptr("project-123"),
			},
			expected:    "",
			expectError: true,
		},
		{
			name: "missing project ID",
			service: api.Service{
				ServiceId: util.Ptr("service-456"),
			},
			expected:    "",
			expectError: true,
		},
		{
			name:        "missing both IDs",
			service:     api.Service{},
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildPasswordKeyringUsername(tt.service)

			if tt.expectError && err == nil {
				t.Errorf("buildPasswordKeyringUsername() expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("buildPasswordKeyringUsername() unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("buildPasswordKeyringUsername() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Test that all storage types properly overwrite previous values
func TestPasswordStorage_OverwritePreviousValue(t *testing.T) {
	tests := []struct {
		name    string
		storage PasswordStorage
		setup   func(t *testing.T)
		cleanup func(t *testing.T, service api.Service)
	}{
		{
			name:    "KeyringStorage",
			storage: &KeyringStorage{},
			cleanup: func(t *testing.T, service api.Service) {
				storage := &KeyringStorage{}
				storage.Remove(service) // Ignore errors in cleanup
			},
		},
		{
			name:    "PgpassStorage",
			storage: &PgpassStorage{},
			setup: func(t *testing.T) {
				tempDir := t.TempDir()
				originalHome := os.Getenv("HOME")
				os.Setenv("HOME", tempDir)
				t.Cleanup(func() {
					os.Setenv("HOME", originalHome)
				})
			},
		},
		{
			name:    "NoStorage",
			storage: &NoStorage{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}

			service := createTestService("overwrite-test-service")

			if tt.cleanup != nil {
				defer tt.cleanup(t, service)
			}

			originalPassword := "original-password-123"
			newPassword := "new-password-456"

			// Save original password
			err1 := tt.storage.Save(service, originalPassword)

			// Save new password (should overwrite)
			err2 := tt.storage.Save(service, newPassword)

			// Handle NoStorage specially (always succeeds but doesn't store)
			if _, ok := tt.storage.(*NoStorage); ok {
				if err1 != nil || err2 != nil {
					t.Errorf("NoStorage.Save() should not return error, got first: %v, second: %v", err1, err2)
				}
				// NoStorage.Get() always returns error, so we can't test retrieval
				_, err := tt.storage.Get(service)
				if err == nil {
					t.Error("NoStorage.Get() should always return error")
				}
				return
			}

			// For storage types that can store and retrieve
			if err1 != nil {
				// If first save failed, second might fail too (e.g., keyring unavailable in CI)
				if err2 != nil {
					t.Skipf("Storage not available in test environment - first: %v, second: %v", err1, err2)
				}
				t.Fatalf("First Save() failed but second succeeded - inconsistent: first: %v, second: %v", err1, err2)
			}
			if err2 != nil {
				t.Fatalf("Second Save() failed: %v", err2)
			}

			// Get the stored password - should be the new one
			retrieved, err := tt.storage.Get(service)
			if err != nil {
				t.Fatalf("Get() failed: %v", err)
			}

			if retrieved != newPassword {
				t.Errorf("Get() = %q, want %q (new password should overwrite old)", retrieved, newPassword)
			}
			if retrieved == originalPassword {
				t.Errorf("Get() returned original password %q, should have been overwritten with %q", originalPassword, newPassword)
			}
		})
	}
}

// Test the sanitizeErrorMessage helper function directly
func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		password string
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			password: "secret",
			expected: "",
		},
		{
			name:     "error without password",
			err:      fmt.Errorf("connection failed"),
			password: "secret",
			expected: "connection failed",
		},
		{
			name:     "error containing password",
			err:      fmt.Errorf("failed to save secret to keyring"),
			password: "secret",
			expected: "failed to save *** to keyring",
		},
		{
			name:     "error with multiple password occurrences",
			err:      fmt.Errorf("secret failed: secret not found"),
			password: "secret",
			expected: "*** failed: *** not found",
		},
		{
			name:     "empty password",
			err:      fmt.Errorf("failed to save secret"),
			password: "",
			expected: "failed to save secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeErrorMessage(tt.err, tt.password)
			if result != tt.expected {
				t.Errorf("sanitizeErrorMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}
