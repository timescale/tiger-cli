package restore

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/jackc/pgx/v5"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/password"
)

// PreflightResult contains the results of pre-flight validation checks
type PreflightResult struct {
	FileExists         bool
	FileReadable       bool
	FileSize           int64
	FileFormat         DumpFormat
	ServiceExists      bool
	ServiceAccessible  bool
	DatabaseExists     bool
	HasTimescaleDB     bool
	TimescaleDBVersion string
	PostgreSQLVersion  string
	PgRestoreAvailable bool
	PgRestorePath      string
}

// runPreflight performs pre-flight validation checks
func (r *Restorer) runPreflight(ctx context.Context) (*PreflightResult, error) {
	result := &PreflightResult{}

	// 1. Check file exists and is readable (unless stdin)
	if r.options.FilePath != "-" {
		fileInfo, err := os.Stat(r.options.FilePath)
		if err != nil {
			return result, fmt.Errorf("file not found: %w", err)
		}
		result.FileExists = true

		// Check if we can open the file
		file, err := os.Open(r.options.FilePath)
		if err != nil {
			return result, fmt.Errorf("file not readable: %w", err)
		}
		file.Close()
		result.FileReadable = true

		// Get file size (0 for directories)
		if !fileInfo.IsDir() {
			result.FileSize = fileInfo.Size()
		}
	} else {
		// stdin - assume readable
		result.FileExists = true
		result.FileReadable = true
		result.FileSize = 0 // unknown size for stdin
	}

	// 2. Detect file format
	format, err := detectFileFormat(r.options.FilePath, r.options.Format)
	if err != nil {
		return result, fmt.Errorf("failed to detect file format: %w", err)
	}
	result.FileFormat = format

	// 3. Check if pg_restore or psql is available (depending on format)
	if format.RequiresPgRestore() {
		pgRestorePath, err := exec.LookPath("pg_restore")
		if err != nil {
			return result, fmt.Errorf("pg_restore not found. Please install PostgreSQL client tools")
		}
		result.PgRestoreAvailable = true
		result.PgRestorePath = pgRestorePath
	} else if format == FormatPlain || format == FormatPlainGzip {
		// Plain SQL requires psql
		psqlPath, err := exec.LookPath("psql")
		if err != nil {
			return result, fmt.Errorf("psql not found. Please install PostgreSQL client tools")
		}
		result.PgRestoreAvailable = true // Reuse this field for psql availability
		result.PgRestorePath = psqlPath
	}

	// 4. Get service details and test connectivity
	service, err := r.getService(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to get service details: %w", err)
	}
	r.service = service
	result.ServiceExists = true

	// 5. Test database connectivity and gather metadata
	connStr, err := r.getConnectionString(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to build connection string: %w", err)
	}

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return result, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer conn.Close(ctx)
	result.ServiceAccessible = true
	result.DatabaseExists = true

	// 6. Check PostgreSQL version
	var pgVersion string
	err = conn.QueryRow(ctx, "SELECT version()").Scan(&pgVersion)
	if err != nil {
		return result, fmt.Errorf("failed to get PostgreSQL version: %w", err)
	}
	result.PostgreSQLVersion = pgVersion

	// 7. Check for TimescaleDB extension
	var timescaleVersion string
	err = conn.QueryRow(ctx, "SELECT extversion FROM pg_extension WHERE extname = 'timescaledb'").Scan(&timescaleVersion)
	if err == nil {
		result.HasTimescaleDB = true
		result.TimescaleDBVersion = timescaleVersion
	}
	// Not an error if TimescaleDB is not installed

	return result, nil
}

// getService fetches service details from the API
func (r *Restorer) getService(ctx context.Context) (api.Service, error) {
	// Get credentials
	apiKey, projectID, err := config.GetCredentials()
	if err != nil {
		return api.Service{}, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err)
	}

	// Create API client
	client, err := api.NewTigerClient(r.config, apiKey)
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to create API client: %w", err)
	}

	// Fetch service details
	resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, r.options.ServiceID)
	if err != nil {
		return api.Service{}, fmt.Errorf("failed to fetch service details: %w", err)
	}

	if resp.StatusCode() != 200 {
		return api.Service{}, fmt.Errorf("API error: %d", resp.StatusCode())
	}

	if resp.JSON200 == nil {
		return api.Service{}, fmt.Errorf("empty response from API")
	}

	return *resp.JSON200, nil
}

// getConnectionString builds a connection string for the service
func (r *Restorer) getConnectionString(ctx context.Context) (string, error) {
	role := r.options.Role
	if role == "" {
		role = "tsdbadmin"
	}

	details, err := password.GetConnectionDetails(r.service, password.ConnectionDetailsOptions{
		Pooled:       false, // Never use pooler for restore operations
		Role:         role,
		WithPassword: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to build connection string: %w", err)
	}

	return details.String(), nil
}

// printPreflightResults prints the pre-flight validation results
func (r *Restorer) printPreflightResults(result *PreflightResult) {
	fmt.Fprintln(r.options.Errors, "\nPre-flight validation:")

	// File info
	if r.options.FilePath == "-" {
		fmt.Fprintln(r.options.Errors, "✓ File: stdin (plain SQL)")
	} else {
		sizeStr := formatBytes(result.FileSize)
		if result.FileSize == 0 {
			sizeStr = "directory"
		}
		fmt.Fprintf(r.options.Errors, "✓ File: %s (%s, %s)\n",
			r.options.FilePath, sizeStr, result.FileFormat)
	}

	// Service info
	if r.service.ServiceId != nil {
		fmt.Fprintf(r.options.Errors, "✓ Service: %s (accessible)\n", *r.service.ServiceId)
	}

	// Database info
	fmt.Fprintf(r.options.Errors, "✓ Database: %s (PostgreSQL)\n",
		getTargetDatabase(r.options.Database))

	// TimescaleDB info
	if result.HasTimescaleDB {
		fmt.Fprintf(r.options.Errors, "✓ TimescaleDB: %s detected\n", result.TimescaleDBVersion)
	}

	// pg_restore/psql info (if needed)
	if result.FileFormat.RequiresPgRestore() {
		fmt.Fprintf(r.options.Errors, "✓ pg_restore: found at %s\n", result.PgRestorePath)
	} else if result.FileFormat == FormatPlain || result.FileFormat == FormatPlainGzip {
		fmt.Fprintf(r.options.Errors, "✓ psql: found at %s\n", result.PgRestorePath)
	}

	fmt.Fprintln(r.options.Errors, "\nReady to restore.")
}

// getTargetDatabase returns the target database name
func getTargetDatabase(database string) string {
	if database != "" {
		return database
	}
	return "tsdb" // default database
}
