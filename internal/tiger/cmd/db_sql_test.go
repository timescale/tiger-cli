package cmd

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBuildCreateRoleSQL_NoFromRoles(t *testing.T) {
	sql := buildCreateRoleSQL("test_role", "'my_password'", nil)
	expected := `CREATE ROLE "test_role" WITH LOGIN PASSWORD 'my_password'`

	if sql != expected {
		t.Errorf("Expected SQL:\n%s\nGot:\n%s", expected, sql)
	}

	// Verify password is a quoted literal
	if !strings.Contains(sql, "'my_password'") {
		t.Error("Expected SQL to use quoted password literal")
	}
}

func TestBuildCreateRoleSQL_SingleFromRole(t *testing.T) {
	sql := buildCreateRoleSQL("ai_analyst", "'test_pass'", []string{"app_role"})
	expected := `CREATE ROLE "ai_analyst" WITH LOGIN PASSWORD 'test_pass' IN ROLE "app_role"`

	if sql != expected {
		t.Errorf("Expected SQL:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBuildCreateRoleSQL_MultipleFromRoles(t *testing.T) {
	sql := buildCreateRoleSQL("ai_analyst", "'test_pass'", []string{"app_role", "readonly_role", "reporting_role"})
	expected := `CREATE ROLE "ai_analyst" WITH LOGIN PASSWORD 'test_pass' IN ROLE "app_role", "readonly_role", "reporting_role"`

	if sql != expected {
		t.Errorf("Expected SQL:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBuildCreateRoleSQL_SQLInjectionPrevention(t *testing.T) {
	// Attempt SQL injection in role name
	maliciousRoleName := `test"; DROP TABLE users; --`
	sql := buildCreateRoleSQL(maliciousRoleName, "'test_pass'", nil)

	// pgx.Identifier.Sanitize() properly escapes quotes by doubling them
	// The dangerous content should be inside a quoted identifier, making it safe
	// Expected output: CREATE ROLE "test""; DROP TABLE users; --" WITH LOGIN PASSWORD 'test_pass'
	// The doubled quote ("") escapes the quote character in PostgreSQL
	if !strings.Contains(sql, `"test""; DROP TABLE users; --"`) {
		t.Errorf("Expected malicious content to be properly quoted and escaped, got: %s", sql)
	}

	// Verify the SQL structure remains correct
	if !strings.HasPrefix(sql, "CREATE ROLE") {
		t.Error("SQL structure was corrupted by malicious input")
	}
	if !strings.Contains(sql, "WITH LOGIN PASSWORD 'test_pass'") {
		t.Error("SQL structure was corrupted - missing WITH LOGIN PASSWORD")
	}

	// Attempt SQL injection in fromRoles
	maliciousFromRole := `admin"; DROP TABLE users; --`
	sql2 := buildCreateRoleSQL("safe_role", "'test_pass'", []string{maliciousFromRole})

	// The malicious fromRole should also be properly escaped
	if !strings.Contains(sql2, `"admin""; DROP TABLE users; --"`) {
		t.Errorf("Expected malicious fromRole to be properly quoted and escaped, got: %s", sql2)
	}

	// Verify the SQL structure remains correct
	if !strings.Contains(sql2, "IN ROLE") {
		t.Error("SQL structure was corrupted - missing IN ROLE clause")
	}
}

func TestBuildCreateRoleSQL_SpecialCharactersInRoleName(t *testing.T) {
	testCases := []struct {
		name         string
		roleName     string
		expectQuoted bool
	}{
		{
			name:         "Simple alphanumeric",
			roleName:     "simple_role",
			expectQuoted: true, // pgx always quotes identifiers via Sanitize()
		},
		{
			name:         "Role with spaces",
			roleName:     "role with spaces",
			expectQuoted: true,
		},
		{
			name:         "Role with special chars",
			roleName:     "role-with-dashes",
			expectQuoted: true,
		},
		{
			name:         "Role with uppercase",
			roleName:     "MyRole",
			expectQuoted: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sql := buildCreateRoleSQL(tc.roleName, "'test_pass'", nil)

			// All identifiers should be quoted when using pgx.Identifier.Sanitize()
			if tc.expectQuoted && !strings.Contains(sql, `"`) {
				t.Errorf("Expected role name to be quoted, got: %s", sql)
			}

			// Verify the SQL structure is correct
			if !strings.HasPrefix(sql, "CREATE ROLE") {
				t.Errorf("Expected SQL to start with CREATE ROLE, got: %s", sql)
			}
			if !strings.Contains(sql, "WITH LOGIN PASSWORD 'test_pass'") {
				t.Errorf("Expected SQL to contain WITH LOGIN PASSWORD 'test_pass', got: %s", sql)
			}
		})
	}
}

func TestBuildReadOnlyAlterSQL(t *testing.T) {
	sql := buildReadOnlyAlterSQL("ai_analyst")
	expected := `ALTER ROLE "ai_analyst" SET tsdb_admin.read_only_role = true`

	if sql != expected {
		t.Errorf("Expected SQL:\n%s\nGot:\n%s", expected, sql)
	}
}

func TestBuildReadOnlyAlterSQL_SQLInjectionPrevention(t *testing.T) {
	maliciousRoleName := `test"; DROP TABLE users; --`
	sql := buildReadOnlyAlterSQL(maliciousRoleName)

	// The malicious content should be properly quoted and escaped
	if !strings.Contains(sql, `"test""; DROP TABLE users; --"`) {
		t.Errorf("Expected malicious content to be properly quoted and escaped, got: %s", sql)
	}

	// Verify the SQL structure remains correct
	if !strings.HasPrefix(sql, "ALTER ROLE") {
		t.Error("SQL structure was corrupted by malicious input")
	}

	// Verify the GUC name is correct
	if !strings.Contains(sql, "tsdb_admin.read_only_role") {
		t.Error("Expected SQL to contain tsdb_admin.read_only_role GUC")
	}

	// Verify the value is set to true
	if !strings.Contains(sql, "= true") {
		t.Error("Expected SQL to set value to true")
	}
}

func TestBuildStatementTimeoutAlterSQL(t *testing.T) {
	testCases := []struct {
		name            string
		timeout         time.Duration
		expectedTimeout int64 // milliseconds
	}{
		{
			name:            "30 seconds",
			timeout:         30 * time.Second,
			expectedTimeout: 30000,
		},
		{
			name:            "5 minutes",
			timeout:         5 * time.Minute,
			expectedTimeout: 300000,
		},
		{
			name:            "1 hour",
			timeout:         1 * time.Hour,
			expectedTimeout: 3600000,
		},
		{
			name:            "1.5 seconds",
			timeout:         1500 * time.Millisecond,
			expectedTimeout: 1500,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sql := buildStatementTimeoutAlterSQL("test_role", tc.timeout)

			// Check structure
			if !strings.HasPrefix(sql, `ALTER ROLE "test_role" SET statement_timeout =`) {
				t.Errorf("Expected SQL to start with ALTER ROLE \"test_role\" SET statement_timeout =, got: %s", sql)
			}

			// Check timeout value is present (convert int64 to string)
			if !strings.Contains(sql, fmt.Sprintf("%d", tc.expectedTimeout)) {
				t.Errorf("Expected SQL to contain timeout value %d, got: %s", tc.expectedTimeout, sql)
			}
		})
	}
}

func TestBuildStatementTimeoutAlterSQL_SQLInjectionPrevention(t *testing.T) {
	maliciousRoleName := `test"; DROP TABLE users; --`
	sql := buildStatementTimeoutAlterSQL(maliciousRoleName, 30*time.Second)

	// The malicious content should be properly quoted and escaped
	if !strings.Contains(sql, `"test""; DROP TABLE users; --"`) {
		t.Errorf("Expected malicious content to be properly quoted and escaped, got: %s", sql)
	}

	// Verify the SQL structure remains correct
	if !strings.HasPrefix(sql, "ALTER ROLE") {
		t.Error("SQL structure was corrupted by malicious input")
	}

	// Verify the GUC name is correct
	if !strings.Contains(sql, "statement_timeout") {
		t.Error("Expected SQL to contain statement_timeout GUC")
	}
}

func TestBuildCreateRoleSQL_EmptyFromRoles(t *testing.T) {
	// Empty slice should be treated the same as nil
	sql := buildCreateRoleSQL("test_role", "'test_pass'", []string{})
	expected := `CREATE ROLE "test_role" WITH LOGIN PASSWORD 'test_pass'`

	if sql != expected {
		t.Errorf("Expected SQL:\n%s\nGot:\n%s", expected, sql)
	}

	// Should not contain IN ROLE clause
	if strings.Contains(sql, "IN ROLE") {
		t.Error("Expected SQL to not contain IN ROLE clause for empty fromRoles")
	}
}

func TestBuildCreateRoleSQL_CasePreservation(t *testing.T) {
	// PostgreSQL role names are case-sensitive when quoted
	// pgx.Identifier.Sanitize() preserves case by quoting
	sql := buildCreateRoleSQL("MixedCase_Role", "'test_pass'", nil)

	// The role name should be preserved with its original case
	if !strings.Contains(sql, `"MixedCase_Role"`) {
		t.Errorf("Expected role name case to be preserved, got: %s", sql)
	}
}
