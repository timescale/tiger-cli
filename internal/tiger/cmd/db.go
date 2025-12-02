package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

var (
	// getCredentialsForDB can be overridden for testing
	getCredentialsForDB = config.GetCredentials

	// getServiceDetailsFunc can be overridden for testing
	getServiceDetailsFunc = getServiceDetails

	// checkStdinIsTTY can be overridden for testing to bypass TTY detection
	checkStdinIsTTY = func() bool {
		return util.IsTerminal(os.Stdin)
	}

	// readPasswordFromTerminal can be overridden for testing to inject password input
	readPasswordFromTerminal = func() (string, error) {
		val, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return "", err
		}
		return string(val), nil
	}
)

func buildDbConnectionStringCmd() *cobra.Command {
	var dbConnectionStringPooled bool
	var dbConnectionStringRole string
	var dbConnectionStringWithPassword bool

	cmd := &cobra.Command{
		Use:   "connection-string [service-id]",
		Short: "Get connection string for a service",
		Long: `Get a PostgreSQL connection string for connecting to a database service.

The service ID can be provided as an argument or will use the default service
from your configuration. The connection string includes all necessary parameters
for establishing a database connection to the TimescaleDB/PostgreSQL service.

By default, passwords are excluded from the connection string for security.
Use --with-password to include the password directly in the connection string.

Examples:
  # Get connection string for default service
  tiger db connection-string

  # Get connection string for specific service
  tiger db connection-string svc-12345

  # Get pooled connection string (uses connection pooler if available)
  tiger db connection-string svc-12345 --pooled

  # Get connection string with custom role/username
  tiger db connection-string svc-12345 --role readonly

  # Get connection string with password included (less secure)
  tiger db connection-string svc-12345 --with-password`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := getServiceDetails(cmd, args)
			if err != nil {
				return err
			}

			details, err := common.GetConnectionDetails(service, common.ConnectionDetailsOptions{
				Pooled:       dbConnectionStringPooled,
				Role:         dbConnectionStringRole,
				WithPassword: dbConnectionStringWithPassword,
			})
			if err != nil {
				return fmt.Errorf("failed to build connection string: %w", err)
			}

			if dbConnectionStringWithPassword && details.Password == "" {
				return fmt.Errorf("password not available to include in connection string")
			}

			if dbConnectionStringPooled && !details.IsPooler {
				return fmt.Errorf("connection pooler not available for this service")
			}

			fmt.Fprintln(cmd.OutOrStdout(), details.String())
			return nil
		},
	}

	// Add flags for db connection-string command
	cmd.Flags().BoolVar(&dbConnectionStringPooled, "pooled", false, "Use connection pooling")
	cmd.Flags().StringVar(&dbConnectionStringRole, "role", "tsdbadmin", "Database role/username")
	cmd.Flags().BoolVar(&dbConnectionStringWithPassword, "with-password", false, "Include password in connection string (less secure)")

	return cmd
}

func buildDbConnectCmd() *cobra.Command {
	var dbConnectPooled bool
	var dbConnectRole string
	var dbConnectPassword string
	var dbConnectResetPassword bool

	cmd := &cobra.Command{
		Use:     "connect [service-id]",
		Aliases: []string{"psql"},
		Short:   "Connect to a database",
		Long: `Connect to a database service using psql client.

The service ID can be provided as an argument or will use the default service
from your configuration. This command will launch an interactive psql session
with the appropriate connection parameters.

Authentication is handled automatically:
1. First tests the stored password (keyring or ~/.pgpass based on --password-storage setting)
2. If authentication fails, offers interactive options:
   - Enter password manually (will be saved for future use)
   - Reset password (generates a new password via the API)
   - Exit

For headless/scripted usage, use --password or --reset-password flags to skip
the interactive menu.

Examples:
  # Connect to default service
  tiger db connect
  tiger db psql

  # Connect to specific service
  tiger db connect svc-12345
  tiger db psql svc-12345

  # Connect using connection pooler
  tiger db connect svc-12345 --pooled
  tiger db psql svc-12345 --pooled

  # Connect with custom role/username
  tiger db connect svc-12345 --role readonly
  tiger db psql svc-12345 --role readonly

  # Connect with explicit password (headless mode)
  tiger db connect svc-12345 --password "my-password"

  # Reset password and connect (headless mode)
  tiger db connect svc-12345 --reset-password

  # Pass additional flags to psql (use -- to separate)
  tiger db connect svc-12345 -- --single-transaction --quiet
  tiger db psql svc-12345 -- -c "SELECT version();" --no-psqlrc`,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: serviceIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Separate service ID from additional psql flags
			serviceArgs, psqlFlags := separateServiceAndPsqlArgs(cmd, args)

			service, err := getServiceDetails(cmd, serviceArgs)
			if err != nil {
				return err
			}

			// Check if psql is available
			psqlPath, err := exec.LookPath("psql")
			if err != nil {
				return fmt.Errorf("psql client not found. Please install PostgreSQL client tools")
			}

			details, err := common.GetConnectionDetails(service, common.ConnectionDetailsOptions{
				Pooled: dbConnectPooled,
				Role:   dbConnectRole,
			})
			if err != nil {
				return fmt.Errorf("failed to build connection string: %w", err)
			}

			if dbConnectPooled && !details.IsPooler {
				return fmt.Errorf("connection pooler not available for this service")
			}

			cmd.SilenceUsage = true

			// Build connect options
			opts := connectOptions{
				password:      dbConnectPassword,
				resetPassword: dbConnectResetPassword,
			}

			return connectWithPasswordMenu(cmd.Context(), cmd, service, details, psqlPath, psqlFlags, dbConnectRole, opts)
		},
	}

	// Add flags for db connect command (works for both connect and psql)
	cmd.Flags().BoolVar(&dbConnectPooled, "pooled", false, "Use connection pooling")
	cmd.Flags().StringVar(&dbConnectRole, "role", "tsdbadmin", "Database role/username")
	cmd.Flags().StringVar(&dbConnectPassword, "password", "", "Password to use for authentication (skips interactive prompt)")
	cmd.Flags().BoolVar(&dbConnectResetPassword, "reset-password", false, "Reset password before connecting (skips interactive prompt)")

	cmd.MarkFlagsMutuallyExclusive("password", "reset-password")

	return cmd
}

func buildDbTestConnectionCmd() *cobra.Command {
	var dbTestConnectionTimeout time.Duration
	var dbTestConnectionPooled bool
	var dbTestConnectionRole string

	cmd := &cobra.Command{
		Use:   "test-connection [service-id]",
		Short: "Test database connectivity",
		Long: `Test database connectivity to a service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command tests if the database is accepting
connections and returns appropriate exit codes following pg_isready conventions.

Return Codes:
  0: Server is accepting connections normally
  1: Server is rejecting connections (e.g., during startup)
  2: No response to connection attempt (server unreachable)
  3: No attempt made (e.g., invalid parameters)

Examples:
  # Test connection to default service
  tiger db test-connection

  # Test connection to specific service
  tiger db test-connection svc-12345

  # Test connection with custom timeout (10 seconds)
  tiger db test-connection svc-12345 --timeout 10s

  # Test connection with longer timeout (5 minutes)
  tiger db test-connection svc-12345 --timeout 5m

  # Test connection with no timeout (wait indefinitely)
  tiger db test-connection svc-12345 --timeout 0`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := getServiceDetails(cmd, args)
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, err)
			}

			// Build connection string for testing with password (if available)
			details, err := common.GetConnectionDetails(service, common.ConnectionDetailsOptions{
				Pooled:       dbTestConnectionPooled,
				Role:         dbTestConnectionRole,
				WithPassword: true,
			})
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, fmt.Errorf("failed to build connection string: %w", err))
			}

			if dbTestConnectionPooled && !details.IsPooler {
				return common.ExitWithCode(common.ExitInvalidParameters, fmt.Errorf("connection pooler not available for this service"))
			}

			// Validate timeout (Cobra handles parsing automatically)
			if dbTestConnectionTimeout < 0 {
				return common.ExitWithCode(common.ExitInvalidParameters, fmt.Errorf("timeout must be positive or zero, got %v", dbTestConnectionTimeout))
			}

			// Test the connection
			return testDatabaseConnection(cmd.Context(), details.String(), dbTestConnectionTimeout, cmd)
		},
	}

	// Add flags for db test-connection command
	cmd.Flags().DurationVarP(&dbTestConnectionTimeout, "timeout", "t", 3*time.Second, "Timeout duration (e.g., 30s, 5m, 1h). Use 0 for no timeout")
	cmd.Flags().BoolVar(&dbTestConnectionPooled, "pooled", false, "Use connection pooling")
	cmd.Flags().StringVar(&dbTestConnectionRole, "role", "tsdbadmin", "Database role/username")

	return cmd
}

func buildDbSavePasswordCmd() *cobra.Command {
	var dbSavePasswordRole string
	var dbSavePasswordValue string

	cmd := &cobra.Command{
		Use:   "save-password [service-id]",
		Short: "Save password for a database service",
		Long: `Save a password for a database service to configured password storage.

The service ID can be provided as an argument or will use the default service
from your configuration. The password can be provided via:
1. --password flag with explicit value (highest precedence)
2. TIGER_NEW_PASSWORD environment variable
3. Interactive prompt (if neither provided)

The password will be saved according to your --password-storage setting
(keyring, pgpass, or none).

Examples:
  # Save password with explicit value (highest precedence)
  tiger db save-password svc-12345 --password=your-password

  # Using environment variable
  export TIGER_NEW_PASSWORD=your-password
  tiger db save-password svc-12345

  # Interactive password prompt (when neither flag nor env var provided)
  tiger db save-password svc-12345

  # Save password for custom role
  tiger db save-password svc-12345 --password=your-password --role readonly

  # Save to specific storage location
  tiger db save-password svc-12345 --password=your-password --password-storage pgpass`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := getServiceDetailsFunc(cmd, args)
			if err != nil {
				return err
			}

			// Determine password based on precedence:
			// 1. --password flag with value
			// 2. TIGER_NEW_PASSWORD environment variable
			// 3. Interactive prompt
			var passwordToSave string

			if cmd.Flags().Changed("password") {
				// --password flag was provided
				passwordToSave = dbSavePasswordValue
				if passwordToSave == "" {
					return fmt.Errorf("password cannot be empty when provided via --password flag")
				}
			} else if envPassword := os.Getenv("TIGER_NEW_PASSWORD"); envPassword != "" {
				// Use environment variable
				passwordToSave = envPassword
			} else {
				// Interactive prompt - check if we're in a terminal
				if !checkStdinIsTTY() {
					return fmt.Errorf("TTY not detected - password required. Use --password flag or TIGER_NEW_PASSWORD environment variable")
				}

				fmt.Fprint(cmd.OutOrStdout(), "Enter password: ")
				passwordToSave, err = readString(cmd.Context(), readPasswordFromTerminal)
				if err != nil {
					return fmt.Errorf("failed to read password: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout()) // Print newline after hidden input
				if passwordToSave == "" {
					return fmt.Errorf("password cannot be empty")
				}
			}

			// Save password using configured storage
			storage := common.GetPasswordStorage()
			if err := storage.Save(service, passwordToSave, dbSavePasswordRole); err != nil {
				return fmt.Errorf("failed to save password: %w", err)
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Password saved successfully for service %s (role: %s)\n",
				*service.ServiceId, dbSavePasswordRole)
			return nil
		},
	}

	// Add flags for db save-password command
	cmd.Flags().StringVarP(&dbSavePasswordValue, "password", "p", "", "Password to save")
	cmd.Flags().StringVar(&dbSavePasswordRole, "role", "tsdbadmin", "Database role/username")

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

// generateSecurePassword generates a cryptographically secure random password
func generateSecurePassword(length int) (string, error) {
	// Generate random bytes
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}

	// Encode as base64 (URL-safe variant to avoid special characters that might need escaping)
	encodedPassword := base64.URLEncoding.EncodeToString(bytes)

	// Trim to desired length (base64 encoding makes it slightly longer)
	if len(encodedPassword) > length {
		encodedPassword = encodedPassword[:length]
	}

	return encodedPassword, nil
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
	return generateSecurePassword(32)
}

// CreateRoleResult represents the output of a create role operation
type CreateRoleResult struct {
	RoleName         string   `json:"role_name" yaml:"role_name"`
	ReadOnly         bool     `json:"read_only,omitempty" yaml:"read_only,omitempty"`
	StatementTimeout string   `json:"statement_timeout,omitempty" yaml:"statement_timeout,omitempty"`
	FromRoles        []string `json:"from_roles,omitempty" yaml:"from_roles,omitempty"`
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
		return util.SerializeToYAML(outputWriter, result, false)
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
from your configuration.

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
		PreRunE:           bindFlags("output"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate arguments
			if roleName == "" {
				return fmt.Errorf("--name is required")
			}

			cmd.SilenceUsage = true

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Get password
			rolePassword, err := getPasswordForRole(passwordFlag)
			if err != nil {
				return fmt.Errorf("failed to determine password: %w", err)
			}

			// Get service details
			service, err := getServiceDetails(cmd, args)
			if err != nil {
				return err
			}

			// Build connection string
			details, err := common.GetConnectionDetails(service, common.ConnectionDetailsOptions{
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
			result, err := common.SavePasswordWithResult(service, rolePassword, roleName)
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

func buildDbCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create database resources",
		Long:  `Create database resources such as roles, databases, and extensions.`,
	}

	cmd.AddCommand(buildDbCreateRoleCmd())

	return cmd
}

func buildDbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database operations and management",
		Long:  `Database-specific operations including connection management, testing, and configuration.`,
	}

	cmd.AddCommand(buildDbConnectionStringCmd())
	cmd.AddCommand(buildDbConnectCmd())
	cmd.AddCommand(buildDbTestConnectionCmd())
	cmd.AddCommand(buildDbSavePasswordCmd())
	cmd.AddCommand(buildDbCreateCmd())

	return cmd
}

// getServiceDetails is a helper that handles common service lookup logic and returns the service details
func getServiceDetails(cmd *cobra.Command, args []string) (api.Service, error) {
	// Get config
	cfg, err := config.Load()
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to load config: %w", err)
	}

	// Determine service ID
	serviceID, err := getServiceID(cfg, args)
	if err != nil {
		return api.Service{}, err
	}

	cmd.SilenceUsage = true

	// Get API key and project ID for authentication
	apiKey, projectID, err := getCredentialsForDB()
	if err != nil {
		return api.Service{}, common.ExitWithCode(common.ExitAuthenticationError, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err))
	}

	// Create API client
	client, err := api.NewTigerClient(cfg, apiKey)
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to create API client: %w", err)
	}

	// Fetch service details
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to fetch service details: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 200 {
		return api.Service{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return api.Service{}, fmt.Errorf("empty response from API")
	}

	return *resp.JSON200, nil
}

// ArgsLenAtDashProvider defines the interface for getting ArgsLenAtDash
type ArgsLenAtDashProvider interface {
	ArgsLenAtDash() int
}

// separateServiceAndPsqlArgs separates service arguments from psql flags using Cobra's ArgsLenAtDash
func separateServiceAndPsqlArgs(cmd ArgsLenAtDashProvider, args []string) ([]string, []string) {
	var serviceArgs []string
	psqlFlags := []string{}

	argsLenAtDash := cmd.ArgsLenAtDash()
	if argsLenAtDash >= 0 {
		// There was a -- separator
		serviceArgs = args[:argsLenAtDash]
		psqlFlags = args[argsLenAtDash:]
	} else {
		// No -- separator
		serviceArgs = args
	}

	return serviceArgs, psqlFlags
}

// launchPsql launches psql using the connection string and additional flags.
// It retrieves the password from storage and sets PGPASSWORD environment variable.
func launchPsql(connectionString, psqlPath string, additionalFlags []string, service api.Service, role string, cmd *cobra.Command) error {
	psqlCmd := buildPsqlCommand(connectionString, psqlPath, additionalFlags, service, role, cmd)
	return psqlCmd.Run()
}

// buildPsqlCommand creates the psql command with proper environment setup
func buildPsqlCommand(connectionString, psqlPath string, additionalFlags []string, service api.Service, role string, cmd *cobra.Command) *exec.Cmd {
	// Build command arguments: connection string first, then additional flags
	// Note: connectionString contains only "postgresql://user@host:port/db" - no password
	// Passwords are passed via PGPASSWORD environment variable (see below)
	args := []string{connectionString}
	args = append(args, additionalFlags...)

	psqlCmd := exec.Command(psqlPath, args...)

	// Use cmd's input/output streams for testability while maintaining CLI behavior
	psqlCmd.Stdin = cmd.InOrStdin()
	psqlCmd.Stdout = cmd.OutOrStdout()
	psqlCmd.Stderr = cmd.ErrOrStderr()

	// Only set PGPASSWORD for keyring storage method
	// pgpass storage relies on psql automatically reading ~/.pgpass file
	storage := common.GetPasswordStorage()
	if _, isKeyring := storage.(*common.KeyringStorage); isKeyring {
		if password, err := storage.Get(service, role); err == nil && password != "" {
			// Set PGPASSWORD environment variable for psql when using keyring
			psqlCmd.Env = append(os.Environ(), "PGPASSWORD="+password)
		}
		// Note: If keyring password retrieval fails, we let psql try without it
		// This allows fallback to other authentication methods
	}

	return psqlCmd
}

// testDatabaseConnection tests the database connection and returns appropriate exit codes
func testDatabaseConnection(ctx context.Context, connectionString string, timeout time.Duration, cmd *cobra.Command) error {
	// Create context with timeout if specified
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Attempt to connect to the database
	// The connection string already includes the password (if available) thanks to PasswordOptional mode
	conn, err := pgx.Connect(ctx, connectionString)
	if err != nil {
		// Determine the appropriate exit code based on error type
		if isContextDeadlineExceeded(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection timeout after %v\n", timeout)
			return common.ExitWithCode(common.ExitTimeout, err) // Connection timeout
		}

		// Check if it's a connection rejection vs unreachable
		if isConnectionRejected(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection rejected: %v\n", err)
			return common.ExitWithCode(common.ExitGeneralError, err) // Server is rejecting connections
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "Connection failed: %v\n", err)
		return common.ExitWithCode(2, err) // No response to connection attempt
	}
	defer conn.Close(ctx)

	// Test the connection with a simple ping
	err = conn.Ping(ctx)
	if err != nil {
		// Determine the appropriate exit code based on error type
		if isContextDeadlineExceeded(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection timeout after %v\n", timeout)
			return common.ExitWithCode(common.ExitTimeout, err) // Connection timeout
		}

		// Check if it's a connection rejection vs unreachable
		if isConnectionRejected(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection rejected: %v\n", err)
			return common.ExitWithCode(common.ExitGeneralError, err) // Server is rejecting connections
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "Connection failed: %v\n", err)
		return common.ExitWithCode(2, err) // No response to connection attempt
	}

	// Connection successful
	fmt.Fprintf(cmd.OutOrStdout(), "Connection successful\n")
	return nil // Server is accepting connections normally
}

// isContextDeadlineExceeded checks if the error is due to context timeout
func isContextDeadlineExceeded(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

// isConnectionRejected determines if the connection was actively rejected vs unreachable
func isConnectionRejected(err error) bool {
	// According to PostgreSQL error codes, only ERRCODE_CANNOT_CONNECT_NOW (57P03)
	// should be considered as "server rejecting connections" (exit code 1).
	// This occurs when the server is running but cannot accept new connections
	// (e.g., during startup, shutdown, or when max_connections is reached).

	// Check if this is a PostgreSQL error with the specific error code
	if pgxErr, ok := err.(*pgconn.PgError); ok {
		// ERRCODE_CANNOT_CONNECT_NOW is 57P03
		return pgxErr.Code == "57P03"
	}

	// All other errors (authentication, authorization, network issues, etc.)
	// should be treated as "unreachable" (exit code 2)
	return false
}

// isAuthenticationError checks if the error is a PostgreSQL authentication failure
func isAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	// Check for PostgreSQL error code 28P01 (invalid_password) or 28000 (invalid_authorization_specification)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "28P01" || pgErr.Code == "28000"
	}
	return false
}

// testConnectionWithPassword tests database connectivity with a specific password
// Returns nil on success, error on failure
func testConnectionWithPassword(ctx context.Context, details *common.ConnectionDetails, password string, timeout time.Duration) error {
	// Build connection string with password (URL-encode the password to handle special chars)
	connStr := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=require",
		details.Role, url.QueryEscape(password), details.Host, details.Port, details.Database)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return nil
}

// passwordRecoveryOption represents the user's choice in the password recovery menu
type passwordRecoveryOption int

const (
	optionEnterPassword passwordRecoveryOption = iota
	optionResetPassword
	optionExit
)

// passwordRecoveryModel is the Bubble Tea model for password recovery selection
type passwordRecoveryModel struct {
	options  []string
	cursor   int
	selected passwordRecoveryOption
	done     bool
}

func newPasswordRecoveryModel() passwordRecoveryModel {
	return passwordRecoveryModel{
		options: []string{
			"Enter password manually",
			"Reset password (generate new)",
			"Exit",
		},
		cursor: 0,
	}
}

func (m passwordRecoveryModel) Init() tea.Cmd {
	return nil
}

func (m passwordRecoveryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.selected = optionExit
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.selected = passwordRecoveryOption(m.cursor)
			m.done = true
			return m, tea.Quit
		case "1":
			m.cursor = 0
			m.selected = optionEnterPassword
			m.done = true
			return m, tea.Quit
		case "2":
			m.cursor = 1
			m.selected = optionResetPassword
			m.done = true
			return m, tea.Quit
		case "3":
			m.cursor = 2
			m.selected = optionExit
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m passwordRecoveryModel) View() string {
	s := "Authentication failed. What would you like to do?\n\n"

	for i, option := range m.options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %d. %s\n", cursor, i+1, option)
	}

	s += "\nUse ↑/↓ arrows or number keys to select, enter to confirm, q to quit"
	return s
}

// selectPasswordRecoveryOption shows the interactive menu for password recovery
func selectPasswordRecoveryOption(out io.Writer) (passwordRecoveryOption, error) {
	model := newPasswordRecoveryModel()

	program := tea.NewProgram(model, tea.WithOutput(out))
	finalModel, err := program.Run()
	if err != nil {
		return optionExit, fmt.Errorf("failed to run password recovery menu: %w", err)
	}

	result := finalModel.(passwordRecoveryModel)
	return result.selected, nil
}

// updateAndSaveServicePassword updates a service password via API and saves it locally.
// It handles the API call and password storage.
func updateAndSaveServicePassword(
	ctx context.Context,
	client api.ClientWithResponsesInterface,
	projectID string,
	service api.Service,
	newPassword string,
	role string,
	statusOut io.Writer,
) error {
	serviceID := *service.ServiceId

	// Call API to update password
	updateReq := api.UpdatePasswordInput{Password: newPassword}
	resp, err := client.PostProjectsProjectIdServicesServiceIdUpdatePasswordWithResponse(ctx, projectID, serviceID, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	if resp.StatusCode() != 200 && resp.StatusCode() != 204 {
		return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	// Save password locally
	result, err := common.SavePasswordWithResult(service, newPassword, role)
	if err != nil {
		fmt.Fprintf(statusOut, "Warning: could not save password: %v\n", err)
	} else if result.Success {
		fmt.Fprintf(statusOut, "%s\n", result.Message)
	}

	return nil
}

// resetServicePassword generates a new password and updates it via API
func resetServicePassword(ctx context.Context, cfg *config.Config, service api.Service, role string, statusOut io.Writer) (string, error) {
	// Generate new password
	newPassword, err := generateSecurePassword(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate new password: %w", err)
	}

	// Get API credentials
	apiKey, projectID, err := getCredentialsForService()
	if err != nil {
		return "", fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err)
	}

	// Create API client
	client, err := api.NewTigerClient(cfg, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to create API client: %w", err)
	}

	// Update and save password
	if err := updateAndSaveServicePassword(ctx, client, projectID, service, newPassword, role, statusOut); err != nil {
		return "", err
	}

	fmt.Fprintf(statusOut, "Password reset successfully for '%s' user\n", role)
	return newPassword, nil
}

// connectOptions holds options for the connect command
type connectOptions struct {
	password      string // Explicit password to use (headless mode)
	resetPassword bool   // Reset password before connecting (headless mode)
}

// testSaveAndLaunchPsqlWithPassword tests a password, saves it if valid, and launches psql.
// Returns a retryable error if authentication fails, or a fatal error otherwise.
func testSaveAndLaunchPsqlWithPassword(
	ctx context.Context,
	cmd *cobra.Command,
	details *common.ConnectionDetails,
	psqlPath string,
	psqlFlags []string,
	service api.Service,
	password string,
	role string,
) error {
	// Test the password
	err := testConnectionWithPassword(ctx, details, password, 10*time.Second)
	if err != nil {
		return err
	}

	// Password works! Save it
	result, saveErr := common.SavePasswordWithResult(service, password, role)
	if saveErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not save password: %v\n", saveErr)
	} else if result.Success {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s\n", result.Message)
	}

	// Launch psql (password is now in storage, launchPsql will retrieve it)
	return launchPsql(details.String(), psqlPath, psqlFlags, service, role, cmd)
}

// resetPasswordAndLaunchPsql resets the password via API and launches psql with the new password.
func resetPasswordAndLaunchPsql(
	ctx context.Context,
	cmd *cobra.Command,
	cfg *config.Config,
	details *common.ConnectionDetails,
	psqlPath string,
	psqlFlags []string,
	service api.Service,
	role string,
) error {
	_, err := resetServicePassword(ctx, cfg, service, role, cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "To view your new password, run: tiger db connection-string --with-password\n")

	// Launch psql (password is now in storage, launchPsql will retrieve it)
	return launchPsql(details.String(), psqlPath, psqlFlags, service, role, cmd)
}

// connectWithPasswordMenu handles the connection flow if the stored password is invalid
// Offers an interactive menu to enter the password manually or reset it
func connectWithPasswordMenu(
	ctx context.Context,
	cmd *cobra.Command,
	service api.Service,
	details *common.ConnectionDetails,
	psqlPath string,
	psqlFlags []string,
	role string,
	opts connectOptions,
) error {
	// Load config (needed for password reset)
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Handle --reset-password flag (headless mode)
	if opts.resetPassword {
		if err := resetPasswordAndLaunchPsql(ctx, cmd, cfg, details, psqlPath, psqlFlags, service, role); err != nil {
			return fmt.Errorf("failed to reset password: %w", err)
		}
		return nil
	}

	// Handle --password flag (headless mode)
	if opts.password != "" {
		err := testSaveAndLaunchPsqlWithPassword(ctx, cmd, details, psqlPath, psqlFlags, service, opts.password, role)
		if err != nil {
			if isAuthenticationError(err) {
				return fmt.Errorf("authentication failed: invalid password")
			}
			return fmt.Errorf("connection failed: %w", err)
		}
		return nil
	}

	// Interactive mode: Get stored password (if any)
	storage := common.GetPasswordStorage()
	storedPassword, _ := storage.Get(service, role)

	// Try to connect with stored password first
	err = testConnectionWithPassword(ctx, details, storedPassword, 10*time.Second)
	if err == nil {
		// Password works, launch psql
		return launchPsql(details.String(), psqlPath, psqlFlags, service, role, cmd)
	}

	// Check if it's an auth error
	if !isAuthenticationError(err) {
		// Non-auth error (network, timeout, etc.) - report it directly
		return fmt.Errorf("connection failed: %w", err)
	}
	// Auth failed with stored password, continue to recovery menu
	fmt.Fprintf(cmd.ErrOrStderr(), "Stored password is invalid or expired.\n\n")

	// Check if we're in a TTY for interactive menu
	if !checkStdinIsTTY() {
		return fmt.Errorf("authentication failed and no TTY available for interactive password entry. " +
			"Use --password or --reset-password flag for headless operation")
	}

	// Interactive recovery loop
	for {
		option, err := selectPasswordRecoveryOption(cmd.ErrOrStderr())
		if err != nil {
			return err
		}

		switch option {
		case optionEnterPassword:
			// Prompt for password
			fmt.Fprint(cmd.ErrOrStderr(), "Enter password: ")
			password, err := readPasswordFromTerminal()
			fmt.Fprintln(cmd.ErrOrStderr()) // newline after password entry
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error reading password: %v\n\n", err)
				continue
			}

			// Test, save, and launch
			err = testSaveAndLaunchPsqlWithPassword(ctx, cmd, details, psqlPath, psqlFlags, service, password, role)
			if err != nil {
				if isAuthenticationError(err) {
					fmt.Fprintf(cmd.ErrOrStderr(), "Password incorrect. Please try again.\n\n")
					continue
				}
				return fmt.Errorf("connection failed: %w", err)
			}
			return nil

		case optionResetPassword:
			// Reset and launch
			if err := resetPasswordAndLaunchPsql(ctx, cmd, cfg, details, psqlPath, psqlFlags, service, role); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error resetting password: %v\n\n", err)
				continue
			}
			return nil

		case optionExit:
			return nil
		}
	}
}
