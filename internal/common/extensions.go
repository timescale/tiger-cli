package common

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/timescale/tiger-cli/internal/config"
)

// Extension is an extension installed in a database.
type Extension struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Schema      string `json:"schema"`
	Description string `json:"description,omitempty"`
}

const extensionsQuery = `SELECT e.extname, e.extversion, n.nspname,
       coalesce(obj_description(e.oid, 'pg_extension'), '')
FROM pg_extension e
JOIN pg_namespace n ON n.oid = e.extnamespace
ORDER BY e.extname`

// FetchExtensions lists the extensions installed in the target's database,
// sorted by name. It is a variable so tests can replace it and run without a
// real database.
var FetchExtensions = func(ctx context.Context, cfg *config.Config, target *ConnectionTarget, role string) ([]Extension, error) {
	// The list of extensions is the same through the pooler, so connect directly.
	conn, err := connectReadOnly(ctx, cfg, target, role, false)
	if err != nil {
		return nil, err
	}
	defer conn.Close(context.Background())

	rows, err := conn.Query(ctx, extensionsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch extensions: %w", err)
	}
	extensions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Extension, error) {
		var ext Extension
		err := row.Scan(&ext.Name, &ext.Version, &ext.Schema, &ext.Description)
		return ext, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch extensions: %w", err)
	}
	return extensions, nil
}
