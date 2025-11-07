package restore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// runTimescaleDBPreRestore executes the timescaledb_pre_restore() function
// This function prepares the database for restore by:
// - Disabling compression policies
// - Pausing background jobs
// - Setting up the database for optimal restore performance
func (r *Restorer) runTimescaleDBPreRestore(ctx context.Context, connStr string) error {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer conn.Close(ctx)

	// Execute timescaledb_pre_restore()
	_, err = conn.Exec(ctx, "SELECT public.timescaledb_pre_restore()")
	if err != nil {
		return fmt.Errorf("failed to execute timescaledb_pre_restore(): %w", err)
	}

	return nil
}

// runTimescaleDBPostRestore executes the timescaledb_post_restore() function
// This function cleans up after restore by:
// - Re-enabling compression policies
// - Resuming background jobs
// - Rebuilding necessary metadata
func (r *Restorer) runTimescaleDBPostRestore(ctx context.Context, connStr string) error {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer conn.Close(ctx)

	// Execute timescaledb_post_restore()
	_, err = conn.Exec(ctx, "SELECT public.timescaledb_post_restore()")
	if err != nil {
		return fmt.Errorf("failed to execute timescaledb_post_restore(): %w", err)
	}

	return nil
}
