package common

import (
	"context"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// hasPooler reports whether a connection pooler exposes an endpoint.
func hasPooler(pooler *api.ConnectionPooler) bool {
	return pooler != nil && pooler.Endpoint != nil
}

// ReplicaPoolerWarning returns the warning to show when pooling was requested
// for a read replica with no pooler (the connection falls back to direct), or ""
// otherwise — including for a non-replica target, so callers need no IsReplica guard.
func ReplicaPoolerWarning(target *ConnectionTarget, pooled bool) string {
	if !target.IsReplica || !pooled || hasPooler(target.Connect.ConnectionPooler) {
		return ""
	}
	return fmt.Sprintf("read replica %q has no connection pooler; connecting directly instead", util.DerefStr(target.Connect.Name))
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
	return GetConnectionDetailsFor(service, service, opts)
}

// GetConnectionDetailsFor builds connection details using connService for the
// endpoint/pooler and credService for the password lookup. For a primary the
// two are the same; for a read replica connService is the replica (its own
// endpoint) and credService is the parent primary whose credentials it shares.
func GetConnectionDetailsFor(connService, credService api.Service, opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	if connService.Endpoint == nil {
		return nil, fmt.Errorf("service endpoint not available")
	}
	return buildConnectionDetails(connService.Endpoint, connService.ConnectionPooler, credService, opts)
}

// ConnectTarget opens a pgx connection to the target (see
// ConnectionTarget.Details for the pooler policy). The caller owns the returned
// connection and must Close it.
func ConnectTarget(ctx context.Context, target *ConnectionTarget, opts ConnectionDetailsOptions, mode pgx.QueryExecMode) (*pgx.Conn, error) {
	details, err := target.Details(opts)
	if err != nil {
		return nil, err
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

	// url.User* percent-encodes the role/password so URL-special characters (e.g.
	// in a manually entered password) don't break connection-string parsing.
	userinfo := url.User(d.Role)
	if d.Password != "" {
		userinfo = url.UserPassword(d.Role, d.Password)
	}
	return fmt.Sprintf("postgresql://%s@%s:%d/%s?%s", userinfo, d.Host, d.Port, d.Database, query)
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
