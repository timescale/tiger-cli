package restore

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// restoreWithPgRestore restores using the pg_restore tool for custom/tar/directory formats
func (r *Restorer) restoreWithPgRestore(ctx context.Context, connStr string, preflight *PreflightResult) error {
	// Build pg_restore command arguments
	args := r.buildPgRestoreArgs(connStr)

	// Create command
	cmd := exec.CommandContext(ctx, preflight.PgRestorePath, args...)

	// Capture output
	var stderr bytes.Buffer
	cmd.Stdout = r.options.Output
	cmd.Stderr = &stderr

	if r.options.Verbose {
		// In verbose mode, show command being executed
		fmt.Fprintf(r.options.Errors, "\nExecuting: pg_restore %s\n", strings.Join(args, " "))
	}

	// Execute pg_restore
	err := cmd.Run()
	if err != nil {
		// pg_restore returns non-zero for warnings too, check stderr
		stderrStr := stderr.String()
		if stderrStr != "" {
			return &RestoreError{
				Phase:         "restore",
				PostgresError: stderrStr,
			}
		}
		return fmt.Errorf("pg_restore failed: %w", err)
	}

	// Show warnings if any
	if stderr.Len() > 0 && r.options.Verbose {
		fmt.Fprintf(r.options.Errors, "\nWarnings:\n%s\n", stderr.String())
	}

	return nil
}

// buildPgRestoreArgs builds the command-line arguments for pg_restore
func (r *Restorer) buildPgRestoreArgs(connStr string) []string {
	args := []string{
		"--dbname=" + connStr,
	}

	// Add format flag if explicitly specified
	if r.options.Format != "" {
		switch r.options.Format {
		case "custom":
			args = append(args, "--format=custom")
		case "tar":
			args = append(args, "--format=tar")
		case "directory":
			args = append(args, "--format=directory")
		}
	}

	// Restore options
	if r.options.Clean {
		args = append(args, "--clean")
	}

	if r.options.IfExists {
		args = append(args, "--if-exists")
	}

	if r.options.NoOwner {
		args = append(args, "--no-owner")
	}

	if r.options.NoPrivileges {
		args = append(args, "--no-privileges")
	}

	if r.options.SingleTransaction {
		args = append(args, "--single-transaction")
	}

	// Parallel restore
	if r.options.Jobs > 1 {
		args = append(args, fmt.Sprintf("--jobs=%d", r.options.Jobs))
	}

	// Error handling
	if r.options.OnErrorStop {
		args = append(args, "--exit-on-error")
	}

	// Verbose output
	if r.options.Verbose {
		args = append(args, "--verbose")
	}

	// Add file path (last argument)
	args = append(args, r.options.FilePath)

	return args
}
