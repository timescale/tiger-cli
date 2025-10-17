package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/password"
)

var (
	// getAPIKeyForDB can be overridden for testing
	getAPIKeyForDB = config.GetAPIKey
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
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := getServiceDetails(cmd, args)
			if err != nil {
				return err
			}

			details, err := password.GetConnectionDetails(service, password.ConnectionDetailsOptions{
				Pooled:       dbConnectionStringPooled,
				Role:         dbConnectionStringRole,
				WithPassword: dbConnectionStringWithPassword,
				WarnWriter:   cmd.ErrOrStderr(),
			})
			if err != nil {
				return fmt.Errorf("failed to build connection string: %w", err)
			}

			if dbConnectionStringWithPassword && details.Password == "" {
				return fmt.Errorf("password not available to include in connection string")
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
		Use:     "connect [service-id]",
		Aliases: []string{"psql"},
		Short:   "Connect to a database",
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

  # Connect using connection pooler (if available)
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
				Pooled:       dbConnectPooled,
				Role:         dbConnectRole,
				WithPassword: false,
				WarnWriter:   cmd.ErrOrStderr(),
			})
			if err != nil {
				return fmt.Errorf("failed to build connection string: %w", err)
			}

			// Launch psql with additional flags
			return launchPsqlWithConnectionString(details.String(), psqlPath, psqlFlags, service, cmd)
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
				WarnWriter:   cmd.ErrOrStderr(),
			})
			if err != nil {
				return exitWithCode(ExitInvalidParameters, fmt.Errorf("failed to build connection string: %w", err))
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

func buildDbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database operations and management",
		Long:  `Database-specific operations including connection management, testing, and configuration.`,
	}

	cmd.AddCommand(buildDbConnectionStringCmd())
	cmd.AddCommand(buildDbConnectCmd())
	cmd.AddCommand(buildDbTestConnectionCmd())

	return cmd
}

// getServiceDetails is a helper that handles common service lookup logic and returns the service details
func getServiceDetails(cmd *cobra.Command, args []string) (api.Service, error) {
	// Get config
	cfg, err := config.Load()
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to load config: %w", err)
	}

	projectID := cfg.ProjectID
	if projectID == "" {
		return api.Service{}, fmt.Errorf("project ID is required. Set it using login with --project-id")
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

	// Get API key for authentication
	apiKey, err := getAPIKeyForDB()
	if err != nil {
		return api.Service{}, exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w", err))
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
func launchPsqlWithConnectionString(connectionString, psqlPath string, additionalFlags []string, service api.Service, cmd *cobra.Command) error {
	psqlCmd := buildPsqlCommand(connectionString, psqlPath, additionalFlags, service, cmd)
	return psqlCmd.Run()
}

// buildPsqlCommand creates the psql command with proper environment setup
func buildPsqlCommand(connectionString, psqlPath string, additionalFlags []string, service api.Service, cmd *cobra.Command) *exec.Cmd {
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
		if password, err := storage.Get(service); err == nil && password != "" {
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
