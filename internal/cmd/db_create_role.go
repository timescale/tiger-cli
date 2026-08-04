package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

func buildDbCreateRoleCmd() *cobra.Command {
	var roleName string
	var readOnly bool
	var fromRoles []string
	var statementTimeout time.Duration
	var passwordFlag string
	var output string

	cmd := &cobra.Command{
		Use:   "role [service-id]",
		Short: "Create a new database role",
		Long: `Create a new database role with optional read-only enforcement.

The service ID can be provided as an argument or will use the default service
from your configuration. A read replica ID is rejected, since replicas are
read-only; create the role on the primary instead.

By default, a secure random password is auto-generated for the new role. You can:
- Provide an explicit password with --password=<value>
- Use TIGER_NEW_PASSWORD environment variable
- Let it auto-generate (default)

The password is saved according to your --password-storage setting (keyring, pgpass, or none).

Read-Only Mode for AI Agents:
The --read-only flag enables permanent read-only enforcement at the PostgreSQL level
using the tsdb_admin.read_only_role extension setting. This is designed to provide
safe database access for AI agents and automated tools that need to read production
data without risk of modification.

Examples:
  # Create a role with global database access (uses default service, auto-generates password)
  tiger db create role --name ai_analyst --from tsdbadmin

  # Create a role for specific service
  tiger db create role svc-12345 --name ai_analyst

  # Create a read-only role
  tiger db create role --name ai_analyst --read-only

  # Create a read-only role with same grants as another role
  tiger db create role --name ai_analyst --read-only --from app_role

  # Create a read-only role inheriting from multiple roles
  tiger db create role --name ai_analyst --read-only --from app_role --from readonly_role

  # Create a read-only role with statement timeout
  tiger db create role --name ai_analyst --read-only --statement-timeout 30s

  # Create a role with specific password
  tiger db create role --name ai_analyst --password=my-secure-password

  # Create a role with password from environment variable
  TIGER_NEW_PASSWORD=my-secure-password tiger db create role --name ai_analyst

Technical Details:
This command executes PostgreSQL statements in a transaction to create and configure the role.

CREATE ROLE Options Used:
  - LOGIN: Always enabled to allow the role to connect
  - PASSWORD: Always set (from flag, env var, or auto-generated)
  - IN ROLE: Added when --from flag is provided to inherit grants from existing roles

PostgreSQL Configuration Parameters That May Be Set:
  - tsdb_admin.read_only_role: Set to 'true' when --read-only flag is used
    (enforces permanent read-only mode for the role)
  - statement_timeout: Set when --statement-timeout flag is provided
    (kills queries that exceed the specified duration, in milliseconds)`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate arguments
			if roleName == "" {
				return fmt.Errorf("--name is required")
			}

			cfg, err := common.LoadConfig(cmd.Context(), cmd.Flags())
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}

			// Get service details
			service, err := getServiceDetailsFunc(cmd, cfg, args)
			if err != nil {
				return err
			}

			// A read replica is read-only, so a role can't be created there.
			if common.IsReadReplica(service) {
				return fmt.Errorf("%q is a read replica; create the role on its primary service %q instead",
					util.Deref(service.ServiceId), util.DerefStr(service.ForkedFrom.ServiceId))
			}

			// Get password
			rolePassword, err := getPasswordForRole(passwordFlag)
			if err != nil {
				return fmt.Errorf("failed to determine password: %w", err)
			}

			// Build connection string
			details, err := common.GetConnectionDetails(cfg.Config, service, common.ConnectionDetailsOptions{
				Pooled:       false,
				Role:         "tsdbadmin", // Use admin role to create new roles
				WithPassword: true,
			})
			if err != nil {
				return fmt.Errorf("failed to build connection string: %w", err)
			}

			// Connect to database
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			conn, err := pgx.Connect(ctx, details.String())
			if err != nil {
				return fmt.Errorf("failed to connect to database: %w", err)
			}
			defer conn.Close(ctx)

			// Create the role with all options in a transaction
			if err := createRoleWithOptions(ctx, conn, roleName, rolePassword, readOnly, statementTimeout, fromRoles); err != nil {
				return fmt.Errorf("failed to create role: %w", err)
			}

			// Save password to storage with the new role name
			result, err := common.SavePasswordWithResult(cfg.Config, service, rolePassword, roleName)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  Warning: %s\n", result.Message)
			} else if !result.Success {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  Warning: %s\n", result.Message)
			}

			// Output result in requested format
			return outputCreateRoleResult(cmd, roleName, readOnly, statementTimeout, fromRoles, cfg.Output)
		},
	}

	// Add flags
	cmd.Flags().StringVar(&roleName, "name", "", "Role name to create (required)")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Enable permanent read-only enforcement via tsdb_admin.read_only_role")
	cmd.Flags().StringSliceVar(&fromRoles, "from", []string{}, "Roles to inherit grants from (e.g., --from app_role --from readonly_role or --from app_role,readonly_role)")
	cmd.Flags().DurationVar(&statementTimeout, "statement-timeout", 0, "Set statement timeout for the role (e.g., 30s, 5m)")
	cmd.Flags().StringVar(&passwordFlag, "password", "", "Password for the role. If not provided, checks TIGER_NEW_PASSWORD environment variable, otherwise auto-generates a secure random password.")
	cmd.Flags().VarP((*outputFlag)(&output), "output", "o", "output format (json, yaml, table)")

	cmd.MarkFlagRequired("name")

	return cmd
}

// buildCreateRoleSQL generates the CREATE ROLE SQL statement with LOGIN, PASSWORD, and optional IN ROLE clause
func buildCreateRoleSQL(roleName string, quotedPassword string, fromRoles []string) string {
	sanitizedRoleName := pgx.Identifier{roleName}.Sanitize()
	createSQL := fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD %s", sanitizedRoleName, quotedPassword)

	// Add IN ROLE clause if fromRoles is specified
	// IN ROLE adds the new role as a member of existing roles (equivalent to GRANT existing_role TO new_role)
	if len(fromRoles) > 0 {
		var sanitizedRoles []string
		for _, role := range fromRoles {
			sanitizedRoles = append(sanitizedRoles, pgx.Identifier{role}.Sanitize())
		}
		createSQL += " IN ROLE " + strings.Join(sanitizedRoles, ", ")
	}

	return createSQL
}

// buildReadOnlyAlterSQL generates the ALTER ROLE SQL statement for read-only enforcement
func buildReadOnlyAlterSQL(roleName string) string {
	sanitizedRoleName := pgx.Identifier{roleName}.Sanitize()
	return fmt.Sprintf("ALTER ROLE %s SET tsdb_admin.read_only_role = true", sanitizedRoleName)
}

// buildStatementTimeoutAlterSQL generates the ALTER ROLE SQL statement for statement timeout configuration
func buildStatementTimeoutAlterSQL(roleName string, timeout time.Duration) string {
	sanitizedRoleName := pgx.Identifier{roleName}.Sanitize()
	timeoutMs := timeout.Milliseconds()
	return fmt.Sprintf("ALTER ROLE %s SET statement_timeout = %d", sanitizedRoleName, timeoutMs)
}

// createRoleWithOptions creates a new PostgreSQL role with all specified options in a single transaction
func createRoleWithOptions(ctx context.Context, conn *pgx.Conn, roleName, rolePassword string, readOnly bool, statementTimeout time.Duration, fromRoles []string) error {
	// Begin transaction for atomic operation
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check if tsdbadmin is in the fromRoles list
	hasTsdbadmin := false
	var otherRoles []string
	for _, role := range fromRoles {
		if role == "tsdbadmin" {
			hasTsdbadmin = true
		} else {
			otherRoles = append(otherRoles, role)
		}
	}

	// If tsdbadmin is requested, use special TimescaleDB Cloud functions
	if hasTsdbadmin {
		// Enforce read-only requirement when inheriting from tsdbadmin
		if !readOnly {
			return fmt.Errorf("roles inheriting from tsdbadmin must be read-only (use --read-only flag)")
		}

		// Cannot set statement_timeout on roles created with create_bare_readonly_role
		// due to permission restrictions on altering special roles
		if statementTimeout > 0 {
			return fmt.Errorf("cannot use --statement-timeout with --from tsdbadmin (permission denied to alter special roles)")
		}

		// Use timescale_functions.create_bare_readonly_role to create the role
		// This function creates a read-only role that can inherit tsdbadmin privileges
		if _, err := tx.Exec(ctx, "SELECT timescale_functions.create_bare_readonly_role($1, $2)",
			roleName, rolePassword); err != nil {
			return fmt.Errorf("failed to create role with create_bare_readonly_role: %w", err)
		}

		// Grant tsdbadmin privileges using the special function
		if _, err := tx.Exec(ctx, "SELECT timescale_functions.grant_tsdbadmin_to_role($1)",
			roleName); err != nil {
			return fmt.Errorf("failed to grant tsdbadmin privileges: %w", err)
		}

		// Grant any other roles (besides tsdbadmin) if specified
		// This is necessary because the special functions don't support IN ROLE clause
		for _, role := range otherRoles {
			grantSQL := fmt.Sprintf("GRANT %s TO %s",
				pgx.Identifier{role}.Sanitize(),
				pgx.Identifier{roleName}.Sanitize())
			if _, err := tx.Exec(ctx, grantSQL); err != nil {
				return fmt.Errorf("failed to grant role %s: %w", role, err)
			}
		}
	} else {
		// Use standard CREATE ROLE for non-tsdbadmin cases
		// Fail if password contains a single quote (we don't support escaping)
		if strings.Contains(rolePassword, "'") {
			return fmt.Errorf("password cannot contain single quotes")
		}
		// Wrap password in single quotes for SQL literal
		quotedPassword := "'" + rolePassword + "'"
		// IN ROLE clause handles all role grants, so no need for separate GRANT statements
		createSQL := buildCreateRoleSQL(roleName, quotedPassword, fromRoles)
		if _, err := tx.Exec(ctx, createSQL); err != nil {
			return fmt.Errorf("failed to create role: %w", err)
		}

		// Configure read-only mode if requested
		if readOnly {
			alterSQL := buildReadOnlyAlterSQL(roleName)
			if _, err := tx.Exec(ctx, alterSQL); err != nil {
				return fmt.Errorf("failed to configure read-only mode: %w", err)
			}
		}
	}

	// Set statement timeout if requested
	if statementTimeout > 0 {
		alterSQL := buildStatementTimeoutAlterSQL(roleName, statementTimeout)
		if _, err := tx.Exec(ctx, alterSQL); err != nil {
			return fmt.Errorf("failed to set statement timeout: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// getPasswordForRole determines the password based on flags and environment
func getPasswordForRole(passwordFlag string) (string, error) {
	// Priority order:
	// 1. Explicit password value from --password flag
	// 2. TIGER_NEW_PASSWORD environment variable
	// 3. Auto-generate secure random password

	if passwordFlag != "" {
		// Explicit password provided via --password flag
		return passwordFlag, nil
	}

	// Check environment variable
	if envPassword := os.Getenv("TIGER_NEW_PASSWORD"); envPassword != "" {
		return envPassword, nil
	}

	// Auto-generate secure password
	return util.GenerateSecurePassword(32)
}

// CreateRoleResult represents the output of a create role operation
type CreateRoleResult struct {
	RoleName         string   `json:"role_name"`
	ReadOnly         bool     `json:"read_only,omitempty"`
	StatementTimeout string   `json:"statement_timeout,omitempty"`
	FromRoles        []string `json:"from_roles,omitempty"`
}

// outputCreateRoleResult formats and outputs the create role result
func outputCreateRoleResult(cmd *cobra.Command, roleName string, readOnly bool, statementTimeout time.Duration, fromRoles []string, format string) error {
	result := CreateRoleResult{
		RoleName: roleName,
		ReadOnly: readOnly,
	}

	if statementTimeout > 0 {
		result.StatementTimeout = statementTimeout.String()
	}

	if len(fromRoles) > 0 {
		result.FromRoles = fromRoles
	}

	outputWriter := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(outputWriter, result)
	case "yaml":
		return util.SerializeToYAML(outputWriter, result)
	default: // table format
		fmt.Fprintf(outputWriter, "✓ Role '%s' created successfully\n", roleName)
		if readOnly {
			fmt.Fprintf(outputWriter, "  Read-only enforcement: enabled (permanent, role-based)\n")
		}
		if statementTimeout > 0 {
			fmt.Fprintf(outputWriter, "  Statement timeout: %s\n", statementTimeout)
		}
		if len(fromRoles) > 0 {
			fmt.Fprintf(outputWriter, "  Inherits from: %s\n", strings.Join(fromRoles, ", "))
		}
		return nil
	}
}
