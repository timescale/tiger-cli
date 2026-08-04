package mcp

import (
	"context"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/logging"
	"github.com/timescale/tiger-cli/internal/util"
)

// DBSchemaInput represents input for db_schema
type DBSchemaInput struct {
	ServiceID   string `json:"service_id"`
	SchemaName  string `json:"schema,omitempty"`
	Internal    bool   `json:"internal,omitempty"`
	Definitions bool   `json:"definitions,omitempty"`
	Comments    bool   `json:"comments,omitempty"`
	Role        string `json:"role,omitempty"`
	Pooled      bool   `json:"pooled,omitempty"`
}

func (DBSchemaInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[DBSchemaInput](nil))

	schema.Properties["service_id"].Description = "Unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs. A read replica set ID is also accepted here — passing one introspects that read replica instead of the primary service."
	schema.Properties["service_id"].Examples = []any{"e6ue9697jf", "u8me885b93"}
	schema.Properties["service_id"].Pattern = "^[a-z0-9]{10}$"

	schema.Properties["schema"].Description = "Restrict output to a single schema (namespace). When omitted, all accessible schemas are returned."
	schema.Properties["schema"].Examples = []any{"public"}

	schema.Properties["internal"].Description = "Include system schemas (pg_*, information_schema, TimescaleDB internals) and extension-owned objects."
	schema.Properties["internal"].Default = util.Must(json.Marshal(false))

	schema.Properties["definitions"].Description = "Include full object definitions (view SELECTs, function/procedure bodies)."
	schema.Properties["definitions"].Default = util.Must(json.Marshal(false))

	schema.Properties["comments"].Description = "Include object comments (COMMENT ON text)."
	schema.Properties["comments"].Default = util.Must(json.Marshal(false))

	schema.Properties["role"].Description = "Database role/username to connect as"
	schema.Properties["role"].Default = util.Must(json.Marshal("tsdbadmin"))
	schema.Properties["role"].Examples = []any{"tsdbadmin", "readonly", "postgres"}

	schema.Properties["pooled"].Description = "Use connection pooling (if available)"
	schema.Properties["pooled"].Default = util.Must(json.Marshal(false))
	schema.Properties["pooled"].Examples = []any{false, true}

	return schema
}

// DBSchemaOutput represents output for db_schema
type DBSchemaOutput struct {
	SchemaText string `json:"schema"`
	Warning    string `json:"warning,omitempty"`
}

func (DBSchemaOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[DBSchemaOutput](nil))

	schema.Properties["schema"].Description = "The database schema rendered as human-readable text, grouped under a SCHEMA header per namespace."

	schema.Properties["warning"].Description = "Present when connection pooling was requested for a read replica that has none; the schema was read over a direct connection instead."

	return schema
}

func newDBSchemaTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "db_schema",
		Title: "Show Database Schema",
		Description: `Display the schema of a service database.

Connects to a PostgreSQL/TimescaleDB service in Tiger Cloud and returns its schema as readable text: tables (regular, partitioned, and foreign), views, materialized views, enum types, functions, procedures, indexes, triggers, and TimescaleDB hypertable and continuous aggregate metadata. Only objects the connecting role can access are returned.

By default only user-facing schemas and objects are shown; view/routine definitions and object comments are omitted unless requested. The connection is opened in immutable read-only mode.`,
		InputSchema:  DBSchemaInput{}.Schema(),
		OutputSchema: DBSchemaOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: util.Ptr(true),
			Title:         "Show Database Schema",
		},
	}
}

// handleDBSchema handles the db_schema MCP tool
func (s *Server) handleDBSchema(ctx context.Context, req *mcp.CallToolRequest, input DBSchemaInput) (*mcp.CallToolResult, DBSchemaOutput, error) {
	cfg, err := common.LoadConfig(ctx, s.flags)
	if err != nil {
		return nil, DBSchemaOutput{}, err
	}

	logging.Debug("MCP: Getting database schema",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID),
		zap.String("schema", input.SchemaName),
		zap.Bool("internal", input.Internal),
		zap.Bool("definitions", input.Definitions),
		zap.Bool("comments", input.Comments),
		zap.String("role", input.Role),
		zap.Bool("pooled", input.Pooled),
	)

	// service_id may name a service or one of its read replicas.
	target, err := common.ResolveConnectionTargetByID(ctx, cfg.Client, cfg.ProjectID, input.ServiceID)
	if err != nil {
		return nil, DBSchemaOutput{}, err
	}

	// A replica without a pooler connects directly; surface that as a warning.
	warning := common.ReplicaPoolerWarning(target, input.Pooled)

	schema, err := common.FetchServiceSchema(ctx, cfg.Config, target, input.Role, input.Pooled, common.SchemaOptions{
		Schema:             input.SchemaName,
		IncludeInternal:    input.Internal,
		IncludeDefinitions: input.Definitions,
		IncludeComments:    input.Comments,
	})
	if err != nil {
		return nil, DBSchemaOutput{}, err
	}

	return nil, DBSchemaOutput{SchemaText: common.FormatSchema(schema), Warning: warning}, nil
}
