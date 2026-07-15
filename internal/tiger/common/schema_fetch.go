package common

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// FetchServiceSchema opens a read-only connection to the service (or one of its
// read replicas, when replica is non-nil) and introspects its schema. It is the
// shared entry point for the `tiger db schema` CLI command and the db_schema
// MCP tool.
//
// The connection is forced read-only: introspection only issues SELECTs, so
// this is always safe and guards against accidental writes.
func FetchServiceSchema(ctx context.Context, service api.Service, replica *api.ReadReplicaSet, role string, pooled bool, opts SchemaOptions) (*DatabaseSchema, error) {
	if err := CheckServiceReady(service); err != nil {
		return nil, err
	}

	// Introspection runs parameterless statements, so the simple protocol fits.
	conn, err := ConnectToTarget(ctx, service, replica, ConnectionDetailsOptions{
		Pooled:       pooled,
		Role:         role,
		WithPassword: true,
		ReadOnly:     true,
	}, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		return nil, err
	}
	defer conn.Close(context.Background())

	ident := SchemaIdent{
		ID:   util.DerefStr(service.ServiceId),
		Name: util.DerefStr(service.Name),
	}
	return FetchSchemaFromConn(ctx, conn, ident, opts)
}
