package common

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/timescale/tiger-cli/internal/config"
)

// FetchServiceSchema opens a read-only connection to the target (a primary
// service or one of its read replicas) and introspects its schema. It is the
// shared entry point for the `tiger db schema` CLI command and the db_schema
// MCP tool.
//
// The connection is forced read-only: introspection only issues SELECTs, so
// this is always safe and guards against accidental writes.
func FetchServiceSchema(ctx context.Context, cfg *config.Config, target *ConnectionTarget, role string, pooled bool, opts SchemaOptions) (*DatabaseSchema, error) {
	if err := CheckServiceReady(target.ConnectionService); err != nil {
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
	// Introspection runs parameterless statements, so the simple protocol fits.
	connConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	conn, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer conn.Close(context.Background())

	ident := SchemaIdent{
		ID:   target.ConnectionService.ServiceID,
		Name: target.ConnectionService.Name,
	}
	return FetchSchemaFromConn(ctx, conn, ident, opts)
}
