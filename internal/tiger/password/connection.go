package password

import (
	"fmt"
	"io"

	"github.com/timescale/tiger-cli/internal/tiger/api"
)

// ConnectionDetailsOptions configures how the connection string is built
type ConnectionDetailsOptions struct {
	// Pooled determines whether to use the pooler endpoint (if available)
	Pooled bool

	// Role is the database role/username to use (e.g., "tsdbadmin")
	Role string

	// WithPassword determines whether to include the password in the output
	WithPassword bool

	// InitialPassword is an optional password to use directly (e.g., from service creation response)
	// If provided and WithPassword is true, this password will be used
	// instead of fetching from password storage. This is useful when password_storage=none.
	InitialPassword string

	// WarnWriter is an optional writer for warning messages (e.g., when pooler is requested but not available)
	// If nil, warnings are suppressed
	WarnWriter io.Writer
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

	if opts.WithPassword {
		if opts.InitialPassword != "" {
			details.Password = opts.InitialPassword
		} else if password, err := GetPassword(service); err == nil {
			details.Password = password
		}
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
