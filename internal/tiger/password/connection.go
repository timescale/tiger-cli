package password

import (
	"fmt"
	"io"

	"github.com/timescale/tiger-cli/internal/tiger/api"
)

// PasswordMode determines how passwords are handled in connection strings
type PasswordMode int

const (
	// PasswordExclude means don't include password in connection string (default)
	// Connection will rely on PGPASSWORD env var or ~/.pgpass file
	PasswordExclude PasswordMode = iota

	// PasswordRequired means include password in connection string, return error if unavailable
	// Used when user explicitly requests --with-password flag
	PasswordRequired

	// PasswordOptional means include password if available, but don't error if unavailable
	// Used for connection testing and psql launching where we want best-effort password inclusion
	PasswordOptional
)

// GetPasswordMode is a helper function for getting a [PasswordMode] from a
// boolean. It only ever returns [PasswordRequired]/[PasswordExcluded]. If you
// need [PasswordOptional], do not use this function.
func GetPasswordMode(required bool) PasswordMode {
	if required {
		return PasswordRequired
	}
	return PasswordExclude
}

// ConnectionStringOptions configures how the connection string is built
type ConnectionStringOptions struct {
	// Pooled determines whether to use the pooler endpoint (if available)
	Pooled bool

	// Role is the database role/username to use (e.g., "tsdbadmin")
	Role string

	// PasswordMode determines how passwords are handled
	PasswordMode PasswordMode

	// InitialPassword is an optional password to use directly (e.g., from service creation response)
	// If provided and PasswordMode is PasswordRequired or PasswordOptional, this password will be used
	// instead of fetching from password storage. This is useful when password_storage=none.
	InitialPassword string

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

	// Use pooler endpoint if requested and available, otherwise use direct endpoint
	var endpoint *api.Endpoint
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
	host := *endpoint.Host

	port := 5432 // Default PostgreSQL port
	if endpoint.Port != nil {
		port = *endpoint.Port
	}

	// Database is always "tsdb" for TimescaleDB/PostgreSQL services
	database := "tsdb"

	// Build connection string in PostgreSQL URI format
	switch opts.PasswordMode {
	case PasswordRequired:
		// Password is required - use InitialPassword if provided, otherwise fetch from storage
		var password string
		if opts.InitialPassword != "" {
			password = opts.InitialPassword
		} else {
			var err error
			if password, err = GetPassword(service); err != nil {
				return "", err
			}
		}

		// Include password in connection string
		return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=require", opts.Role, password, host, port, database), nil
	case PasswordOptional:
		// Try to include password - use InitialPassword if provided, otherwise try fetching from storage
		var password string
		if opts.InitialPassword != "" {
			password = opts.InitialPassword
		} else {
			password, _ = GetPassword(service) // Ignore error for optional mode
		}

		// Only include password if we have one
		if password != "" {
			return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=require", opts.Role, password, host, port, database), nil
		} else {
			// Fall back to connection string without password
			return fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=require", opts.Role, host, port, database), nil
		}
	default: // PasswordExclude
		// Build connection string without password (default behavior)
		// Password is handled separately via PGPASSWORD env var or ~/.pgpass file
		// This ensures credentials are never visible in process arguments
		return fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=require", opts.Role, host, port, database), nil
	}
}

// GetPassword fetches the password for the specified service from the
// configured password storage mechanism. It returns an error if it fails to
// find the password.
func GetPassword(service api.Service) (string, error) {
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
	return password, nil
}
