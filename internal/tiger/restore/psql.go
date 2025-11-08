package restore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/timescale/tiger-cli/internal/tiger/password"
)

// restorePlainSQLWithPsql restores a plain SQL dump using psql
func (r *Restorer) restorePlainSQLWithPsql(ctx context.Context, connStr string, preflight *PreflightResult) error {
	startTime := getNow()
	// Check if psql is available
	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		return fmt.Errorf("psql client not found. Please install PostgreSQL client tools")
	}

	// Build psql command arguments
	args := []string{connStr}

	// Add options based on restore settings
	if r.options.OnErrorStop {
		args = append(args, "--set", "ON_ERROR_STOP=1")
	} else {
		// When not stopping on errors, add helpful psql variable settings
		// ON_ERROR_ROLLBACK=on always rolls back failed statements but continues
		// This is useful for cloud environments where some objects may already exist
		args = append(args, "--set", "ON_ERROR_ROLLBACK=on")
	}

	if r.options.SingleTransaction {
		args = append(args, "--single-transaction")
	}

	// Control output verbosity
	if r.options.Quiet {
		args = append(args, "--quiet")
		// Also suppress NOTICE messages
		args = append(args, "--set", "VERBOSITY=terse")
	} else if !r.options.Verbose {
		// Default: show only errors, suppress successful command output
		// --quiet suppresses meta-commands and table results
		args = append(args, "--quiet")
		// VERBOSITY=terse makes error messages more concise
		args = append(args, "--set", "VERBOSITY=terse")
	} else {
		// Verbose mode: show everything including commands being executed
		args = append(args, "--echo-all")
	}

	// Add file input (or stdin)
	if r.options.FilePath == "-" {
		// Read from stdin - psql will automatically read from stdin if no file specified
		args = append(args, "--file", "-")
	} else {
		args = append(args, "--file", r.options.FilePath)
	}

	// Create command
	cmd := exec.CommandContext(ctx, psqlPath, args...)

	// Set up I/O
	if r.options.Verbose {
		// Verbose mode: show everything
		cmd.Stdout = r.options.Output
		cmd.Stderr = r.options.Errors
	} else if r.options.Quiet {
		// Quiet mode: suppress everything
		cmd.Stdout = nil
		cmd.Stderr = nil
	} else {
		// Default mode: suppress all output (both stdout and stderr)
		// Errors are expected and tolerated unless ON_ERROR_STOP=1 is set in the environment
		// Only show final success/failure summary
		cmd.Stdout = nil
		cmd.Stderr = nil
	}

	if r.options.FilePath == "-" {
		cmd.Stdin = os.Stdin
	}

	// Set PGPASSWORD environment variable if using keyring storage
	storage := password.GetPasswordStorage()
	if _, isKeyring := storage.(*password.KeyringStorage); isKeyring {
		if r.service == nil {
			return fmt.Errorf("service is not initialized; cannot retrieve password")
		}
		if pwd, err := storage.Get(r.service, r.options.Role); err == nil && pwd != "" {
			cmd.Env = append(os.Environ(), "PGPASSWORD="+pwd)
		}
	}

	// Execute psql
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql execution failed: %w", err)
	}

	// Calculate duration
	duration := getNow().Sub(startTime)

	// Show restore summary unless quiet mode
	if !r.options.Quiet {
		fmt.Fprintln(r.options.Errors) // blank line
		if err := r.printRestoreSummary(ctx, connStr, duration); err != nil {
			// Don't fail the restore if we can't get summary stats
			fmt.Fprintf(r.options.Errors, "⚠️  Warning: Could not retrieve restore summary: %v\n", err)
		}
	}

	return nil
}

// RestoreSummary contains statistics about what was restored
type RestoreSummary struct {
	Tables       int
	Views        int
	Functions    int
	Sequences    int
	Indexes      int
	Hypertables  int
	TotalRows    int64
	HasTimescale bool
}

// printRestoreSummary queries the database and prints a summary of restored objects
func (r *Restorer) printRestoreSummary(ctx context.Context, connStr string, duration time.Duration) error {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	summary := &RestoreSummary{}

	// Count tables in public schema
	err = conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_tables
		WHERE schemaname = 'public'
	`).Scan(&summary.Tables)
	if err != nil {
		return err
	}

	// Count views in public schema
	err = conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_views
		WHERE schemaname = 'public'
	`).Scan(&summary.Views)
	if err != nil {
		return err
	}

	// Count functions in public schema
	err = conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_proc p
		JOIN pg_namespace n ON p.pronamespace = n.oid
		WHERE n.nspname = 'public'
	`).Scan(&summary.Functions)
	if err != nil {
		return err
	}

	// Count sequences in public schema
	err = conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_sequences
		WHERE schemaname = 'public'
	`).Scan(&summary.Sequences)
	if err != nil {
		return err
	}

	// Count indexes in public schema
	err = conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_indexes
		WHERE schemaname = 'public'
	`).Scan(&summary.Indexes)
	if err != nil {
		return err
	}

	// Check for TimescaleDB and count hypertables
	var hasTimescale bool
	err = conn.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_extension WHERE extname = 'timescaledb'
		)
	`).Scan(&hasTimescale)
	if err == nil && hasTimescale {
		summary.HasTimescale = true
		err = conn.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM timescaledb_information.hypertables
			WHERE hypertable_schema = 'public'
		`).Scan(&summary.Hypertables)
		if err != nil {
			return err
		}
	}

	// Estimate total rows across all tables in public schema
	err = conn.QueryRow(ctx, `
		SELECT COALESCE(SUM(n_live_tup), 0)
		FROM pg_stat_user_tables
		WHERE schemaname = 'public'
	`).Scan(&summary.TotalRows)
	if err != nil {
		return err
	}

	// Print sleek, minimal summary
	fmt.Fprintf(r.options.Errors, "✓ Restore completed in %s\n\n", formatDuration(duration))

	// Show compact stats on one or two lines
	var stats []string
	if summary.Tables > 0 {
		stats = append(stats, fmt.Sprintf("%d table%s", summary.Tables, pluralize(summary.Tables)))
	}
	if summary.Views > 0 {
		stats = append(stats, fmt.Sprintf("%d view%s", summary.Views, pluralize(summary.Views)))
	}
	if summary.Functions > 0 {
		stats = append(stats, fmt.Sprintf("%d function%s", summary.Functions, pluralize(summary.Functions)))
	}
	if summary.Sequences > 0 {
		stats = append(stats, fmt.Sprintf("%d sequence%s", summary.Sequences, pluralize(summary.Sequences)))
	}
	if summary.Indexes > 0 {
		stats = append(stats, fmt.Sprintf("%d index%s", summary.Indexes, pluralize(summary.Indexes)))
	}
	if summary.HasTimescale && summary.Hypertables > 0 {
		stats = append(stats, fmt.Sprintf("%d hypertable%s", summary.Hypertables, pluralize(summary.Hypertables)))
	}

	if len(stats) > 0 {
		fmt.Fprintf(r.options.Errors, "📊 ")
		for i, stat := range stats {
			if i > 0 {
				fmt.Fprintf(r.options.Errors, " • ")
			}
			fmt.Fprintf(r.options.Errors, "%s", stat)
		}
		fmt.Fprintln(r.options.Errors)
	}

	if summary.TotalRows > 0 {
		fmt.Fprintf(r.options.Errors, "📈 %s rows\n", formatRowCount(summary.TotalRows))
	}

	return nil
}

// pluralize returns "s" if count != 1, otherwise empty string
func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// formatRowCount formats row counts with thousand separators
func formatRowCount(count int64) string {
	if count < 1000 {
		return fmt.Sprintf("%d", count)
	}
	if count < 1000000 {
		return fmt.Sprintf("%.1fK", float64(count)/1000)
	}
	if count < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(count)/1000000)
	}
	return fmt.Sprintf("%.1fB", float64(count)/1000000000)
}
