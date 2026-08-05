package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

func TestDBSavePassword_ExplicitPassword(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)
	tmpDir := setupDBTest(t)

	// Set keyring as the password storage method for this test
	t.Setenv("TIGER_PASSWORD_STORAGE", "keyring")

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
	mockTestPAT(t)
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

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
	storage := common.GetPasswordStorage(testConfig(t))
	retrievedPassword, err := storage.Get(mockService, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to retrieve saved password: %v", err)
	}
	defer storage.Remove(mockService, "tsdbadmin")

	if retrievedPassword != testPassword {
		t.Errorf("Expected password %q, got %q", testPassword, retrievedPassword)
	}
}

// TestDBSavePassword_ReplicaResolvesToParent verifies that passing a read
// replica id stores the password against the parent primary, so it is found by
// the connect/test-connection read path.
func TestDBSavePassword_ReplicaResolvesToParent(t *testing.T) {
	config.SetTestServiceName(t)
	tmpDir := setupDBTest(t)

	t.Setenv("TIGER_PASSWORD_STORAGE", "keyring")

	const projectID = "test-project-123"
	port := 5432
	primaryHost := "svcprimary.example.com"
	replicaHost := "replica.example.com"
	primary := api.Service{
		ServiceId: util.Ptr("svcprimary"),
		ProjectId: util.Ptr(projectID),
		Endpoint:  &api.Endpoint{Host: &primaryHost, Port: &port},
	}
	replica := api.Service{
		ServiceId: util.Ptr("rep1234567"),
		ProjectId: util.Ptr(projectID),
		Endpoint:  &api.Endpoint{Host: &replicaHost, Port: &port},
		ForkedFrom: &api.ForkSpec{
			IsStandby: util.Ptr(true),
			ProjectId: util.Ptr(projectID),
			ServiceId: util.Ptr("svcprimary"),
		},
	}

	// Serve the parent primary lookup; the replica itself comes from the mocked
	// getServiceDetailsFunc, so only the parent fetch reaches the API.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if parts[len(parts)-1] == "svcprimary" {
			_ = json.NewEncoder(w).Encode(primary)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
	}))
	defer srv.Close()

	_, err := config.UseTestConfig(tmpDir, map[string]any{
		"api_url":    srv.URL,
		"project_id": projectID,
		"service_id": "rep1234567",
	})
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	mockTestPAT(t)
	originalGetServiceDetails := getServiceDetailsFunc
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return replica, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

	const testPassword = "replica-parent-pw"
	output, err := executeDBCommand(t.Context(), "db", "save-password", "rep1234567", "--password="+testPassword)
	if err != nil {
		t.Fatalf("Expected save-password to succeed, got error: %v", err)
	}
	if !strings.Contains(output, "svcprimary") {
		t.Errorf("expected parent primary id in output, got: %s", output)
	}

	storage := common.GetPasswordStorage(testConfig(t))
	// Stored against the parent primary, matching the connect read path.
	got, err := storage.Get(primary, "tsdbadmin")
	if err != nil {
		t.Fatalf("expected password stored under primary, got error: %v", err)
	}
	defer storage.Remove(primary, "tsdbadmin")
	if got != testPassword {
		t.Errorf("expected %q under primary, got %q", testPassword, got)
	}
	// Not stored under the replica id.
	if pw, err := storage.Get(replica, "tsdbadmin"); err == nil && pw != "" {
		t.Errorf("expected no password under replica, got %q", pw)
	}
}

func TestDBSavePassword_EnvironmentVariable(t *testing.T) {
	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)
	tmpDir := setupDBTest(t)

	// Set keyring as the password storage method for this test
	t.Setenv("TIGER_PASSWORD_STORAGE", "keyring")

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
	mockTestPAT(t)
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

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
	storage := common.GetPasswordStorage(testConfig(t))
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
	t.Setenv("TIGER_PASSWORD_STORAGE", "keyring")

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
	mockTestPAT(t)
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

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
	storage := common.GetPasswordStorage(testConfig(t))
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
	mockTestPAT(t)
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

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
	t.Setenv("TIGER_PASSWORD_STORAGE", "keyring")

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
	mockTestPAT(t)
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

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
	storage := common.GetPasswordStorage(testConfig(t))
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
	mockTestPAT(t)

	// No need to mock service details since it should fail before reaching getServiceDetailsFunc

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
	mockNotLoggedIn(t)

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
	t.Setenv("TIGER_PASSWORD_STORAGE", "pgpass")

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
	mockTestPAT(t)
	getServiceDetailsFunc = func(cmd *cobra.Command, app *common.App, args []string) (api.Service, error) {
		return mockService, nil
	}
	defer func() { getServiceDetailsFunc = originalGetServiceDetails }()

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
	storage := common.GetPasswordStorage(testConfig(t))
	retrievedPassword, err := storage.Get(mockService, "tsdbadmin")
	if err != nil {
		t.Fatalf("Failed to retrieve saved password from pgpass: %v", err)
	}
	defer storage.Remove(mockService, "tsdbadmin")

	if retrievedPassword != testPassword {
		t.Errorf("Expected password %q, got %q", testPassword, retrievedPassword)
	}
}
