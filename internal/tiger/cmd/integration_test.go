package cmd

import (
	"bytes"
	"context"
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

	// Disable analytics for integration tests to avoid tracking test events
	os.Setenv("TIGER_ANALYTICS", "false")

	// Reset global config and viper to ensure test isolation
	config.ResetGlobalConfig()

	// Re-establish viper environment configuration after reset
	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

	// Set API URL in temporary config if integration URL is provided
	if apiURL := os.Getenv("TIGER_API_URL_INTEGRATION"); apiURL != "" {
		// Use a simple command execution without the full executeIntegrationCommand wrapper
		// to avoid circular dependencies during setup
		rootCmd, err := buildRootCmd(t.Context())
		if err != nil {
			t.Fatalf("Failed to build root command during setup: %v", err)
		}
		rootCmd.SetArgs([]string{"config", "set", "api_url", apiURL})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Failed to set integration API URL during setup: %v", err)
		}
	}

	t.Cleanup(func() {
		// Reset global config and viper first
		config.ResetGlobalConfig()
		// Clean up environment variables BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_ANALYTICS")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

// executeIntegrationCommand executes a CLI command for integration testing
func executeIntegrationCommand(ctx context.Context, args ...string) (string, error) {
	// Reset both global config and viper before each command execution
	// This ensures fresh config loading with proper flag precedence
	config.ResetGlobalConfig()

	// Re-establish viper environment configuration after reset
	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot, err := buildRootCmd(ctx)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err = testRoot.Execute()
	return buf.String(), err
}

// TestServiceLifecycleIntegration tests the complete authentication and service lifecycle:
// login -> status -> create -> get -> update-password -> delete -> logout
func TestServiceLifecycleIntegration(t *testing.T) {
	config.SetTestServiceName(t)
	// Check for required environment variables
	publicKey := os.Getenv("TIGER_PUBLIC_KEY_INTEGRATION")
	secretKey := os.Getenv("TIGER_SECRET_KEY_INTEGRATION")
	if publicKey == "" || secretKey == "" {
		t.Skip("Skipping integration test: TIGER_PUBLIC_KEY_INTEGRATION and TIGER_SECRET_KEY_INTEGRATION must be set")
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
		_, err := executeIntegrationCommand(t.Context(), "auth", "logout")
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
				t.Context(),
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
			t.Context(),
			"auth", "login",
			"--public-key", publicKey,
			"--secret-key", secretKey,
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

		output, err := executeIntegrationCommand(t.Context(), "auth", "status")
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
			t.Context(),
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
			t.Context(),
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
			t.Context(),
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
			t.Context(),
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
			t.Context(),
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
			t.Context(), "db", "connection-string", serviceID,
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
			t.Context(),
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

	// Track created roles for testing (used by CreateRole_CountRoles and CreateRole_DuplicateError tests)
	var createdRoles []string

	t.Run("CreateRole_Basic", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		roleName := fmt.Sprintf("integration_test_role_%d", time.Now().Unix())
		createdRoles = append(createdRoles, roleName)

		t.Logf("Creating basic role: %s", roleName)

		output, err := executeIntegrationCommand(
			t.Context(),
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
			t.Context(),
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

		t.Logf("Creating read-only role with inherited grants from tsdbadmin: %s", roleName)

		output, err := executeIntegrationCommand(
			t.Context(),
			"db", "create", "role", serviceID,
			"--name", roleName,
			"--from", "tsdbadmin",
			"--read-only", // Required when inheriting from tsdbadmin
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

		// Verify read_only flag in output (required for tsdbadmin inheritance)
		if readOnly, ok := result["read_only"].(bool); !ok || !readOnly {
			t.Errorf("Expected read_only=true in output, got: %v", result["read_only"])
		}

		t.Logf("✅ Successfully created read-only role with inherited grants: %s", roleName)
	})

	t.Run("CreateRole_ReadOnly", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		roleName := fmt.Sprintf("integration_test_role_readonly_%d", time.Now().Unix())
		createdRoles = append(createdRoles, roleName)

		t.Logf("Creating read-only role: %s", roleName)

		output, err := executeIntegrationCommand(
			t.Context(),
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
			t.Context(),
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

	t.Run("CreateRole_ReadOnlyWithInheritance", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		// Step 1: Create a base role with write privileges
		// (tsdbadmin automatically gets ADMIN OPTION on roles it creates)
		baseRoleName := fmt.Sprintf("integration_test_base_role_%d", time.Now().Unix())
		basePassword := fmt.Sprintf("base-password-%d", time.Now().Unix())
		createdRoles = append(createdRoles, baseRoleName)

		t.Logf("Creating base role with write privileges: %s", baseRoleName)

		output, err := executeIntegrationCommand(
			t.Context(),
			"db", "create", "role", serviceID,
			"--name", baseRoleName,
			"--password", basePassword,
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Failed to create base role: %v\nOutput: %s", err, output)
		}

		// Grant CREATE privilege on public schema to base role
		t.Logf("Granting CREATE privilege to %s", baseRoleName)
		_, err = executeIntegrationCommand(
			t.Context(),
			"db", "psql", serviceID,
			"--", "-c", fmt.Sprintf("GRANT CREATE ON SCHEMA public TO %s;", baseRoleName),
		)
		if err != nil {
			t.Fatalf("Failed to grant CREATE privilege: %v", err)
		}

		// Step 2: Use base role to create a table and insert data
		tableName := fmt.Sprintf("test_table_%d", time.Now().Unix())
		t.Logf("Creating table %s and inserting data as %s", tableName, baseRoleName)

		// Create table and insert data
		_, err = executeIntegrationCommand(
			t.Context(),
			"db", "psql", serviceID,
			"--role", baseRoleName,
			"--", "-c", fmt.Sprintf("CREATE TABLE %s (id INT, data TEXT);", tableName),
		)
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}

		_, err = executeIntegrationCommand(
			t.Context(),
			"db", "psql", serviceID,
			"--role", baseRoleName,
			"--", "-c", fmt.Sprintf("INSERT INTO %s VALUES (1, 'test data');", tableName),
		)
		if err != nil {
			t.Fatalf("Failed to insert data: %v", err)
		}

		// Step 3: Create read-only role that inherits from base role
		// (tsdbadmin can grant base_role since it has ADMIN OPTION)
		readOnlyRoleName := fmt.Sprintf("integration_test_readonly_inherited_%d", time.Now().Unix())
		readOnlyPassword := fmt.Sprintf("readonly-password-%d", time.Now().Unix())
		createdRoles = append(createdRoles, readOnlyRoleName)

		t.Logf("Creating read-only role with inheritance from %s: %s", baseRoleName, readOnlyRoleName)

		output, err = executeIntegrationCommand(
			t.Context(),
			"db", "create", "role", serviceID,
			"--name", readOnlyRoleName,
			"--password", readOnlyPassword,
			"--from", baseRoleName,
			"--read-only",
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Failed to create read-only role with inheritance: %v\nOutput: %s", err, output)
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

		// Verify from_roles in output
		fromRoles, ok := result["from_roles"].([]interface{})
		if !ok || len(fromRoles) == 0 {
			t.Error("Expected from_roles in output")
		} else if fromRoles[0] != baseRoleName {
			t.Errorf("Expected from_roles to contain '%s', got: %v", baseRoleName, fromRoles)
		}

		// Step 4: Verify read-only role can READ the data
		t.Logf("Verifying read-only role can read data from %s", tableName)
		readOutput, err := executeIntegrationCommand(
			t.Context(),
			"db", "psql", serviceID,
			"--role", readOnlyRoleName,
			"--", "-c", fmt.Sprintf("SELECT * FROM %s;", tableName),
		)
		if err != nil {
			t.Fatalf("Read-only role failed to read data: %v\nOutput: %s", err, readOutput)
		}

		if !strings.Contains(readOutput, "test data") {
			t.Errorf("Expected to read 'test data' from table, got: %s", readOutput)
		}
		t.Logf("✅ Read-only role successfully read data")

		// Step 5: Verify read-only role CANNOT WRITE
		t.Logf("Verifying read-only role cannot write to %s", tableName)
		writeOutput, err := executeIntegrationCommand(
			t.Context(),
			"db", "psql", serviceID,
			"--role", readOnlyRoleName,
			"--", "-c", fmt.Sprintf("INSERT INTO %s VALUES (2, 'should fail');", tableName),
		)

		// We EXPECT this to fail
		if err == nil {
			t.Errorf("Read-only role should NOT be able to write, but succeeded: %s", writeOutput)
		} else {
			// Verify it failed due to read-only enforcement
			if !strings.Contains(writeOutput, "read-only") && !strings.Contains(writeOutput, "permission denied") {
				t.Logf("Warning: Write failed but error message unexpected: %s", writeOutput)
			}
			t.Logf("✅ Read-only role correctly prevented from writing")
		}

		// Step 6: Clean up - drop the table
		t.Logf("Cleaning up table %s", tableName)
		_, _ = executeIntegrationCommand(
			t.Context(),
			"db", "psql", serviceID,
			"--role", baseRoleName,
			"--", "-c", fmt.Sprintf("DROP TABLE IF EXISTS %s;", tableName),
		)

		t.Logf("✅ Successfully verified read-only role with inheritance")
	})

	t.Run("CreateRole_AllOptions", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		roleName := fmt.Sprintf("integration_test_role_all_%d", time.Now().Unix())
		password := fmt.Sprintf("test-password-all-%d", time.Now().Unix())
		createdRoles = append(createdRoles, roleName)

		t.Logf("Creating role with all valid options (tsdbadmin + read-only): %s", roleName)

		// Note: --statement-timeout cannot be used with --from tsdbadmin due to permission restrictions
		output, err := executeIntegrationCommand(
			t.Context(),
			"db", "create", "role", serviceID,
			"--name", roleName,
			"--password", password,
			"--from", "tsdbadmin",
			"--read-only",
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
		// Note: statement_timeout not checked as it cannot be set with --from tsdbadmin

		// Verify from_roles in output
		fromRoles, ok := result["from_roles"].([]interface{})
		if !ok || len(fromRoles) == 0 {
			t.Error("Expected from_roles in output")
		} else if fromRoles[0] != "tsdbadmin" {
			t.Errorf("Expected from_roles to contain 'tsdbadmin', got: %v", fromRoles)
		}

		t.Logf("✅ Successfully created role with all valid options: %s", roleName)
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
				t.Context(),
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
			t.Context(),
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

	t.Run("StopService", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Stopping service: %s", serviceID)

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "stop", serviceID,
			"--wait-timeout", "10m", // Longer timeout for integration tests
		)

		if err != nil {
			t.Fatalf("Service stop failed: %v\nOutput: %s", err, output)
		} else {
			// Verify stop success message
			if !strings.Contains(output, "Stop request accepted") &&
				!strings.Contains(output, "stopped successfully") {
				t.Errorf("Expected stop success message, got: %s", output)
			}
			t.Logf("Service stop completed successfully")
		}
	})

	t.Run("VerifyServiceStopped", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Verifying service is stopped")

		output, err := executeIntegrationCommand(t.Context(), "service", "describe", serviceID, "--output", "json")
		if err != nil {
			t.Fatalf("Failed to describe service after stop: %v\nOutput: %s", err, output)
		}

		// Parse JSON to check status
		var service api.Service
		if err := json.Unmarshal([]byte(output), &service); err != nil {
			t.Fatalf("Failed to parse service JSON: %v", err)
		}

		var status string
		if service.Status != nil {
			status = string(*service.Status)
		}

		t.Logf("Service status after stop: %s", status)

		// The status should be PAUSED for stopped services
		if status != "PAUSED" {
			t.Logf("Warning: Expected service status to be PAUSED, got %s", status)
		} else {
			t.Logf("✅ Service is correctly in PAUSED state")
		}
	})

	t.Run("StopAlreadyStoppedService", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Attempting to stop already-stopped service: %s", serviceID)

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "stop", serviceID,
			"--wait-timeout", "10m",
		)

		// This should fail with an invalid parameters error (exit code 3)
		if err == nil {
			t.Errorf("Expected stop to fail for already-stopped service, but got output: %s", output)
		} else {
			// Check that it's an exitCodeError with ExitInvalidParameters
			if exitErr, ok := err.(interface{ ExitCode() int }); ok {
				if exitErr.ExitCode() != ExitInvalidParameters {
					t.Errorf("Expected exit code %d (ExitInvalidParameters) for already-stopped service, got %d. Error: %s", ExitInvalidParameters, exitErr.ExitCode(), err.Error())
				} else {
					t.Logf("✅ Stop correctly failed with invalid parameters error (exit code %d) for already-stopped service", ExitInvalidParameters)
				}
			} else {
				t.Errorf("Expected exitCodeError with ExitInvalidParameters exit code for already-stopped service, got: %v", err)
			}
		}
	})

	t.Run("StartService", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Starting service: %s", serviceID)

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "start", serviceID,
			"--wait-timeout", "10m", // Longer timeout for integration tests
		)

		if err != nil {
			t.Fatalf("Service start failed: %v\nOutput: %s", err, output)
		} else {
			// Verify start success message
			if !strings.Contains(output, "Start request accepted") &&
				!strings.Contains(output, "ready and running") {
				t.Errorf("Expected start success message, got: %s", output)
			}
			t.Logf("Service start completed successfully")
		}
	})

	t.Run("VerifyServiceStarted", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Verifying service is started")

		output, err := executeIntegrationCommand(t.Context(), "service", "describe", serviceID, "--output", "json")
		if err != nil {
			t.Fatalf("Failed to describe service after start: %v\nOutput: %s", err, output)
		}

		// Parse JSON to check status
		var service api.Service
		if err := json.Unmarshal([]byte(output), &service); err != nil {
			t.Fatalf("Failed to parse service JSON: %v", err)
		}

		var status string
		if service.Status != nil {
			status = string(*service.Status)
		}

		t.Logf("Service status after start: %s", status)

		// The status should be READY for started services
		if status != "READY" {
			t.Logf("Warning: Expected service status to be READY, got %s", status)
		} else {
			t.Logf("✅ Service is correctly in READY state")
		}
	})

	t.Run("StartAlreadyStartedService", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available from create test")
		}

		t.Logf("Attempting to start already-started service: %s", serviceID)

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "start", serviceID,
			"--wait-timeout", "10m",
		)

		// This should fail with an invalid parameters error (exit code 3)
		if err == nil {
			t.Errorf("Expected start to fail for already-started service, but got output: %s", output)
		} else {
			// Check that it's an exitCodeError with ExitInvalidParameters
			if exitErr, ok := err.(interface{ ExitCode() int }); ok {
				if exitErr.ExitCode() != ExitInvalidParameters {
					t.Errorf("Expected exit code %d (ExitInvalidParameters) for already-started service, got %d. Error: %s", ExitInvalidParameters, exitErr.ExitCode(), err.Error())
				} else {
					t.Logf("✅ Start correctly failed with invalid parameters error (exit code %d) for already-started service", ExitInvalidParameters)
				}
			} else {
				t.Errorf("Expected exitCodeError with ExitInvalidParameters exit code for already-started service, got: %v", err)
			}
		}
	})

	t.Run("DeleteService", func(t *testing.T) {
		if serviceID == "" {
			t.Skip("No service ID available for deletion")
		}

		t.Logf("Deleting service: %s", serviceID)

		output, err := executeIntegrationCommand(
			t.Context(),
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
			t.Context(),
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

		output, err := executeIntegrationCommand(t.Context(), "auth", "logout")
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

		output, err := executeIntegrationCommand(t.Context(), "auth", "status")
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

// TestServiceNotFoundIntegration tests that commands requiring service ID fail with correct exit code for non-existent services
func TestServiceNotFoundIntegration(t *testing.T) {
	// Check for required environment variables
	publicKey := os.Getenv("TIGER_PUBLIC_KEY_INTEGRATION")
	secretKey := os.Getenv("TIGER_SECRET_KEY_INTEGRATION")
	config.SetTestServiceName(t)

	if publicKey == "" || secretKey == "" {
		t.Skip("Skipping service not found test: TIGER_PUBLIC_KEY_INTEGRATION and TIGER_SECRET_KEY_INTEGRATION must be set")
	}

	// Set up isolated test environment with temporary config directory
	tmpDir := setupIntegrationTest(t)
	t.Logf("Using temporary config directory: %s", tmpDir)

	// Always logout at the end to clean up credentials
	defer func() {
		t.Logf("Cleaning up authentication")
		_, err := executeIntegrationCommand(t.Context(), "auth", "logout")
		if err != nil {
			t.Logf("Warning: Failed to logout: %v", err)
		}
	}()

	// Login first
	output, err := executeIntegrationCommand(
		t.Context(),
		"auth", "login",
		"--public-key", publicKey,
		"--secret-key", secretKey,
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
			name:             "service start",
			args:             []string{"service", "start", nonExistentServiceID},
			expectedExitCode: ExitServiceNotFound,
		},
		{
			name:             "service stop",
			args:             []string{"service", "stop", nonExistentServiceID},
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
			output, err := executeIntegrationCommand(t.Context(), tc.args...)

			if err == nil {
				t.Errorf("Expected %s to fail for non-existent service, but got output: %s", tc.name, output)
				return
			}

			// Verify error message indicates service doesn't exist (API returns various messages)
			errorMsg := strings.ToLower(err.Error())
			if !strings.Contains(errorMsg, "not found") &&
				!strings.Contains(errorMsg, "no entries") &&
				!strings.Contains(errorMsg, "no service") &&
				!strings.Contains(errorMsg, "could not be found") {
				t.Errorf("Expected error message indicating service doesn't exist for %s, got: %v", tc.name, err)
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
	existingServiceID := os.Getenv("TIGER_EXISTING_SERVICE_ID_INTEGRATION") // Optional: use existing service
	config.SetTestServiceName(t)

	if publicKey == "" || secretKey == "" {
		t.Skip("Skipping integration test: TIGER_PUBLIC_KEY_INTEGRATION and TIGER_SECRET_KEY_INTEGRATION must be set")
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
		_, err := executeIntegrationCommand(t.Context(), "auth", "logout")
		if err != nil {
			t.Logf("Warning: Failed to logout: %v", err)
		}
	}()

	t.Run("Login", func(t *testing.T) {
		t.Logf("Logging in for database tests")

		output, err := executeIntegrationCommand(
			t.Context(),
			"auth", "login",
			"--public-key", publicKey,
			"--secret-key", secretKey,
		)

		if err != nil {
			t.Fatalf("Login failed: %v\nOutput: %s", err, output)
		}

		t.Logf("Login successful for database tests")
	})

	t.Run("DatabaseTestConnection", func(t *testing.T) {
		t.Logf("Testing database connection for service: %s", existingServiceID)

		output, err := executeIntegrationCommand(
			t.Context(),
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
	config.SetTestServiceName(t)

	if publicKey == "" || secretKey == "" {
		t.Skip("Skipping authentication error integration test: TIGER_PUBLIC_KEY_INTEGRATION and TIGER_SECRET_KEY_INTEGRATION must be set")
	}

	// Set up isolated test environment with temporary config directory
	tmpDir := setupIntegrationTest(t)
	t.Logf("Using temporary config directory: %s", tmpDir)

	// Make sure we're logged out (this should always succeed or be a no-op)
	_, _ = executeIntegrationCommand(t.Context(), "auth", "logout")

	// Log in with invalid credentials to trigger authentication errors (401 response from server)
	invalidPublicKey := "invalid-public-key"
	invalidSecretKey := "invalid-secret-key"

	// Login with invalid credentials (this should fail during validation)
	loginOutput, loginErr := executeIntegrationCommand(t.Context(), "auth", "login",
		"--public-key", invalidPublicKey,
		"--secret-key", invalidSecretKey)
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
			args: []string{"service", "list"},
		},
		{
			name: "service get",
			args: []string{"service", "get", "non-existent-service"},
		},
		{
			name: "service create",
			args: []string{"service", "create", "--name", "test-service", "--no-wait"},
		},
		{
			name: "service update-password",
			args: []string{"service", "update-password", "non-existent-service", "--new-password", "test-pass"},
		},
		{
			name: "service delete",
			args: []string{"service", "delete", "non-existent-service", "--confirm", "--no-wait"},
		},
		{
			name: "service start",
			args: []string{"service", "start", "non-existent-service", "--no-wait"},
		},
		{
			name: "service stop",
			args: []string{"service", "stop", "non-existent-service", "--no-wait"},
		},
	}

	// Test db commands that should return authentication errors
	dbCommands := []struct {
		name string
		args []string
	}{
		{
			name: "db connection-string",
			args: []string{"db", "connection-string", "non-existent-service"},
		},
		{
			name: "db connect",
			args: []string{"db", "connect", "non-existent-service"},
		},
		// Note: db test-connection follows pg_isready conventions, so it uses exit code 3 (ExitInvalidParameters)
		// for authentication issues, not ExitAuthenticationError like other commands
		{
			name: "db test-connection",
			args: []string{"db", "test-connection", "non-existent-service"},
		},
	}

	// Test all service commands
	for _, tc := range serviceCommands {
		t.Run(tc.name, func(t *testing.T) {
			output, err := executeIntegrationCommand(t.Context(), tc.args...)

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
			output, err := executeIntegrationCommand(t.Context(), tc.args...)

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

// TestServiceForkIntegration tests forking a service with --now strategy and validates data is correctly copied
func TestServiceForkIntegration(t *testing.T) {
	config.SetTestServiceName(t)
	// Check for required environment variables
	publicKey := os.Getenv("TIGER_PUBLIC_KEY_INTEGRATION")
	secretKey := os.Getenv("TIGER_SECRET_KEY_INTEGRATION")
	if publicKey == "" || secretKey == "" {
		t.Skip("Skipping integration test: TIGER_PUBLIC_KEY_INTEGRATION and TIGER_SECRET_KEY_INTEGRATION must be set")
	}

	// Set up isolated test environment with temporary config directory
	tmpDir := setupIntegrationTest(t)
	t.Logf("Using temporary config directory: %s", tmpDir)

	// Generate unique names to avoid conflicts
	timestamp := time.Now().Unix()
	sourceServiceName := fmt.Sprintf("integration-fork-source-%d", timestamp)
	tableName := fmt.Sprintf("fork_test_data_%d", timestamp)

	var sourceServiceID string
	var forkedServiceID string

	// Always logout at the end to clean up credentials
	defer func() {
		t.Logf("Cleaning up authentication")
		_, err := executeIntegrationCommand(t.Context(), "auth", "logout")
		if err != nil {
			t.Logf("Warning: Failed to logout: %v", err)
		}
	}()

	// Cleanup function to ensure source service is deleted
	defer func() {
		if sourceServiceID != "" {
			t.Logf("Cleaning up source service: %s", sourceServiceID)
			_, err := executeIntegrationCommand(
				t.Context(),
				"service", "delete", sourceServiceID,
				"--confirm",
				"--wait-timeout", "5m",
			)
			if err != nil {
				t.Logf("Warning: Failed to cleanup source service %s: %v", sourceServiceID, err)
			}
		}
	}()

	// Cleanup function to ensure forked service is deleted
	defer func() {
		if forkedServiceID != "" {
			t.Logf("Cleaning up forked service: %s", forkedServiceID)
			_, err := executeIntegrationCommand(
				t.Context(),
				"service", "delete", forkedServiceID,
				"--confirm",
				"--wait-timeout", "5m",
			)
			if err != nil {
				t.Logf("Warning: Failed to cleanup forked service %s: %v", forkedServiceID, err)
			}
		}
	}()

	t.Run("Login", func(t *testing.T) {
		t.Logf("Logging in with public key: %s", publicKey[:8]+"...") // Only show first 8 chars

		output, err := executeIntegrationCommand(
			t.Context(),
			"auth", "login",
			"--public-key", publicKey,
			"--secret-key", secretKey,
		)

		if err != nil {
			t.Fatalf("Login failed: %v\nOutput: %s", err, output)
		}

		t.Logf("Login successful")
	})

	t.Run("CreateSourceService", func(t *testing.T) {
		t.Logf("Creating source service: %s", sourceServiceName)

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "create",
			"--name", sourceServiceName,
			"--cpu", "shared",
			"--wait-timeout", "15m",
			"--no-set-default",
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Source service creation failed: %v\nOutput: %s", err, output)
		}

		extractedServiceID := extractServiceIDFromCreateOutput(t, output)
		if extractedServiceID == "" {
			t.Fatalf("Could not extract source service ID from create output: %s", output)
		}

		sourceServiceID = extractedServiceID
		t.Logf("Created source service with ID: %s", sourceServiceID)
	})

	t.Run("InsertTestData", func(t *testing.T) {
		if sourceServiceID == "" {
			t.Skip("No source service ID available")
		}

		t.Logf("Creating test table: %s", tableName)

		// Create table
		output, err := executeIntegrationCommand(
			t.Context(),
			"db", "psql", sourceServiceID,
			"--", "-c", fmt.Sprintf("CREATE TABLE %s (id INT PRIMARY KEY, data TEXT, created_at TIMESTAMP DEFAULT NOW());", tableName),
		)

		if err != nil {
			t.Fatalf("Failed to create test table: %v\nOutput: %s", err, output)
		}

		t.Logf("Inserting test data into table: %s", tableName)

		// Insert test data
		output, err = executeIntegrationCommand(
			t.Context(),
			"db", "psql", sourceServiceID,
			"--", "-c", fmt.Sprintf("INSERT INTO %s (id, data) VALUES (1, 'test-row-1'), (2, 'test-row-2'), (3, 'test-row-3');", tableName),
		)

		if err != nil {
			t.Fatalf("Failed to insert test data: %v\nOutput: %s", err, output)
		}

		t.Logf("✅ Test data inserted successfully")
	})

	t.Run("VerifySourceData", func(t *testing.T) {
		if sourceServiceID == "" {
			t.Skip("No source service ID available")
		}

		t.Logf("Verifying test data in source service")

		output, err := executeIntegrationCommand(
			t.Context(),
			"db", "psql", sourceServiceID,
			"--", "-c", fmt.Sprintf("SELECT * FROM %s ORDER BY id;", tableName),
		)

		if err != nil {
			t.Fatalf("Failed to query test data: %v\nOutput: %s", err, output)
		}

		// Verify all three rows are present
		if !strings.Contains(output, "test-row-1") {
			t.Errorf("Expected 'test-row-1' in output, got: %s", output)
		}
		if !strings.Contains(output, "test-row-2") {
			t.Errorf("Expected 'test-row-2' in output, got: %s", output)
		}
		if !strings.Contains(output, "test-row-3") {
			t.Errorf("Expected 'test-row-3' in output, got: %s", output)
		}

		t.Logf("✅ Source data verified: 3 rows present")
	})

	t.Run("ForkService_LastSnapshot_NoBackupsYet", func(t *testing.T) {
		if sourceServiceID == "" {
			t.Skip("No source service ID available")
		}

		t.Logf("Attempting to fork with --last-snapshot (should fail - no backups yet)")

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "fork", sourceServiceID,
			"--last-snapshot",
			"--wait-timeout", "15m",
			"--no-set-default",
			"--output", "json",
		)

		// We expect this to fail
		if err == nil {
			t.Errorf("Expected fork with --last-snapshot to fail when no backups exist, but it succeeded")
		} else {
			// Verify the error message indicates no backups/snapshots available
			if !strings.Contains(err.Error(), "doesn't yet have any backups or snapshots available") &&
				!strings.Contains(output, "doesn't yet have any backups or snapshots available") {
				t.Errorf("Expected error about no backups/snapshots, got: %v\nOutput: %s", err, output)
			} else {
				t.Logf("✅ Fork with --last-snapshot correctly failed: no backups available yet")
			}
		}
	})

	t.Run("ForkService_Now", func(t *testing.T) {
		if sourceServiceID == "" {
			t.Skip("No source service ID available")
		}

		t.Logf("Forking service: %s with --now strategy", sourceServiceID)

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "fork", sourceServiceID,
			"--now",
			"--wait-timeout", "15m",
			"--no-set-default",
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Service fork failed: %v\nOutput: %s", err, output)
		}

		extractedServiceID := extractServiceIDFromCreateOutput(t, output)
		if extractedServiceID == "" {
			t.Fatalf("Could not extract forked service ID from fork output: %s", output)
		}

		forkedServiceID = extractedServiceID
		t.Logf("✅ Created forked service with ID: %s", forkedServiceID)
	})

	t.Run("VerifyForkedData", func(t *testing.T) {
		if forkedServiceID == "" {
			t.Skip("No forked service ID available")
		}

		t.Logf("Verifying test data in forked service")

		output, err := executeIntegrationCommand(
			t.Context(),
			"db", "psql", forkedServiceID,
			"--", "-c", fmt.Sprintf("SELECT * FROM %s ORDER BY id;", tableName),
		)

		if err != nil {
			t.Fatalf("Failed to query forked service data: %v\nOutput: %s", err, output)
		}

		// Verify all three rows are present in fork
		if !strings.Contains(output, "test-row-1") {
			t.Errorf("Expected 'test-row-1' in forked service output, got: %s", output)
		}
		if !strings.Contains(output, "test-row-2") {
			t.Errorf("Expected 'test-row-2' in forked service output, got: %s", output)
		}
		if !strings.Contains(output, "test-row-3") {
			t.Errorf("Expected 'test-row-3' in forked service output, got: %s", output)
		}

		t.Logf("✅ Forked data verified: 3 rows present matching source")
	})

	t.Run("VerifyDataIndependence", func(t *testing.T) {
		if sourceServiceID == "" || forkedServiceID == "" {
			t.Skip("Source or forked service ID not available")
		}

		t.Logf("Verifying data independence between source and fork")

		// Insert new data in forked service
		t.Logf("Inserting new row in forked service")
		output, err := executeIntegrationCommand(
			t.Context(),
			"db", "psql", forkedServiceID,
			"--", "-c", fmt.Sprintf("INSERT INTO %s (id, data) VALUES (4, 'fork-only-row');", tableName),
		)

		if err != nil {
			t.Fatalf("Failed to insert data in fork: %v\nOutput: %s", err, output)
		}

		// Verify fork has 4 rows
		t.Logf("Verifying fork has 4 rows")
		output, err = executeIntegrationCommand(
			t.Context(),
			"db", "psql", forkedServiceID,
			"--", "-c", fmt.Sprintf("SELECT COUNT(*) FROM %s;", tableName),
		)

		if err != nil {
			t.Fatalf("Failed to count rows in fork: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(output, "4") {
			t.Errorf("Expected 4 rows in fork after insert, got: %s", output)
		}

		// Verify source still has 3 rows (unchanged)
		t.Logf("Verifying source still has 3 rows (unchanged)")
		output, err = executeIntegrationCommand(
			t.Context(),
			"db", "psql", sourceServiceID,
			"--", "-c", fmt.Sprintf("SELECT COUNT(*) FROM %s;", tableName),
		)

		if err != nil {
			t.Fatalf("Failed to count rows in source: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(output, "3") {
			t.Errorf("Expected 3 rows in source (unchanged), got: %s", output)
		}

		// Verify source doesn't have fork-only row
		output, err = executeIntegrationCommand(
			t.Context(),
			"db", "psql", sourceServiceID,
			"--", "-c", fmt.Sprintf("SELECT * FROM %s WHERE data = 'fork-only-row';", tableName),
		)

		if err != nil {
			t.Fatalf("Failed to query source for fork-only row: %v\nOutput: %s", err, output)
		}

		// The output should show "0 rows" or similar since the row shouldn't exist
		if strings.Contains(output, "fork-only-row") {
			t.Errorf("Source service should not contain fork-only row, but got: %s", output)
		}

		t.Logf("✅ Data independence verified: fork and source are truly independent")
	})

	t.Run("DeleteForkedService_Now", func(t *testing.T) {
		if forkedServiceID == "" {
			t.Skip("No forked service ID available")
		}

		t.Logf("Deleting --now forked service: %s", forkedServiceID)

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "delete", forkedServiceID,
			"--confirm",
			"--wait-timeout", "10m",
		)

		if err != nil {
			t.Fatalf("Forked service deletion failed: %v\nOutput: %s", err, output)
		}

		// Clear forkedServiceID so cleanup doesn't try to delete again
		forkedServiceID = ""
		t.Logf("✅ --now forked service deleted successfully")
	})

	t.Run("ForkService_LastSnapshot_Success", func(t *testing.T) {
		if sourceServiceID == "" {
			t.Skip("No source service ID available")
		}
		waitDuration := 120 * time.Second
		t.Logf("Waiting %v for snapshot to become available for --last-snapshot fork...", waitDuration)
		time.Sleep(waitDuration)

		t.Logf("Forking service with --last-snapshot (should succeed now - snapshot from --now fork exists)")

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "fork", sourceServiceID,
			"--last-snapshot",
			"--wait-timeout", "15m",
			"--no-set-default",
			"--output", "json",
		)

		if err != nil {
			t.Fatalf("Service fork with --last-snapshot failed: %v\nOutput: %s", err, output)
		}

		extractedServiceID := extractServiceIDFromCreateOutput(t, output)
		if extractedServiceID == "" {
			t.Fatalf("Could not extract forked service ID from fork output: %s", output)
		}

		forkedServiceID = extractedServiceID
		t.Logf("✅ Created --last-snapshot forked service with ID: %s", forkedServiceID)
	})

	t.Run("VerifyLastSnapshotForkWorks", func(t *testing.T) {
		if forkedServiceID == "" {
			t.Skip("No forked service ID available")
		}

		t.Logf("Verifying --last-snapshot forked service is functional")

		output, err := executeIntegrationCommand(
			t.Context(),
			"db", "psql", forkedServiceID,
			"--", "-c", "SELECT 1 as test;",
		)

		if err != nil {
			t.Fatalf("Failed to query --last-snapshot forked service: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(output, "1") {
			t.Errorf("Expected to see '1' in query output, got: %s", output)
		}

		t.Logf("✅ --last-snapshot forked service is functional")
	})

	t.Run("DeleteForkedService_LastSnapshot", func(t *testing.T) {
		if forkedServiceID == "" {
			t.Skip("No forked service ID available")
		}

		t.Logf("Deleting --last-snapshot forked service: %s", forkedServiceID)

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "delete", forkedServiceID,
			"--confirm",
			"--wait-timeout", "10m",
		)

		if err != nil {
			t.Fatalf("Forked service deletion failed: %v\nOutput: %s", err, output)
		}

		// Clear forkedServiceID so cleanup doesn't try to delete again
		forkedServiceID = ""
		t.Logf("✅ --last-snapshot forked service deleted successfully")
	})

	t.Run("DeleteSourceService", func(t *testing.T) {
		if sourceServiceID == "" {
			t.Skip("No source service ID available")
		}

		t.Logf("Deleting source service: %s", sourceServiceID)

		output, err := executeIntegrationCommand(
			t.Context(),
			"service", "delete", sourceServiceID,
			"--confirm",
			"--wait-timeout", "10m",
		)

		if err != nil {
			t.Fatalf("Source service deletion failed: %v\nOutput: %s", err, output)
		}

		// Clear sourceServiceID so cleanup doesn't try to delete again
		sourceServiceID = ""
		t.Logf("✅ Source service deleted successfully")
	})

	t.Run("Logout", func(t *testing.T) {
		t.Logf("Logging out")

		output, err := executeIntegrationCommand(
			t.Context(),
			"auth", "logout",
		)

		if err != nil {
			t.Fatalf("Logout failed: %v\nOutput: %s", err, output)
		}

		t.Logf("Logout successful")
	})
}
