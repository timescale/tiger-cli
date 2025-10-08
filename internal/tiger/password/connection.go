package password

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

	// WithPassword determines whether to include the password in the connection string.
	// When true, the password will be embedded if available (from InitialPassword or storage).
	// When false, the password is never included (connection relies on PGPASSWORD env var or ~/.pgpass).
	WithPassword bool

	// InitialPassword is an optional password to use directly (e.g., from service creation response).
	// If provided and WithPassword is true, this password will be used instead of fetching from password
	// storage. This is useful when password_storage=none.
	InitialPassword string

	// WarnWriter is an optional writer for warning messages (e.g., when pooler is requested but not available,
	// or when password is requested but not found). If nil, warnings are suppressed.
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

	// WithPassword is true - try to include password if available
	var password string
	if opts.WithPassword {
		if opts.InitialPassword != "" {
			password = opts.InitialPassword
		} else {
			// Try fetching from storage, but don't fail if unavailable
			var err error
			password, err = GetPassword(service)
			if err != nil && opts.WarnWriter != nil {
				// Password was requested but not found - issue warning if WarnWriter is provided
				fmt.Fprintf(opts.WarnWriter, "⚠️  Warning: Password was requested but could not be found: %s", err)
			}
		}
	}

	// Include password in connection string if we have one
	if password != "" {
		return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=require", opts.Role, password, host, port, database), nil
	}

	// Fall back to connection string without password
	return fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=require", opts.Role, host, port, database), nil
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
