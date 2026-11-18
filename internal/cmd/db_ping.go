package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildDbPingCmd(app *common.App) *cobra.Command {
	var timeout time.Duration
	var pooled bool
	var role string

	cmd := &cobra.Command{
		Use:     "ping [name-or-id]",
		Aliases: []string{"test", "test-connection"},
		Short:   "Test database connectivity",
		Long: `Test database connectivity to a service.

The service can be given by ID or name as an argument, or will use the default
service from your configuration. This command tests if the database is accepting
connections and returns appropriate exit codes following pg_isready conventions.

You can also pass a read replica set ID to test connectivity to that replica.` + branchRefHelp(app.Experimental, "You can also pass a branch, by ID or name, to test connectivity to that branch.") + `

--timeout limits the whole check. The host can resolve to several addresses,
and each gets up to ` + common.ConnectTimeout.String() + ` (or PGCONNECT_TIMEOUT, if set) before the check
moves on to the next, so a --timeout shorter than that limits the first
address too.

Return Codes:
  0: Server is accepting connections normally
  1: Server is rejecting connections (e.g., during startup)
  2: No response to connection attempt (server unreachable)
  3: No attempt made (e.g., invalid parameters)`,
		Example: `  # Test connection to default service
  tiger db ping

  # Test connection to specific service
  tiger db ping svc-12345

  # Test connection with custom timeout (10 seconds)
  tiger db ping svc-12345 --timeout 10s

  # Test connection with longer timeout (5 minutes)
  tiger db ping svc-12345 --timeout 5m

  # Test connection with no timeout (wait indefinitely)
  tiger db ping svc-12345 --timeout 0`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, err)
			}

			serviceRef, err := getServiceRef(cmd, cfg, args)
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, err)
			}

			target, err := resolveConnectionTarget(cmd.Context(), client, projectID, serviceRef, app.Experimental)
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, err)
			}

			warnReplicaPooler(cmd, target, pooled)

			// Build connection string for testing with password (if available)
			details, err := target.Details(cfg, common.ConnectionDetailsOptions{
				Pooled:       pooled,
				Role:         role,
				WithPassword: true,
			})
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, err)
			}

			// Validate timeout (Cobra handles parsing automatically)
			if timeout < 0 {
				return common.ExitWithCode(common.ExitInvalidParameters, fmt.Errorf("timeout must be positive or zero, got %v", timeout))
			}

			connConfig, err := details.ConnConfig()
			if err != nil {
				return common.ExitWithCode(common.ExitInvalidParameters, err)
			}

			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			// Test the connection
			return testDatabaseConnection(ctx, connConfig, cmd)
		},
	}

	// Add flags for db ping command
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 30*time.Second, "Timeout duration (e.g., 30s, 5m, 1h). Use 0 for no timeout")
	cmd.Flags().BoolVar(&pooled, "pooled", false, "Use connection pooling")
	cmd.Flags().StringVar(&role, "role", "tsdbadmin", "Database role/username")

	return cmd
}

// testDatabaseConnection tests the database connection and returns appropriate
// exit codes.
func testDatabaseConnection(ctx context.Context, connConfig *pgx.ConnConfig, cmd *cobra.Command) error {
	// The config includes the password, if one is available.
	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return connectionFailure(err)
	}
	defer conn.Close(ctx)

	// Test the connection with a simple ping
	if err := conn.Ping(ctx); err != nil {
		return connectionFailure(err)
	}

	// Connection successful
	cmd.Printf("Connection successful\n")
	return nil // Server is accepting connections normally
}

// connectionFailure wraps the error from a failed connection or ping with its
// exit code. pgx joins every address's failure into one error, so a server
// reply anywhere in it decides: it shows the server is reachable, whatever
// happened at the other addresses.
func connectionFailure(err error) error {
	if _, ok := errors.AsType[*pgconn.PgError](err); ok {
		if isConnectionRejected(err) {
			return common.ExitWithCode(common.ExitGeneralError, fmt.Errorf("server is rejecting connections: %w", err))
		}
		return common.ExitWithCode(2, err) // Server replied with an error
	}
	if isConnectionTimeout(err) {
		return common.ExitWithCode(common.ExitTimeout, fmt.Errorf("connection timed out: %w", err))
	}
	return common.ExitWithCode(2, err) // No response to connection attempt
}

// isConnectionTimeout checks if the error is due to the connection timeout:
// either the context's own deadline error, or a network timeout — the dialer
// applies the context deadline to the socket, so the same expiry can surface
// as an i/o timeout instead (which error wins varies by platform and pgx
// version).
func isConnectionTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isConnectionRejected determines if the connection was actively rejected vs unreachable
func isConnectionRejected(err error) bool {
	// According to PostgreSQL error codes, only ERRCODE_CANNOT_CONNECT_NOW (57P03)
	// should be considered as "server rejecting connections" (exit code 1).
	// This occurs when the server is running but cannot accept new connections
	// (e.g., during startup, shutdown, or when max_connections is reached).

	// Check if this is a PostgreSQL error with the specific error code. pgx
	// returns a connect failure wrapped in a *pgconn.ConnectError, so look
	// through the chain.
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		// ERRCODE_CANNOT_CONNECT_NOW is 57P03
		return pgErr.Code == "57P03"
	}

	// All other errors (authentication, authorization, network issues, etc.)
	// should be treated as "unreachable" (exit code 2)
	return false
}
