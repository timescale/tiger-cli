package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/password"
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
		Use:               "connection-string [service-id]",
		Short:             "Get connection string for a service",
		ValidArgsFunction: serviceIDCompletion,
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
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := getServiceDetails(cmd, args)
			if err != nil {
				return err
			}

			details, err := password.GetConnectionDetails(service, password.ConnectionDetailsOptions{
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

	cmd := &cobra.Command{
		Use:               "connect [service-id]",
		Aliases:           []string{"psql"},
		Short:             "Connect to a database",
		ValidArgsFunction: serviceIDCompletion,
		Long: `Connect to a database service using psql client.

The service ID can be provided as an argument or will use the default service
from your configuration. This command will launch an interactive psql session
with the appropriate connection parameters.

Authentication is handled automatically using:
1. Stored password (keyring, ~/.pgpass, or none based on --password-storage setting)  
2. PGPASSWORD environment variable
3. Interactive password prompt (if neither above is available)

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

  # Pass additional flags to psql (use -- to separate)
  tiger db connect svc-12345 -- --single-transaction --quiet
  tiger db psql svc-12345 -- -c "SELECT version();" --no-psqlrc`,
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

			details, err := password.GetConnectionDetails(service, password.ConnectionDetailsOptions{
				Pooled: dbConnectPooled,
				Role:   dbConnectRole,
			})
			if err != nil {
				return fmt.Errorf("failed to build connection string: %w", err)
			}

			if dbConnectPooled && !details.IsPooler {
				return fmt.Errorf("connection pooler not available for this service")
			}

			// Launch psql with additional flags
			return launchPsqlWithConnectionString(details.String(), psqlPath, psqlFlags, service, dbConnectRole, cmd)
		},
	}

	// Add flags for db connect command (works for both connect and psql)
	cmd.Flags().BoolVar(&dbConnectPooled, "pooled", false, "Use connection pooling")
	cmd.Flags().StringVar(&dbConnectRole, "role", "tsdbadmin", "Database role/username")

	return cmd
}

func buildDbTestConnectionCmd() *cobra.Command {
	var dbTestConnectionTimeout time.Duration
	var dbTestConnectionPooled bool
	var dbTestConnectionRole string

	cmd := &cobra.Command{
		Use:               "test-connection [service-id]",
		Short:             "Test database connectivity",
		ValidArgsFunction: serviceIDCompletion,
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
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := getServiceDetails(cmd, args)
			if err != nil {
				return exitWithCode(ExitInvalidParameters, err)
			}

			// Build connection string for testing with password (if available)
			details, err := password.GetConnectionDetails(service, password.ConnectionDetailsOptions{
				Pooled:       dbTestConnectionPooled,
				Role:         dbTestConnectionRole,
				WithPassword: true,
			})
			if err != nil {
				return exitWithCode(ExitInvalidParameters, fmt.Errorf("failed to build connection string: %w", err))
			}

			if dbTestConnectionPooled && !details.IsPooler {
				return exitWithCode(ExitInvalidParameters, fmt.Errorf("connection pooler not available for this service"))
			}

			// Validate timeout (Cobra handles parsing automatically)
			if dbTestConnectionTimeout < 0 {
				return exitWithCode(ExitInvalidParameters, fmt.Errorf("timeout must be positive or zero, got %v", dbTestConnectionTimeout))
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
		Use:               "save-password [service-id]",
		Short:             "Save password for a database service",
		ValidArgsFunction: serviceIDCompletion,
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
			storage := password.GetPasswordStorage()
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
		Use:               "role [service-id]",
		Short:             "Create a new database role",
		ValidArgsFunction: serviceIDCompletion,
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

			// Use flag value if provided, otherwise use config value
			if cmd.Flags().Changed("output") {
				cfg.Output = output
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
			details, err := password.GetConnectionDetails(service, password.ConnectionDetailsOptions{
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
			result, err := password.SavePasswordWithResult(service, rolePassword, roleName)
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
	var serviceID string
	if len(args) > 0 {
		serviceID = args[0]
	} else {
		serviceID = cfg.ServiceID
	}

	if serviceID == "" {
		return api.Service{}, fmt.Errorf("service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'")
	}

	cmd.SilenceUsage = true

	// Get API key and project ID for authentication
	apiKey, projectID, err := getCredentialsForDB()
	if err != nil {
		return api.Service{}, exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err))
	}

	// Create API client
	client, err := api.NewTigerClient(apiKey)
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
		return api.Service{}, exitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
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

// launchPsqlWithConnectionString launches psql using the connection string and additional flags
func launchPsqlWithConnectionString(connectionString, psqlPath string, additionalFlags []string, service api.Service, role string, cmd *cobra.Command) error {
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
	storage := password.GetPasswordStorage()
	if _, isKeyring := storage.(*password.KeyringStorage); isKeyring {
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
			return exitWithCode(ExitTimeout, err) // Connection timeout
		}

		// Check if it's a connection rejection vs unreachable
		if isConnectionRejected(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection rejected: %v\n", err)
			return exitWithCode(ExitGeneralError, err) // Server is rejecting connections
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "Connection failed: %v\n", err)
		return exitWithCode(2, err) // No response to connection attempt
	}
	defer conn.Close(ctx)

	// Test the connection with a simple ping
	err = conn.Ping(ctx)
	if err != nil {
		// Determine the appropriate exit code based on error type
		if isContextDeadlineExceeded(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection timeout after %v\n", timeout)
			return exitWithCode(ExitTimeout, err) // Connection timeout
		}

		// Check if it's a connection rejection vs unreachable
		if isConnectionRejected(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection rejected: %v\n", err)
			return exitWithCode(ExitGeneralError, err) // Server is rejecting connections
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "Connection failed: %v\n", err)
		return exitWithCode(2, err) // No response to connection attempt
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
