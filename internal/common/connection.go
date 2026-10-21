package common

import (
	"context"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
)

// hasPooler reports whether a connection pooler exposes an endpoint.
func hasPooler(pooler *api.ConnectionPooler) bool {
	return pooler != nil && pooler.Endpoint != nil
}

// ReplicaPoolerWarning returns the warning to show when pooling was requested
// for a read replica with no pooler (the connection falls back to direct), or ""
// otherwise — including for a non-replica target, so callers need no kind check.
func ReplicaPoolerWarning(target *ConnectionTarget, pooled bool) string {
	if target.Kind != TargetReadReplica || !pooled || hasPooler(target.Pooler) {
		return ""
	}
	return fmt.Sprintf("read replica %q has no connection pooler; connecting directly instead", target.Name)
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

// readOnlyConnectionOption is the URL-encoded `options` query parameter that
// activates Tiger Cloud's immutable read-only connection mode.
const readOnlyConnectionOption = "options=-c%20tsdb_admin.read_only_connection%3Dtrue"

// GetConnectionDetails builds the connection details for a target: it selects
// the endpoint (the pooler when requested and available, otherwise the direct
// one) and looks up the password, from the target's credentials, when one is
// wanted. ConnectionTarget.Details adds the pooler check connecting needs.
func GetConnectionDetails(cfg *config.Config, target *ConnectionTarget, opts ConnectionDetailsOptions) (*ConnectionDetails, error) {
	if target.Endpoint == nil {
		return nil, fmt.Errorf("service endpoint not available")
	}

	endpoint := target.Endpoint
	isPooler := false
	if opts.Pooled && hasPooler(target.Pooler) {
		endpoint = target.Pooler.Endpoint
		isPooler = true
	}

	if endpoint.Host == nil || *endpoint.Host == "" {
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

	// A missing password is deliberately not fatal: pgx and psql both fall back
	// to PGPASSWORD or a ~/.pgpass entry the user manages themselves.
	if opts.WithPassword {
		if opts.InitialPassword != "" {
			details.Password = opts.InitialPassword
		} else if password, err := GetPassword(cfg, target.Credentials.PasswordTarget, opts.Role); err == nil {
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

// GetPassword fetches the password stored for the target from the configured
// password storage mechanism. It returns an error if it fails to find the
// password.
func GetPassword(cfg *config.Config, target PasswordTarget, role string) (string, error) {
	storage := GetPasswordStorage(cfg)
	password, err := storage.Get(target, role)
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

// connectReadOnly opens a read-only connection to the target once it is ready.
// The caller must close it.
func connectReadOnly(ctx context.Context, cfg *config.Config, target *ConnectionTarget, role string, pooled bool) (*pgx.Conn, error) {
	if err := target.CheckReady(); err != nil {
		return nil, err
	}

	details, err := target.Details(cfg, ConnectionDetailsOptions{
		Pooled:       pooled,
		Role:         role,
		WithPassword: true,
		ReadOnly:     true,
	})
	if err != nil {
		return nil, err
	}

	connConfig, err := pgx.ParseConfig(details.String())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}
	// Callers run parameterless statements, so the simple protocol fits.
	connConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return conn, nil
}
