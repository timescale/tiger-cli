package util

import (
	"fmt"
	"io"

	"github.com/timescale/tiger-cli/internal/tiger/api"
)

// ConnectionStringOptions configures how the connection string is built
type ConnectionStringOptions struct {
	// Pooled determines whether to use the pooler endpoint (if available)
	Pooled bool

	// Role is the database role/username to use (e.g., "tsdbadmin")
	Role string

	// WithPassword determines whether to include the password in the connection string
	// If false, the connection string will not include a password (for use with PGPASSWORD env var or ~/.pgpass)
	WithPassword bool

	// WarnWriter is an optional writer for warning messages (e.g., when pooler is requested but not available)
	// If nil, warnings are suppressed
	WarnWriter io.Writer
}

// BuildConnectionString creates a PostgreSQL connection string from service details
//
// The function supports various configuration options through ConnectionStringOptions:
// - Pooled connections (if available on the service)
// - With or without password embedded in the URI
// - Custom database role/username
// - Optional warning output when pooler is unavailable
//
// Examples:
//
//	// Simple connection string without password (for use with PGPASSWORD or ~/.pgpass)
//	connStr, err := BuildConnectionString(service, ConnectionStringOptions{
//		Role: "tsdbadmin",
//		WithPassword: false,
//	})
//
//	// Connection string with password embedded
//	connStr, err := BuildConnectionString(service, ConnectionStringOptions{
//		Role: "tsdbadmin",
//		WithPassword: true,
//	})
//
//	// Pooled connection with warnings
//	connStr, err := BuildConnectionString(service, ConnectionStringOptions{
//		Pooled: true,
//		Role: "tsdbadmin",
//		WithPassword: true,
//		WarnWriter: os.Stderr,
//	})
func BuildConnectionString(service api.Service, opts ConnectionStringOptions) (string, error) {
	if service.Endpoint == nil {
		return "", fmt.Errorf("service endpoint not available")
	}

	var endpoint *api.Endpoint
	var host string
	var port int

	// Use pooler endpoint if requested and available, otherwise use direct endpoint
	if opts.Pooled && service.ConnectionPooler != nil && service.ConnectionPooler.Endpoint != nil {
		endpoint = service.ConnectionPooler.Endpoint
	} else {
		// If pooled was requested but no pooler is available, warn if writer is provided
		if opts.Pooled && opts.WarnWriter != nil {
			fmt.Fprintf(opts.WarnWriter, "⚠️  Warning: Connection pooler not available for this service, using direct connection\n")
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
	var connectionString string
	if opts.WithPassword {
		// Get password from storage
		storage := GetPasswordStorage()
		password, err := storage.Get(service)
		if err != nil {
			// Provide specific error messages based on storage type
			switch storage.(type) {
			case *NoStorage:
				return "", fmt.Errorf("password storage is disabled (--password-storage=none)")
			case *KeyringStorage:
				return "", fmt.Errorf("no password found in keyring for this service")
			case *PgpassStorage:
				return "", fmt.Errorf("no password found in ~/.pgpass for this service")
			default:
				return "", fmt.Errorf("failed to retrieve password: %w", err)
			}
		}

		if password == "" {
			return "", fmt.Errorf("no password available for service")
		}

		// Include password in connection string
		connectionString = fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=require", opts.Role, password, host, port, database)
	} else {
		// Build connection string without password (default behavior)
		// Password is handled separately via PGPASSWORD env var or ~/.pgpass file
		// This ensures credentials are never visible in process arguments
		connectionString = fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=require", opts.Role, host, port, database)
	}

	return connectionString, nil
}
