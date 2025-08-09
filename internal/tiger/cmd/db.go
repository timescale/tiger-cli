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

	"github.com/tigerdata/tiger-cli/internal/tiger/api"
	"github.com/tigerdata/tiger-cli/internal/tiger/config"
)

var (
	// getAPIKeyForDB can be overridden for testing
	getAPIKeyForDB = getAPIKey
)

func buildDbConnectionStringCmd() *cobra.Command {
	var dbConnectionStringPooled bool
	var dbConnectionStringRole string

	cmd := &cobra.Command{
		Use:   "connection-string [service-id]",
		Short: "Get connection string for a service",
		Long: `Get a PostgreSQL connection string for connecting to a database service.

The service ID can be provided as an argument or will use the default service
from your configuration. The connection string includes all necessary parameters
for establishing a database connection to the TimescaleDB/PostgreSQL service.

Examples:
  # Get connection string for default service
  tiger db connection-string

  # Get connection string for specific service
  tiger db connection-string svc-12345

  # Get pooled connection string (uses connection pooler if available)
  tiger db connection-string svc-12345 --pooled

  # Get connection string with custom role/username
  tiger db connection-string svc-12345 --role readonly`,
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := getServiceDetails(cmd, args)
			if err != nil {
				return err
			}

			connectionString, err := buildConnectionString(service, dbConnectionStringPooled, dbConnectionStringRole, cmd)
			if err != nil {
				return fmt.Errorf("failed to build connection string: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), connectionString)
			return nil
		},
	}

	// Add flags for db connection-string command
	cmd.Flags().BoolVar(&dbConnectionStringPooled, "pooled", false, "Use connection pooling")
	cmd.Flags().StringVar(&dbConnectionStringRole, "role", "tsdbadmin", "Database role/username")

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
1. ~/.pgpass file (if password was saved during service creation)  
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

			// Get connection string using existing logic
			connectionString, err := buildConnectionString(service, dbConnectPooled, dbConnectRole, cmd)
			if err != nil {
				return fmt.Errorf("failed to build connection string: %w", err)
			}

			// Launch psql with additional flags
			return launchPsqlWithConnectionString(connectionString, psqlPath, psqlFlags, cmd)
		},
	}

	// Add flags for db connect command (works for both connect and psql)
	cmd.Flags().BoolVar(&dbConnectPooled, "pooled", false, "Use connection pooling")
	cmd.Flags().StringVar(&dbConnectRole, "role", "tsdbadmin", "Database role/username")

	return cmd
}

func buildDbTestConnectionCmd() *cobra.Command {
	var dbTestConnectionTimeout int
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
  tiger db test-connection svc-12345 --timeout 10

  # Test connection with no timeout (wait indefinitely)
  tiger db test-connection svc-12345 --timeout 0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := getServiceDetails(cmd, args)
			if err != nil {
				return exitWithCode(3, err) // Invalid parameters
			}

			// Build connection string for testing
			connectionString, err := buildConnectionString(service, dbTestConnectionPooled, dbTestConnectionRole, cmd)
			if err != nil {
				return exitWithCode(3, fmt.Errorf("failed to build connection string: %w", err))
			}

			// Test the connection
			return testDatabaseConnection(connectionString, dbTestConnectionTimeout, cmd)
		},
	}

	// Add flags for db test-connection command
	cmd.Flags().IntVarP(&dbTestConnectionTimeout, "timeout", "t", 3, "Timeout in seconds (0 for no timeout)")
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

// buildConnectionString creates a PostgreSQL connection string from service details
func buildConnectionString(service api.Service, pooled bool, role string, cmd *cobra.Command) (string, error) {
	if service.Endpoint == nil {
		return "", fmt.Errorf("service endpoint not available")
	}

	var endpoint *api.Endpoint
	var host string
	var port int

	// Use pooler endpoint if requested and available, otherwise use direct endpoint
	if pooled && service.ConnectionPooler != nil && service.ConnectionPooler.Endpoint != nil {
		endpoint = service.ConnectionPooler.Endpoint
	} else {
		// If pooled was requested but no pooler is available, warn the user
		if pooled {
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  Warning: Connection pooler not available for this service, using direct connection\n")
		}
		endpoint = service.Endpoint
	}

	if endpoint.Host == nil {
		return "", fmt.Errorf("endpoint host not available")
	}
	host = *endpoint.Host

	if endpoint.Port != nil {
		port = *endpoint.Port
	} else {
		port = 5432 // Default PostgreSQL port
	}

	// Database is always "tsdb" for TimescaleDB/PostgreSQL services
	database := "tsdb"

	// Build connection string in PostgreSQL URI format
	connectionString := fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=require", role, host, port, database)

	return connectionString, nil
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
		return api.Service{}, fmt.Errorf("authentication required: %w", err)
	}

	// Create API client
	client, err := api.NewTigerClient(apiKey)
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to create API client: %w", err)
	}

	// Fetch service details
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to fetch service details: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200:
		if resp.JSON200 == nil {
			return api.Service{}, fmt.Errorf("empty response from API")
		}

		return *resp.JSON200, nil

	case 401, 403:
		return api.Service{}, fmt.Errorf("authentication failed: invalid API key")
	case 404:
		return api.Service{}, fmt.Errorf("service '%s' not found in project '%s'", serviceID, projectID)
	default:
		return api.Service{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}

// ArgsLenAtDashProvider defines the interface for getting ArgsLenAtDash
type ArgsLenAtDashProvider interface {
	ArgsLenAtDash() int
}

// separateServiceAndPsqlArgs separates service arguments from psql flags using Cobra's ArgsLenAtDash
func separateServiceAndPsqlArgs(cmd ArgsLenAtDashProvider, args []string) ([]string, []string) {
	serviceArgs := []string{}
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
func launchPsqlWithConnectionString(connectionString, psqlPath string, additionalFlags []string, _ *cobra.Command) error {
	// Build command arguments: connection string first, then additional flags
	args := []string{connectionString}
	args = append(args, additionalFlags...)

	psqlCmd := exec.Command(psqlPath, args...)
	psqlCmd.Stdin = os.Stdin
	psqlCmd.Stdout = os.Stdout
	psqlCmd.Stderr = os.Stderr

	return psqlCmd.Run()
}

// exitWithCode creates an error that will cause the program to exit with the specified code
type exitCodeError struct {
	code int
	err  error
}

func (e exitCodeError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e exitCodeError) ExitCode() int {
	return e.code
}

// exitWithCode returns an error that will cause the program to exit with the specified code
func exitWithCode(code int, err error) error {
	return exitCodeError{code: code, err: err}
}

// testDatabaseConnection tests the database connection and returns appropriate exit codes
func testDatabaseConnection(connectionString string, timeoutSeconds int, cmd *cobra.Command) error {
	// Create context with timeout if specified
	var ctx context.Context
	var cancel context.CancelFunc

	if timeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
	} else {
		ctx = context.Background()
	}

	// Parse the connection string first to validate it
	config, err := pgx.ParseConfig(connectionString)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Failed to parse connection string: %v\n", err)
		return exitWithCode(3, err) // Invalid parameters
	}

	// Attempt to connect to the database
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		// Determine the appropriate exit code based on error type
		if isContextDeadlineExceeded(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection timeout after %d seconds\n", timeoutSeconds)
			return exitWithCode(2, err) // No response to connection attempt
		}

		// Check if it's a connection rejection vs unreachable
		if isConnectionRejected(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection rejected: %v\n", err)
			return exitWithCode(1, err) // Server is rejecting connections
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
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection timeout after %d seconds\n", timeoutSeconds)
			return exitWithCode(2, err) // No response to connection attempt
		}

		// Check if it's a connection rejection vs unreachable
		if isConnectionRejected(err) {
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection rejected: %v\n", err)
			return exitWithCode(1, err) // Server is rejecting connections
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
