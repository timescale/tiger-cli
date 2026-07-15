package common

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// replicaHasPooler reports whether a read replica exposes a pooler endpoint.
func replicaHasPooler(replica *api.ReadReplicaSet) bool {
	return replica != nil && replica.ConnectionPooler != nil && replica.ConnectionPooler.Endpoint != nil
}

// ReplicaPoolerWarning returns the single-sourced warning shown when pooling was
// requested for a replica with no pooler (the connection then falls back to
// direct, handled by buildConnectionDetails).
func ReplicaPoolerWarning(replica *api.ReadReplicaSet, pooled bool) string {
	if pooled && replica != nil && !replicaHasPooler(replica) {
		return fmt.Sprintf("read replica %q has no connection pooler; connecting directly instead", util.DerefStr(replica.Name))
	}
	return ""
}

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

	// ReadOnly forces the connection into Tiger Cloud's immutable read-only
	// mode by injecting the tsdb_admin.read_only_connection GUC as a startup
	// parameter. The GUC cannot be disabled with SET for the duration of the
	// session, so this is safe to use even when the LLM controls the SQL.
	ReadOnly bool
}

type ConnectionDetails struct {
	Role     string `json:"role,omitempty"`
	Password string `json:"password,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Database string `json:"database,omitempty"`
	IsPooler bool   `json:"is_pooler,omitempty"`
	readOnly bool
}

// RequirePooler returns an error when pooling was requested but the resolved
// connection isn't using the pooler endpoint. Callers that treat a missing
// pooler as fatal use this; the read replica path instead warns and falls back
// to a direct connection.
func (d *ConnectionDetails) RequirePooler(requested bool) error {
	if requested && !d.IsPooler {
		return fmt.Errorf("connection pooler not available for this service")
	}
	return nil
}

// readOnlyConnectionOption is the URL-encoded `options` query parameter that
// activates Tiger Cloud's immutable read-only connection mode.
const readOnlyConnectionOption = "options=-c%20tsdb_admin.read_only_connection%3Dtrue"

func GetConnectionDetails(service api.Service, opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	if service.Endpoint == nil {
		return nil, fmt.Errorf("service endpoint not available")
	}
	return buildConnectionDetails(service.Endpoint, service.ConnectionPooler, service, opts)
}

// ConnectToService resolves the service's connection details and opens a pgx
// connection using the given query execution mode. It is the shared
// service-to-connection path used by the query and schema tools. The caller
// owns the returned connection and must Close it.
func ConnectToService(ctx context.Context, service api.Service, opts ConnectionDetailsOptions, mode pgx.QueryExecMode) (*pgx.Conn, error) {
	details, err := GetConnectionDetails(service, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}
	if err := details.RequirePooler(opts.Pooled); err != nil {
		return nil, err
	}

	return connectWithDetails(ctx, details, mode)
}

// ConnectToTarget opens a pgx connection to the primary service, or to a read
// replica when replica is non-nil (credentials still resolve against the
// primary). A requested pooler that the replica lacks falls back to a direct
// connection; this function does not warn — callers surface that via
// common.ReplicaPoolerWarning. The caller owns the returned connection and must
// Close it.
func ConnectToTarget(ctx context.Context, primary api.Service, replica *api.ReadReplicaSet, opts ConnectionDetailsOptions, mode pgx.QueryExecMode) (*pgx.Conn, error) {
	if replica == nil {
		return ConnectToService(ctx, primary, opts, mode)
	}

	details, err := GetReplicaConnectionDetails(primary, *replica, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build connection string: %w", err)
	}
	return connectWithDetails(ctx, details, mode)
}

// connectWithDetails parses connection details and opens a pgx connection using
// the given query execution mode.
func connectWithDetails(ctx context.Context, details *ConnectionDetails, mode pgx.QueryExecMode) (*pgx.Conn, error) {
	connConfig, err := pgx.ParseConfig(details.String())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}
	connConfig.DefaultQueryExecMode = mode

	return pgx.ConnectConfig(ctx, connConfig)
}

// GetReplicaConnectionDetails builds connection details for a read replica set.
// Host/port come from the replica's endpoint, but the password is looked up via
// the primary, since replicas share the primary's credentials.
func GetReplicaConnectionDetails(primary api.Service, replica api.ReadReplicaSet, opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	if replica.Endpoint == nil {
		return nil, fmt.Errorf("read replica endpoint not available")
	}
	return buildConnectionDetails(replica.Endpoint, replica.ConnectionPooler, primary, opts)
}

// buildConnectionDetails selects the endpoint (pooler when requested and
// available, otherwise direct) and assembles the connection details. The
// password, if requested, is looked up against passwordService.
func buildConnectionDetails(direct *api.Endpoint, pooler *api.ConnectionPooler, passwordService api.Service, opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	endpoint := direct
	isPooler := false
	if opts.Pooled && pooler != nil && pooler.Endpoint != nil {
		endpoint = pooler.Endpoint
		isPooler = true
	}

	if endpoint == nil || endpoint.Host == nil || *endpoint.Host == "" {
		return nil, fmt.Errorf("endpoint host not available")
	}
	if endpoint.Port == nil || *endpoint.Port == 0 {
		return nil, fmt.Errorf("endpoint port not available")
	}

	details := &ConnectionDetails{
		Role:     opts.Role,
		Host:     *endpoint.Host,
		Port:     *endpoint.Port,
		Database: "tsdb", // Database is always "tsdb" for TimescaleDB/PostgreSQL services
		IsPooler: isPooler,
		readOnly: opts.ReadOnly,
	}

	if opts.WithPassword {
		if opts.InitialPassword != "" {
			details.Password = opts.InitialPassword
		} else if password, err := GetPassword(passwordService, opts.Role); err == nil {
			details.Password = password
		}
	}

	return details, nil
}

// String creates a PostgreSQL connection string from service details
func (d *ConnectionDetails) String() string {
	query := "sslmode=require"
	if d.readOnly {
		query += "&" + readOnlyConnectionOption
	}

	if d.Password == "" {
		// Build connection string without password (default behavior)
		return fmt.Sprintf("postgresql://%s@%s:%d/%s?%s", d.Role, d.Host, d.Port, d.Database, query)
	}
	// Include password in connection string
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?%s", d.Role, d.Password, d.Host, d.Port, d.Database, query)
}

// GetPassword fetches the password for the specified service from the
// configured password storage mechanism. It returns an error if it fails to
// find the password.
func GetPassword(service api.Service, role string) (string, error) {
	storage := GetPasswordStorage()
	password, err := storage.Get(service, role)
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
