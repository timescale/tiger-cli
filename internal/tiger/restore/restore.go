package restore

import (
	"context"
	"fmt"
	"io"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"go.uber.org/zap"
)

// Options contains configuration for database restore operations
type Options struct {
	ServiceID          string
	Database           string
	Role               string
	FilePath           string // "-" for stdin
	Format             string // plain, custom, directory, tar (empty = auto-detect)
	Clean              bool
	IfExists           bool
	NoOwner            bool
	NoPrivileges       bool
	TimescaleDBHooks   bool
	NoTimescaleDBHooks bool
	Confirm            bool
	OnErrorStop        bool
	SingleTransaction  bool
	Quiet              bool
	Verbose            bool
	Jobs               int // parallel restore jobs for custom/directory/tar formats
	Output             io.Writer
	Errors             io.Writer
}

// Restorer orchestrates database restore operations
type Restorer struct {
	config  *config.Config
	options *Options
	logger  *zap.Logger
	service api.Service
}

// NewRestorer creates a new Restorer instance
func NewRestorer(cfg *config.Config, opts *Options) *Restorer {
	return &Restorer{
		config:  cfg,
		options: opts,
		logger:  zap.L(),
	}
}

// Execute performs the complete restore operation
func (r *Restorer) Execute(ctx context.Context) error {
	// 1. Pre-flight validation
	if !r.options.Quiet {
		fmt.Fprintf(r.options.Errors, "⚙️  Preparing restore...\n")
	}

	preflight, err := r.runPreflight(ctx)
	if err != nil {
		return fmt.Errorf("pre-flight validation failed: %w", err)
	}

	if !r.options.Quiet && r.options.Verbose {
		// Only show detailed preflight in verbose mode
		r.printPreflightResults(preflight)
	}

	// 2. Confirmation for destructive operations
	if r.options.Clean && !r.options.Confirm {
		if err := r.confirmDestructive(); err != nil {
			return err
		}
	}

	// 3. Get connection string
	connStr, err := r.getConnectionString(ctx)
	if err != nil {
		return err
	}

	// 4. Run TimescaleDB pre-restore hook
	shouldUseHooks := r.shouldUseTimescaleDBHooks(preflight)
	if shouldUseHooks {
		if !r.options.Quiet && r.options.Verbose {
			fmt.Fprintln(r.options.Errors, "Running TimescaleDB pre-restore hooks...")
		}
		if err := r.runTimescaleDBPreRestore(ctx, connStr); err != nil {
			return fmt.Errorf("timescaledb pre-restore failed: %w", err)
		}
		if !r.options.Quiet && r.options.Verbose {
			fmt.Fprintln(r.options.Errors, "✓ TimescaleDB pre-restore hooks executed")
		}
	}

	// 5. Execute restore with progress tracking
	if !r.options.Quiet {
		fmt.Fprintf(r.options.Errors, "📦 Restoring database...\n")
	}

	if err := r.executeRestore(ctx, connStr, preflight); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}

	// 6. Run TimescaleDB post-restore hook
	if shouldUseHooks {
		if !r.options.Quiet && r.options.Verbose {
			fmt.Fprintln(r.options.Errors, "\nRunning TimescaleDB post-restore hooks...")
		}
		if err := r.runTimescaleDBPostRestore(ctx, connStr); err != nil {
			return fmt.Errorf("timescaledb post-restore failed: %w", err)
		}
		if !r.options.Quiet && r.options.Verbose {
			fmt.Fprintln(r.options.Errors, "✓ TimescaleDB post-restore hooks executed")
		}
	}

	// 7. Print summary - moved to psql.go to combine with restore stats

	return nil
}

// executeRestore dispatches to the appropriate restore method based on format
func (r *Restorer) executeRestore(ctx context.Context, connStr string, preflight *PreflightResult) error {
	format := preflight.FileFormat

	switch format {
	case FormatPlain, FormatPlainGzip:
		return r.restorePlainSQLWithPsql(ctx, connStr, preflight)
	case FormatCustom, FormatTar, FormatDirectory:
		return r.restoreWithPgRestore(ctx, connStr, preflight)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// shouldUseTimescaleDBHooks determines if TimescaleDB hooks should be run
func (r *Restorer) shouldUseTimescaleDBHooks(preflight *PreflightResult) bool {
	// Explicit flags take precedence
	if r.options.NoTimescaleDBHooks {
		return false
	}
	if r.options.TimescaleDBHooks {
		return true
	}

	// Auto-detect: use hooks if TimescaleDB is installed
	return preflight.HasTimescaleDB
}
