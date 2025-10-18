package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
)

// setupIntegrationTest sets up isolated test environment with temporary config directory
func setupIntegrationTest(t *testing.T) string {
	t.Helper()

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Reset global config and viper to ensure test isolation
	config.ResetGlobalConfig()

	// Re-establish viper environment configuration after reset
	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

	// Set API URL in temporary config if integration URL is provided
	if apiURL := os.Getenv("TIGER_API_URL_INTEGRATION"); apiURL != "" {
		// Use a simple command execution without the full executeIntegrationCommand wrapper
		// to avoid circular dependencies during setup
		rootCmd := buildRootCmd()
		rootCmd.SetArgs([]string{"config", "set", "api_url", apiURL})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Failed to set integration API URL during setup: %v", err)
		}
	}

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

// executeIntegrationCommand executes a CLI command for integration testing
func executeIntegrationCommand(args ...string) (string, error) {
	// Reset both global config and viper before each command execution
	// This ensures fresh config loading with proper flag precedence
	config.ResetGlobalConfig()

	// Re-establish viper environment configuration after reset
	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot := buildRootCmd()

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err := testRoot.Execute()
	return buf.String(), err
}

// TestServiceLifecycleIntegration tests the complete authentication and service lifecycle:
// login -> status -> create -> get -> update-password -> delete -> logout
func TestServiceLifecycleIntegration(t *testing.T) {
	config.SetTestServiceName(t)
	// Check for required environment variables
	publicKey := os.Getenv("TIGER_PUBLIC_KEY_INTEGRATION")
	secretKey := os.Getenv("TIGER_SECRET_KEY_INTEGRATION")
	projectID := os.Getenv("TIGER_PROJECT_ID_INTEGRATION")
	if publicKey == "" || secretKey == "" || projectID == "" {
		t.Skip("Skipping integration test: TIGER_PUBLIC_KEY_INTEGRATION, TIGER_SECRET_KEY_INTEGRATION, and TIGER_PROJECT_ID_INTEGRATION must be set")
	}

	// Set up isolated test environment with temporary config directory
	tmpDir := setupIntegrationTest(t)
	t.Logf("Using temporary config directory: %s", tmpDir)

	// Generate unique service name to avoid conflicts
	serviceName := fmt.Sprintf("integration-test-%d", time.Now().Unix())
	var serviceID string
	var deletedServiceID string // Keep track of deleted service for verification

	// Always logout at the end to clean up credentials
	defer func() {
		t.Logf("Cleaning up authentication")
		_, err := executeIntegrationCommand("auth", "logout")
		if err != nil {
			t.Logf("Warning: Failed to logout: %v", err)
		}
	}()

	// Cleanup function to ensure service is deleted even if test fails
	defer func() {
		if serviceID != "" {
			t.Logf("Cleaning up service: %s", serviceID)
			// Best effort cleanup - don't fail the test if cleanup fails
			_, err := executeIntegrationCommand(
				"service", "delete", serviceID,
				"--confirm",
				"--wait-timeout", "5m",
			)
			if err != nil {
				t.Logf("Warning: Failed to cleanup service %s: %v", serviceID, err)
			}
		}
	}()

	t.Run("Login", func(t *testing.T) {
		t.Logf("Logging in with public key: %s", publicKey[:8]+"...") // Only show first 8 chars

		output, err := executeIntegrationCommand(
			"auth", "login",
			"--public-key", publicKey,
			"--secret-key", secretKey,
			"--project-id", projectID,
		)

		if err != nil {
			t.Fatalf("Login failed: %v\nOutput: %s", err, output)
		}

		// Verify login success message
		if !strings.Contains(output, "Successfully logged in") && !strings.Contains(output, "Logged in") {
			t.Errorf("Login output: %s", output)
		}

		t.Logf("Login successful")
	})

	t.Run("Status", func(t *testing.T) {
		t.Logf("Verifying authentication status")

		output, err := executeIntegrationCommand("auth", "status")
		if err != nil {
			t.Fatalf("Status failed: %v\nOutput: %s", err, output)
		}

		// Should not say "Not logged in"
		if strings.Contains(output, "Not logged in") {
			t.Errorf("Expected to be logged in, but got: %s", output)
		}

		t.Logf("Current authentication status: %s", strings.TrimSpace(output))
	})

	t.Run("CreateService", func(t *testing.T) {
		t.Logf("Creating service: %s", serviceName)

		output, err := executeIntegrationCommand(
			"service", "create",
			"--name", serviceName,
			"--wait-timeout", "15m", // Longer timeout for integration tests
			"--no-set-default", // Don't modify user's default service
			"--output", "json", // Use JSON for easier parsing
		)

		if err != nil {
			t.Fatalf("Service creation failed: %v\nOutput: %s", err, output)
		}

		// Extract service ID from JSON output
		extractedServiceID := extractServiceIDFromCreateOutput(t, output)
		if extractedServiceID == "" {
			t.Fatalf("Could not extract service ID from create output: %s", output)
		}

		serviceID = extractedServiceID
		t.Logf("Created service with ID: %s", serviceID)
	})

	t.Run("ListServices", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Listing services to verify creation")

		output, err := executeIntegrationCommand(
			"service", "list",
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Service list failed: %v\nOutput: %s", err, output)
		}

		// Verify our service appears in the list
		if !strings.Contains(output, serviceID) {
			t.Errorf("Service ID %s not found in service list output", serviceID)
		}

		if !strings.Contains(output, serviceName) {
			t.Errorf("Service name %s not found in service list output", serviceName)
		}
	})

	t.Run("GetService", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Getting service details: %s", serviceID)

		output, err := executeIntegrationCommand(
			"service", "get", serviceID,
			"--output", "json",
		)

		t.Logf("Raw service get output: %s", output)

		if err != nil {
			t.Fatalf("Service get failed: %v\nOutput: %s", err, output)
		}

		// Parse JSON to verify service details
		var service api.Service
		if err := json.Unmarshal([]byte(output), &service); err != nil {
			t.Fatalf("Failed to parse service JSON: %v\nOutput: %s", err, output)
		}

		// Verify service details
		if service.ServiceId == nil || *service.ServiceId != serviceID {
			t.Errorf("Expected service ID %s, got %v", serviceID, service.ServiceId)
		}

		if service.Name == nil || *service.Name != serviceName {
			t.Errorf("Expected service name %s, got %v", serviceName, service.Name)
		}

		// Verify service has expected structure
		if service.Endpoint == nil {
			t.Error("Service endpoint should not be nil")
		}

		t.Logf("Service status: %v", service.Status)
	})

	t.Run("DatabasePsqlCommand_OriginalPassword", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Testing psql command with original password for service: %s", serviceID)

		output, err := executeIntegrationCommand(
			"db", "psql", serviceID,
			"--", "-c", "SELECT 1 as original_password_test;",
		)

		if err != nil {
			t.Fatalf("Database psql command with original password failed: %v\nOutput: %s", err, output)
		}

		// Verify we got expected output from SELECT 1
		if !strings.Contains(output, "1") && !strings.Contains(output, "original_password_test") {
			t.Errorf("psql command succeeded but output format unexpected - expected to contain '1' or 'original_password_test': %s", output)
		}

		t.Logf("✅ psql command with original password succeeded")
	})

	t.Run("UpdatePassword", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		newPassword := fmt.Sprintf("integration-test-password-%d", time.Now().Unix())

		t.Logf("Updating password for service: %s", serviceID)

		output, err := executeIntegrationCommand(
			"service", "update-password", serviceID,
			"--new-password", newPassword,
			"--password-storage", "keychain", // Save to keychain for psql test
		)

		if err != nil {
			t.Fatalf("Password update failed: %v\nOutput: %s", err, output)
		}

		// Verify success message
		if !strings.Contains(output, "updated successfully") {
			t.Errorf("Expected success message in output: %s", output)
		}
	})

	t.Run("DatabaseConnectionString", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Getting connection string for service: %s", serviceID)

		output, err := executeIntegrationCommand(
			"db", "connection-string", serviceID,
		)

		if err != nil {
			t.Fatalf("Connection string failed: %v\nOutput: %s", err, output)
		}

		// Verify connection string format
		if !strings.HasPrefix(strings.TrimSpace(output), "postgresql://") {
			t.Errorf("Expected PostgreSQL connection string, got: %s", output)
		}

		// Verify connection string contains expected components
		if !strings.Contains(output, "sslmode=require") {
			t.Errorf("Expected connection string to include sslmode=require: %s", output)
		}
	})

	t.Run("DatabasePsqlCommand_UpdatedPassword", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Testing psql command with updated password for service: %s", serviceID)

		output, err := executeIntegrationCommand(
			"db", "psql", serviceID,
			"--", "-c", "SELECT 1 as updated_password_test;",
		)

		if err != nil {
			t.Fatalf("Database psql command with updated password failed: %v\nOutput: %s", err, output)
		}

		// Verify we got expected output from SELECT 1
		if !strings.Contains(output, "1") && !strings.Contains(output, "updated_password_test") {
			t.Errorf("psql command succeeded but output format unexpected - expected to contain '1' or 'updated_password_test': %s", output)
		}

		t.Logf("✅ psql command with updated password succeeded")
	})

	// Track created roles for cleanup - must capture serviceID since it gets cleared later
	var createdRoles []string
	cleanupRoles := func() {
		// Clean up all created roles - use captured serviceID
		capturedServiceID := serviceID
		if capturedServiceID == "" {
			t.Logf("Warning: No service ID available for role cleanup")
			return
		}
		for _, roleName := range createdRoles {
			t.Logf("Cleaning up role: %s", roleName)
			// Use psql to drop the role - best effort cleanup
			_, err := executeIntegrationCommand(
				"db", "psql", capturedServiceID,
				"--", "-c", fmt.Sprintf("DROP ROLE IF EXISTS %s;", roleName),
			)
			if err != nil {
				t.Logf("Warning: Failed to cleanup role %s: %v", roleName, err)
			}
		}
	}

	t.Run("CreateRole_Basic", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		roleName := fmt.Sprintf("integration_test_role_%d", time.Now().Unix())
		createdRoles = append(createdRoles, roleName)

		t.Logf("Creating basic role: %s", roleName)

		output, err := executeIntegrationCommand(
			"db", "create", "role", serviceID,
			"--name", roleName,
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Create role failed: %v\nOutput: %s", err, output)
		}

		// Parse JSON output
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("Failed to parse create role JSON: %v\nOutput: %s", err, output)
		}

		// Verify role name in output
		if result["role_name"] != roleName {
			t.Errorf("Expected role_name=%s in output, got: %v", roleName, result["role_name"])
		}

		t.Logf("✅ Successfully created basic role: %s", roleName)
	})

	t.Run("CreateRole_WithExplicitPassword", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		roleName := fmt.Sprintf("integration_test_role_pwd_%d", time.Now().Unix())
		password := fmt.Sprintf("test-password-%d", time.Now().Unix())
		createdRoles = append(createdRoles, roleName)

		t.Logf("Creating role with explicit password: %s", roleName)

		output, err := executeIntegrationCommand(
			"db", "create", "role", serviceID,
			"--name", roleName,
			"--password", password,
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Create role with password failed: %v\nOutput: %s", err, output)
		}

		// Parse JSON output
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("Failed to parse create role JSON: %v\nOutput: %s", err, output)
		}

		// Verify role name
		if result["role_name"] != roleName {
			t.Errorf("Expected role_name=%s in output, got: %v", roleName, result["role_name"])
		}

		t.Logf("✅ Successfully created role with explicit password: %s", roleName)
	})

	t.Run("CreateRole_WithInheritedGrants", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		roleName := fmt.Sprintf("integration_test_role_grants_%d", time.Now().Unix())
		createdRoles = append(createdRoles, roleName)

		t.Logf("Creating role with inherited grants from tsdbadmin: %s", roleName)

		output, err := executeIntegrationCommand(
			"db", "create", "role", serviceID,
			"--name", roleName,
			"--from", "tsdbadmin",
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Create role with --from failed: %v\nOutput: %s", err, output)
		}

		// Parse JSON output
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("Failed to parse create role JSON: %v\nOutput: %s", err, output)
		}

		// Verify role name
		if result["role_name"] != roleName {
			t.Errorf("Expected role_name=%s in output, got: %v", roleName, result["role_name"])
		}

		// Verify from_roles in output
		fromRoles, ok := result["from_roles"].([]interface{})
		if !ok || len(fromRoles) == 0 {
			t.Error("Expected from_roles in output")
		} else if fromRoles[0] != "tsdbadmin" {
			t.Errorf("Expected from_roles to contain 'tsdbadmin', got: %v", fromRoles)
		}

		t.Logf("✅ Successfully created role with inherited grants: %s", roleName)
	})

	t.Run("CreateRole_ReadOnly", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		roleName := fmt.Sprintf("integration_test_role_readonly_%d", time.Now().Unix())
		createdRoles = append(createdRoles, roleName)

		t.Logf("Creating read-only role: %s", roleName)

		output, err := executeIntegrationCommand(
			"db", "create", "role", serviceID,
			"--name", roleName,
			"--read-only",
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Create read-only role failed: %v\nOutput: %s", err, output)
		}

		// Parse JSON output
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("Failed to parse create role JSON: %v\nOutput: %s", err, output)
		}

		// Verify read_only flag in output
		if readOnly, ok := result["read_only"].(bool); !ok || !readOnly {
			t.Errorf("Expected read_only=true in output, got: %v", result["read_only"])
		}

		t.Logf("✅ Successfully created read-only role: %s", roleName)
	})

	t.Run("CreateRole_WithStatementTimeout", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		roleName := fmt.Sprintf("integration_test_role_timeout_%d", time.Now().Unix())
		createdRoles = append(createdRoles, roleName)

		t.Logf("Creating role with statement timeout: %s", roleName)

		output, err := executeIntegrationCommand(
			"db", "create", "role", serviceID,
			"--name", roleName,
			"--statement-timeout", "30s",
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Create role with statement timeout failed: %v\nOutput: %s", err, output)
		}

		// Parse JSON output
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("Failed to parse create role JSON: %v\nOutput: %s", err, output)
		}

		// Verify statement_timeout in output
		if result["statement_timeout"] != "30s" {
			t.Errorf("Expected statement_timeout=30s in output, got: %v", result["statement_timeout"])
		}

		t.Logf("✅ Successfully created role with statement timeout: %s", roleName)
	})

	t.Run("CreateRole_AllOptions", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		roleName := fmt.Sprintf("integration_test_role_all_%d", time.Now().Unix())
		password := fmt.Sprintf("test-password-all-%d", time.Now().Unix())
		createdRoles = append(createdRoles, roleName)

		t.Logf("Creating role with all options: %s", roleName)

		output, err := executeIntegrationCommand(
			"db", "create", "role", serviceID,
			"--name", roleName,
			"--password", password,
			"--from", "tsdbadmin",
			"--read-only",
			"--statement-timeout", "1m",
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Create role with all options failed: %v\nOutput: %s", err, output)
		}

		// Parse JSON output
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("Failed to parse create role JSON: %v\nOutput: %s", err, output)
		}

		// Verify all options in output
		if result["role_name"] != roleName {
			t.Errorf("Expected role_name=%s, got: %v", roleName, result["role_name"])
		}
		if readOnly, ok := result["read_only"].(bool); !ok || !readOnly {
			t.Errorf("Expected read_only=true, got: %v", result["read_only"])
		}
		if result["statement_timeout"] != "1m0s" {
			t.Errorf("Expected statement_timeout=1m0s, got: %v", result["statement_timeout"])
		}

		t.Logf("✅ Successfully created role with all options: %s", roleName)
	})

	t.Run("CreateRole_VerifyRolesExist", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		if len(createdRoles) == 0 {
			t.Skip("No roles created to verify")
		}

		// Verify all created roles exist by querying pg_roles
		t.Logf("Verifying created roles exist in database")

		for _, roleName := range createdRoles {
			output, err := executeIntegrationCommand(
				"db", "psql", serviceID,
				"--", "-c", fmt.Sprintf("SELECT rolname FROM pg_roles WHERE rolname = '%s';", roleName),
			)

			if err != nil {
				t.Errorf("Failed to verify role %s exists: %v\nOutput: %s", roleName, err, output)
				continue
			}

			// Verify role name appears in output
			if !strings.Contains(output, roleName) {
				t.Errorf("Role %s not found in pg_roles query output: %s", roleName, output)
			} else {
				t.Logf("✅ Verified role exists in database: %s", roleName)
			}
		}
	})

	t.Run("CreateRole_DuplicateName", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		if len(createdRoles) == 0 {
			t.Skip("No roles created to test duplicate")
		}

		// Try to create a role with the same name as the first created role
		duplicateRoleName := createdRoles[0]

		t.Logf("Attempting to create duplicate role: %s", duplicateRoleName)

		output, err := executeIntegrationCommand(
			"db", "create", "role", serviceID,
			"--name", duplicateRoleName,
		)

		// This should fail with a role already exists error
		if err == nil {
			t.Errorf("Expected duplicate role creation to fail, but got output: %s", output)
		} else {
			// Verify error message indicates role already exists
			if !strings.Contains(err.Error(), "already exists") && !strings.Contains(output, "already exists") {
				t.Logf("Note: Error message may not contain 'already exists': %v\nOutput: %s", err, output)
			} else {
				t.Logf("✅ Duplicate role creation correctly failed")
			}
		}
	})

	t.Run("DeleteService", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available for deletion")
		}

		// Clean up roles before deleting the service
		cleanupRoles()

		t.Logf("Deleting service: %s", serviceID)

		output, err := executeIntegrationCommand(
			"service", "delete", serviceID,
			"--confirm",
			"--wait-timeout", "10m",
		)

		if err != nil {
			t.Fatalf("Service deletion failed: %v\nOutput: %s", err, output)
		}

		// Verify deletion success message
		if !strings.Contains(output, "deleted successfully") && !strings.Contains(output, "Deletion completed") && !strings.Contains(output, "successfully deleted") {
			t.Errorf("Expected deletion success message in output: %s", output)
		}

		// Store serviceID for verification, then clear it so cleanup doesn't try to delete again
		deletedServiceID = serviceID
		serviceID = ""
	})

	t.Run("VerifyServiceDeleted", func(t *testing.T) {
		if deletedServiceID == "" {
			t.Skip("No deleted service ID available for verification")
		}

		t.Logf("Verifying service %s no longer exists", deletedServiceID)

		// Try to get the deleted service - should fail
		output, err := executeIntegrationCommand(
			"service", "get", deletedServiceID,
		)

		// We expect this to fail since the service should be deleted
		if err == nil {
			t.Errorf("Expected service get to fail for deleted service, but got output: %s", output)
		}

		// Check that error indicates service not found
		if !strings.Contains(err.Error(), "no service with that id exists") {
			t.Errorf("Expected 'no service with that id exists' error for deleted service, got: %v", err)
		}

		// Check that it returns the correct exit code (this should be required)
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			if exitErr.ExitCode() != ExitServiceNotFound {
				t.Errorf("Expected exit code %d for service not found, got %d", ExitServiceNotFound, exitErr.ExitCode())
			}
		} else {
			t.Error("Expected exitCodeError with ExitServiceNotFound exit code for deleted service")
		}
	})

	t.Run("Logout", func(t *testing.T) {
		t.Logf("Logging out")

		output, err := executeIntegrationCommand("auth", "logout")
		if err != nil {
			t.Fatalf("Logout failed: %v\nOutput: %s", err, output)
		}

		// Verify logout success message
		if !strings.Contains(output, "Successfully logged out") && !strings.Contains(output, "Logged out") {
			t.Errorf("Logout output: %s", output)
		}

		t.Logf("Logout successful")
	})

	t.Run("VerifyLoggedOut", func(t *testing.T) {
		t.Logf("Verifying we're logged out")

		output, err := executeIntegrationCommand("auth", "status")
		// This should either fail or say "Not logged in"
		if err == nil && !strings.Contains(output, "Not logged in") {
			t.Errorf("Expected to be logged out, but status succeeded: %s", output)
		}

		t.Logf("Verified logged out status")
	})
}

// extractServiceIDFromCreateOutput extracts the service ID from service create command output
func extractServiceIDFromCreateOutput(t *testing.T, output string) string {
	t.Helper()

	// Try to parse as JSON first (if --output json was used)
	var service api.Service
	if err := json.Unmarshal([]byte(output), &service); err == nil {
		if service.ServiceId != nil {
			return *service.ServiceId
		}
	}

	// Fall back to regex extraction for text output
	// Look for service ID pattern (svc- followed by alphanumeric characters)
	serviceIDRegex := regexp.MustCompile(`svc-[a-zA-Z0-9]+`)
	matches := serviceIDRegex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0]
	}

	// Try to extract from structured output lines
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Service ID") || strings.Contains(line, "service_id") {
			// Extract ID from lines like "📋 Service ID: p7yqpiw7a8" or "service_id: svc-12345"
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				id := strings.TrimSpace(parts[1])
				// Look for any service ID pattern (not just svc- prefix)
				serviceIDRegex := regexp.MustCompile(`[a-zA-Z0-9]{8,}`)
				if match := serviceIDRegex.FindString(id); match != "" {
					return match
				}
			}
		}
	}

	return ""
}

// TestServiceNotFound tests that commands requiring service ID fail with correct exit code for non-existent services
func TestServiceNotFound(t *testing.T) {
	// Check for required environment variables
	publicKey := os.Getenv("TIGER_PUBLIC_KEY_INTEGRATION")
	secretKey := os.Getenv("TIGER_SECRET_KEY_INTEGRATION")
	projectID := os.Getenv("TIGER_PROJECT_ID_INTEGRATION")
	config.SetTestServiceName(t)

	if publicKey == "" || secretKey == "" || projectID == "" {
		t.Skip("Skipping service not found test: TIGER_PUBLIC_KEY_INTEGRATION, TIGER_SECRET_KEY_INTEGRATION, and TIGER_PROJECT_ID_INTEGRATION must be set")
	}

	// Set up isolated test environment with temporary config directory
	tmpDir := setupIntegrationTest(t)
	t.Logf("Using temporary config directory: %s", tmpDir)

	// Always logout at the end to clean up credentials
	defer func() {
		t.Logf("Cleaning up authentication")
		_, err := executeIntegrationCommand("auth", "logout")
		if err != nil {
			t.Logf("Warning: Failed to logout: %v", err)
		}
	}()

	// Login first
	output, err := executeIntegrationCommand(
		"auth", "login",
		"--public-key", publicKey,
		"--secret-key", secretKey,
		"--project-id", projectID,
	)

	if err != nil {
		t.Fatalf("Login failed: %v\nOutput: %s", err, output)
	}
	t.Logf("Login successful for service not found tests")

	// Use a definitely non-existent service ID
	nonExistentServiceID := "nonexistent-service-12345"

	// Table of commands that should fail with specific exit codes for non-existent services
	testCases := []struct {
		name             string
		args             []string
		expectedExitCode int
		reason           string
	}{
		{
			name:             "service get",
			args:             []string{"service", "get", nonExistentServiceID},
			expectedExitCode: ExitServiceNotFound,
		},
		{
			name:             "service update-password",
			args:             []string{"service", "update-password", nonExistentServiceID, "--new-password", "test-password"},
			expectedExitCode: ExitServiceNotFound,
		},
		{
			name:             "service delete",
			args:             []string{"service", "delete", nonExistentServiceID, "--confirm"},
			expectedExitCode: ExitServiceNotFound,
		},
		{
			name:             "db connection-string",
			args:             []string{"db", "connection-string", nonExistentServiceID},
			expectedExitCode: ExitServiceNotFound,
		},
		{
			name:             "db test-connection",
			args:             []string{"db", "test-connection", nonExistentServiceID},
			expectedExitCode: ExitInvalidParameters,
			reason:           "maintains compatibility with PostgreSQL tooling conventions",
		},
		{
			name:             "db psql",
			args:             []string{"db", "psql", nonExistentServiceID, "--", "-c", "SELECT 1;"},
			expectedExitCode: ExitServiceNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := executeIntegrationCommand(tc.args...)

			if err == nil {
				t.Errorf("Expected %s to fail for non-existent service, but got output: %s", tc.name, output)
				return
			}

			// Verify error message contains "not found"
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("Expected 'not found' error message for %s, got: %v", tc.name, err)
			}

			// Verify correct exit code
			if exitErr, ok := err.(interface{ ExitCode() int }); ok {
				if exitErr.ExitCode() != tc.expectedExitCode {
					t.Errorf("Expected exit code %d for %s, got %d", tc.expectedExitCode, tc.name, exitErr.ExitCode())
				}
			} else {
				t.Errorf("Expected exitCodeError with exit code %d for %s", tc.expectedExitCode, tc.name)
			}

			reasonMsg := ""
			if tc.reason != "" {
				reasonMsg = fmt.Sprintf(" (%s)", tc.reason)
			}
			t.Logf("✅ %s correctly failed with exit code %d%s", tc.name, tc.expectedExitCode, reasonMsg)
		})
	}
}

// TestDatabaseCommandsIntegration tests database-related commands that don't require service creation
func TestDatabaseCommandsIntegration(t *testing.T) {
	// Check for required environment variables
	publicKey := os.Getenv("TIGER_PUBLIC_KEY_INTEGRATION")
	secretKey := os.Getenv("TIGER_SECRET_KEY_INTEGRATION")
	projectID := os.Getenv("TIGER_PROJECT_ID_INTEGRATION")
	existingServiceID := os.Getenv("TIGER_EXISTING_SERVICE_ID_INTEGRATION") // Optional: use existing service
	config.SetTestServiceName(t)

	if publicKey == "" || secretKey == "" || projectID == "" {
		t.Skip("Skipping integration test: TIGER_PUBLIC_KEY_INTEGRATION, TIGER_SECRET_KEY_INTEGRATION, and TIGER_PROJECT_ID_INTEGRATION must be set")
	}

	if existingServiceID == "" {
		t.Skip("Skipping database integration test: TIGER_EXISTING_SERVICE_ID_INTEGRATION not set")
	}

	// Set up isolated test environment with temporary config directory
	tmpDir := setupIntegrationTest(t)
	t.Logf("Using temporary config directory: %s", tmpDir)

	// Always logout at the end to clean up credentials
	defer func() {
		t.Logf("Cleaning up authentication")
		_, err := executeIntegrationCommand("auth", "logout")
		if err != nil {
			t.Logf("Warning: Failed to logout: %v", err)
		}
	}()

	t.Run("Login", func(t *testing.T) {
		t.Logf("Logging in for database tests")

		output, err := executeIntegrationCommand(
			"auth", "login",
			"--public-key", publicKey,
			"--secret-key", secretKey,
			"--project-id", projectID,
		)

		if err != nil {
			t.Fatalf("Login failed: %v\nOutput: %s", err, output)
		}

		t.Logf("Login successful for database tests")
	})

	t.Run("DatabaseTestConnection", func(t *testing.T) {
		t.Logf("Testing database connection for service: %s", existingServiceID)

		output, err := executeIntegrationCommand(
			"db", "test-connection", existingServiceID,
			"--timeout", "30s",
		)

		// Note: This may fail if the database isn't fully ready or credentials aren't set up
		// We log the result but don't fail the test since it depends on database state
		if err != nil {
			t.Logf("Database connection test failed (this may be expected): %v\nOutput: %s", err, output)
		} else {
			t.Logf("Database connection test succeeded: %s", output)
		}
	})
}

// TestAuthenticationErrorsIntegration tests that all commands requiring authentication
// properly handle authentication failures and return appropriate exit codes
func TestAuthenticationErrorsIntegration(t *testing.T) {
	// Check if we have valid integration test credentials
	publicKey := os.Getenv("TIGER_PUBLIC_KEY_INTEGRATION")
	secretKey := os.Getenv("TIGER_SECRET_KEY_INTEGRATION")
	projectID := os.Getenv("TIGER_PROJECT_ID_INTEGRATION")
	config.SetTestServiceName(t)

	if publicKey == "" || secretKey == "" || projectID == "" {
		t.Skip("Skipping authentication error integration test: TIGER_PUBLIC_KEY_INTEGRATION, TIGER_SECRET_KEY_INTEGRATION, and TIGER_PROJECT_ID_INTEGRATION must be set")
	}

	// Set up isolated test environment with temporary config directory
	tmpDir := setupIntegrationTest(t)
	t.Logf("Using temporary config directory: %s", tmpDir)

	// Make sure we're logged out (this should always succeed or be a no-op)
	_, _ = executeIntegrationCommand("auth", "logout")

	// Log in with invalid credentials to trigger authentication errors (401 response from server)
	invalidPublicKey := "invalid-public-key"
	invalidSecretKey := "invalid-secret-key"

	// Login with invalid credentials (this should succeed locally but fail on API calls)
	loginOutput, loginErr := executeIntegrationCommand("auth", "login",
		"--public-key", invalidPublicKey,
		"--secret-key", invalidSecretKey,
		"--project-id", projectID)
	if loginErr != nil {
		t.Logf("Login with invalid credentials failed (expected): %s", loginOutput)
	} else {
		t.Fatalf("Cannot test authentication errors: login with invalid credentials succeeded: %v", loginErr)
	}

	// Test service commands that should return authentication errors
	serviceCommands := []struct {
		name string
		args []string
	}{
		{
			name: "service list",
			args: []string{"service", "list", "--project-id", projectID},
		},
		{
			name: "service get",
			args: []string{"service", "get", "non-existent-service", "--project-id", projectID},
		},
		{
			name: "service create",
			args: []string{"service", "create", "--name", "test-service", "--project-id", projectID, "--no-wait"},
		},
		{
			name: "service update-password",
			args: []string{"service", "update-password", "non-existent-service", "--new-password", "test-pass", "--project-id", projectID},
		},
		{
			name: "service delete",
			args: []string{"service", "delete", "non-existent-service", "--confirm", "--project-id", projectID, "--no-wait"},
		},
	}

	// Test db commands that should return authentication errors
	dbCommands := []struct {
		name string
		args []string
	}{
		{
			name: "db connection-string",
			args: []string{"db", "connection-string", "non-existent-service", "--project-id", projectID},
		},
		{
			name: "db connect",
			args: []string{"db", "connect", "non-existent-service", "--project-id", projectID},
		},
		// Note: db test-connection follows pg_isready conventions, so it uses exit code 3 (ExitInvalidParameters)
		// for authentication issues, not ExitAuthenticationError like other commands
		{
			name: "db test-connection",
			args: []string{"db", "test-connection", "non-existent-service", "--project-id", projectID},
		},
	}

	// Test all service commands
	for _, tc := range serviceCommands {
		t.Run(tc.name, func(t *testing.T) {
			output, err := executeIntegrationCommand(tc.args...)

			// Should fail with authentication error
			if err == nil {
				t.Errorf("Expected %s to fail with authentication error when using invalid API key, but got output: %s", tc.name, output)
				return
			}

			// Check that it's an exitCodeError with ExitAuthenticationError
			if exitErr, ok := err.(interface{ ExitCode() int }); ok {
				if exitErr.ExitCode() != ExitAuthenticationError {
					t.Errorf("Expected exit code %d (ExitAuthenticationError) for %s, got %d. Error: %s", ExitAuthenticationError, tc.name, exitErr.ExitCode(), err.Error())
				} else {
					t.Logf("✅ %s correctly failed with authentication error (exit code %d)", tc.name, ExitAuthenticationError)
				}
			} else {
				t.Errorf("Expected exitCodeError with ExitAuthenticationError exit code for %s, got: %v", tc.name, err)
			}

			// Check error message contains authentication-related text
			if !strings.Contains(err.Error(), "authentication") && !strings.Contains(err.Error(), "API key") {
				t.Logf("Note: %s error message may be acceptable: %s", tc.name, err.Error())
			}
		})
	}

	// Test all db commands
	for _, tc := range dbCommands {
		t.Run(tc.name, func(t *testing.T) {
			output, err := executeIntegrationCommand(tc.args...)

			// Should fail with authentication error
			if err == nil {
				t.Errorf("Expected %s to fail with authentication error when using invalid API key, but got output: %s", tc.name, output)
				return
			}

			// Check that it's an exitCodeError with the expected exit code
			if exitErr, ok := err.(interface{ ExitCode() int }); ok {
				expectedExitCode := ExitAuthenticationError
				expectedDescription := "authentication error"

				// db test-connection follows pg_isready conventions and uses exit code 3
				if tc.name == "db test-connection" {
					expectedExitCode = ExitInvalidParameters
					expectedDescription = "invalid parameters (pg_isready convention)"
				}

				if exitErr.ExitCode() != expectedExitCode {
					t.Errorf("Expected exit code %d (%s) for %s, got %d. Error: %s", expectedExitCode, expectedDescription, tc.name, exitErr.ExitCode(), err.Error())
				} else {
					t.Logf("✅ %s correctly failed with %s (exit code %d)", tc.name, expectedDescription, expectedExitCode)
				}
			} else {
				t.Errorf("Expected exitCodeError for %s, got: %v", tc.name, err)
			}

			// Check error message contains authentication-related text
			if !strings.Contains(err.Error(), "authentication") && !strings.Contains(err.Error(), "API key") {
				t.Logf("Note: %s error message may be acceptable: %s", tc.name, err.Error())
			}
		})
	}
}
