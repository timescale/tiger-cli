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

// ConnectionDetailsOptions configures how the connection string is built
type ConnectionDetailsOptions struct {
	// Pooled determines whether to use the pooler endpoint (if available)
	Pooled bool

	// Role is the database role/username to use (e.g., "tsdbadmin")
	Role string

	// PasswordMode determines how passwords are handled
	PasswordMode PasswordMode

	// WarnWriter is an optional writer for warning messages (e.g., when pooler is requested but not available)
	// If nil, warnings are suppressed
	WarnWriter io.Writer

	// If passed, we skip fetching from the password storage
	InitialPassword string
}

type ConnectionDetails struct {
	Role     string `json:"role,omitempty" yaml:"role,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
	Host     string `json:"host,omitempty" yaml:"host,omitempty"`
	Port     int    `json:"port,omitempty" yaml:"port,omitempty"`
	Database string `json:"database,omitempty" yaml:"database,omitempty"`
}

func GetConnectionDetails(service api.Service, opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	if service.Endpoint == nil {
		return nil, fmt.Errorf("service endpoint not available")
	}

	var endpoint *api.Endpoint

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
		return nil, fmt.Errorf("endpoint host not available")
	}

	details := &ConnectionDetails{
		Role:     opts.Role,
		Host:     *endpoint.Host,
		Port:     5432,   // Default PostgreSQL port
		Database: "tsdb", // Database is always "tsdb" for TimescaleDB/PostgreSQL services
	}

	if endpoint.Port != nil {
		details.Port = *endpoint.Port
	}

	if opts.PasswordMode == PasswordRequired || opts.PasswordMode == PasswordOptional {
		var password string
		if opts.InitialPassword != "" {
			password = opts.InitialPassword
		}
		if password == "" {
			storage := GetPasswordStorage()
			if storedPassword, err := storage.Get(service); err != nil && opts.PasswordMode == PasswordRequired {
				// Provide specific error messages based on storage type
				switch storage.(type) {
				case *NoStorage:
					return nil, fmt.Errorf("password storage is disabled (--password-storage=none)")
				case *KeyringStorage:
					return nil, fmt.Errorf("no password found in keyring for this service")
				case *PgpassStorage:
					return nil, fmt.Errorf("no password found in ~/.pgpass for this service")
				default:
					return nil, fmt.Errorf("failed to retrieve password: %w", err)
				}
			} else {
				password = storedPassword
			}
		}

		if password == "" && opts.PasswordMode == PasswordRequired {
			return nil, fmt.Errorf("no password available for service")
		}

		details.Password = password
	}

	return details, nil
}

// String creates a PostgreSQL connection string from service details
func (d *ConnectionDetails) String() string {
	if d.Password == "" {
		// Build connection string without password (default behavior)
		return fmt.Sprintf("postgresql://%s@%s:%d/%s?sslmode=require", d.Role, d.Host, d.Port, d.Database)
	}
	// Include password in connection string
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=require", d.Role, d.Password, d.Host, d.Port, d.Database)
}
