package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildDbTestConnectionCmd(app *common.App) *cobra.Command {
	var dbTestConnectionTimeout time.Duration
	var dbTestConnectionPooled bool
	var dbTestConnectionRole string

	cmd := &cobra.Command{
		Use:     "test-connection [service-id]",
		Aliases: []string{"test", "ping"},
		Short:   "Test database connectivity",
		Long: `Test database connectivity to a service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command tests if the database is accepting
connections and returns appropriate exit codes following pg_isready conventions.

You can also pass a read replica set ID to test connectivity to that replica.

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
		ValidArgsFunction: serviceIDCompletion(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, _, _, err := app.GetAll()
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, err)
			}

			target, err := lookupConnectionTarget(cmd, app, args)
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, err)
			}

			// Build connection string for testing with password (if available)
			details, err := buildConnectionDetailsForTarget(cmd, cfg, target, common.ConnectionDetailsOptions{
				Pooled:       dbTestConnectionPooled,
				Role:         dbTestConnectionRole,
				WithPassword: true,
			})
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, err)
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

// testDatabaseConnection tests the database connection and returns appropriate exit codes
func testDatabaseConnection(ctx context.Context, connectionString string, timeout time.Duration, cmd *cobra.Command) error {
	// Every failure below is reported here, so don't let cobra print it again.
	cmd.SilenceErrors = true

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
			cmd.PrintErrf("Connection timeout after %v\n", timeout)
			return common.ExitWithCode(common.ExitTimeout, err) // Connection timeout
		}

		// Check if it's a connection rejection vs unreachable
		if isConnectionRejected(err) {
			cmd.PrintErrf("Connection rejected: %v\n", err)
			return common.ExitWithCode(common.ExitGeneralError, err) // Server is rejecting connections
		}

		cmd.PrintErrf("Connection failed: %v\n", err)
		return common.ExitWithCode(2, err) // No response to connection attempt
	}
	defer conn.Close(ctx)

	// Test the connection with a simple ping
	err = conn.Ping(ctx)
	if err != nil {
		// Determine the appropriate exit code based on error type
		if isContextDeadlineExceeded(err) {
			cmd.PrintErrf("Connection timeout after %v\n", timeout)
			return common.ExitWithCode(common.ExitTimeout, err) // Connection timeout
		}

		// Check if it's a connection rejection vs unreachable
		if isConnectionRejected(err) {
			cmd.PrintErrf("Connection rejected: %v\n", err)
			return common.ExitWithCode(common.ExitGeneralError, err) // Server is rejecting connections
		}

		cmd.PrintErrf("Connection failed: %v\n", err)
		return common.ExitWithCode(2, err) // No response to connection attempt
	}

	// Connection successful
	cmd.Printf("Connection successful\n")
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
