package mcp

import (
	"context"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// DBExtensionsListInput represents input for db_extensions_list
type DBExtensionsListInput struct {
	ServiceID string `json:"service_id"`
	Role      string `json:"role,omitempty"`
}

func (DBExtensionsListInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[DBExtensionsListInput](nil))

	schema.Properties["service_id"].Description = "Unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs. A read replica set ID is also accepted here — passing one lists the extensions on that read replica instead of the primary service."
	schema.Properties["service_id"].Examples = []any{"e6ue9697jf", "u8me885b93"}
	schema.Properties["service_id"].Pattern = "^[a-z0-9]{10}$"

	schema.Properties["role"].Description = "Database role/username to connect as"
	schema.Properties["role"].Default = util.Must(json.Marshal("tsdbadmin"))
	schema.Properties["role"].Examples = []any{"tsdbadmin", "readonly", "postgres"}

	return schema
}

// DBExtensionsListOutput represents output for db_extensions_list
type DBExtensionsListOutput struct {
	Extensions []common.Extension `json:"extensions"`
}

func (DBExtensionsListOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[DBExtensionsListOutput](nil))

	schema.Properties["extensions"].Description = "The extensions installed in the database, ordered by name."

	ext := schema.Properties["extensions"].Items
	ext.Properties["name"].Description = "Extension name, as used in CREATE EXTENSION."
	ext.Properties["name"].Examples = []any{"timescaledb", "vector", "postgis"}

	ext.Properties["version"].Description = "Installed version of the extension."
	ext.Properties["version"].Examples = []any{"2.21.0", "0.8.0"}

	ext.Properties["schema"].Description = "Schema the extension's objects are installed in."
	ext.Properties["schema"].Examples = []any{"public", "pg_catalog"}

	ext.Properties["description"].Description = "The extension's description (its COMMENT ON EXTENSION text), omitted when it has none."
	ext.Properties["description"].Examples = []any{
		"Enables scalable inserts and complex queries for time-series data",
		"vector data type and ivfflat and hnsw access methods",
	}

	return schema
}

func newDBExtensionsListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolDBExtensionsList,
		Title: "List Database Extensions",
		Description: `List the PostgreSQL extensions installed in a service database.

Connects to a PostgreSQL/TimescaleDB service in Tiger Cloud and returns each installed extension's name, version, schema, and description. The connection is opened in immutable read-only mode.`,
		InputSchema:  DBExtensionsListInput{}.Schema(),
		OutputSchema: DBExtensionsListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "List Database Extensions",
		},
	}
}

// handleDBExtensionsList handles the db_extensions_list MCP tool
func (s *Server) handleDBExtensionsList(ctx context.Context, req *mcp.CallToolRequest, input DBExtensionsListInput) (*mcp.CallToolResult, DBExtensionsListOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, DBExtensionsListOutput{}, err
	}

	target, err := common.ResolveConnectionTargetByID(ctx, client, projectID, input.ServiceID)
	if err != nil {
		return nil, DBExtensionsListOutput{}, err
	}

	extensions, err := common.FetchExtensions(ctx, cfg, target, input.Role)
	if err != nil {
		return nil, DBExtensionsListOutput{}, handleDatabaseError(err)
	}

	return nil, DBExtensionsListOutput{Extensions: extensions}, nil
}
