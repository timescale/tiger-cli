package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

const (
	// mcpMaxResponseBytes caps total serialized row data per response, catching a
	// few very wide rows the row cap alone would miss. Not user-configurable.
	mcpMaxResponseBytes = 256 * 1024
)

// DBQueryInput represents input for db_query
type DBQueryInput struct {
	ServiceID      string   `json:"service_id"`
	Query          string   `json:"query,omitempty"`
	File           string   `json:"file,omitempty"`
	Parameters     []string `json:"parameters,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	Role           string   `json:"role,omitempty"`
	Pooled         bool     `json:"pooled,omitempty"`
}

func (DBQueryInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[DBQueryInput](nil))

	schema.Properties["service_id"].Description = "Unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs. A read replica set ID is also accepted here — passing one runs the query against that read replica (which is read-only) instead of the primary service."
	schema.Properties["service_id"].Examples = []any{"e6ue9697jf", "u8me885b93"}
	schema.Properties["service_id"].Pattern = "^[a-z0-9]{10}$"

	schema.Properties["query"].Description = "PostgreSQL query to execute. Exactly one of 'query' or 'file' must be provided."

	schema.Properties["file"].Description = "Path to a SQL file on disk to execute, instead of passing its contents as 'query'. Exactly one of 'query' or 'file' must be provided."
	schema.Properties["file"].Examples = []any{"./migrations/001_init.sql", "~/schema.sql"}

	schema.Properties["parameters"].Description = "Query parameters. Values are substituted for $1, $2, etc. placeholders in the query. Only supported for single-statement queries."
	schema.Properties["parameters"].Examples = []any{[]string{"1", "alice"}, []string{"2024-01-01", "100"}}

	schema.Properties["timeout_seconds"].Description = "Query timeout in seconds"
	schema.Properties["timeout_seconds"].Minimum = new(0.0)
	schema.Properties["timeout_seconds"].Default = util.Must(json.Marshal(30))
	schema.Properties["timeout_seconds"].Examples = []any{10, 30, 60}

	schema.Properties["role"].Description = "Database role/username to connect as"
	schema.Properties["role"].Default = util.Must(json.Marshal("tsdbadmin"))
	schema.Properties["role"].Examples = []any{"tsdbadmin", "readonly", "postgres"}

	schema.Properties["pooled"].Description = "Use connection pooling (if available)"
	schema.Properties["pooled"].Default = util.Must(json.Marshal(false))
	schema.Properties["pooled"].Examples = []any{false, true}

	return schema
}

// DBQueryOutput represents output for db_query
type DBQueryOutput struct {
	ResultSets    []common.ResultSet `json:"result_sets"`
	ExecutionTime string             `json:"execution_time"`
	Truncated     bool               `json:"truncated,omitempty"`
	Notice        string             `json:"notice,omitempty"`
	Warning       string             `json:"warning,omitempty"`
}

func (DBQueryOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[DBQueryOutput](nil))

	schema.Properties["result_sets"].Description = "Array of result sets returned. For single-statement queries, this array will contain one element. For multi-statement queries, this array will contain one element per statement."

	// Add descriptions for nested ResultSet fields
	resultSetSchema := schema.Properties["result_sets"].Items

	resultSetSchema.Properties["command_tag"].Description = "Identifies the type of command executed."
	resultSetSchema.Properties["command_tag"].Examples = []any{"SELECT 2", "INSERT 0 2"}

	resultSetSchema.Properties["columns"].Description = "Column metadata including name and PostgreSQL type. Omitted for commands that don't return rows (INSERT, UPDATE, DELETE, etc.)"
	resultSetSchema.Properties["columns"].Examples = []any{[]common.Column{
		{Name: "id", Type: "int4"},
		{Name: "name", Type: "text"},
		{Name: "created_at", Type: "timestamptz"},
	}}

	resultSetSchema.Properties["rows"].Description = "Result rows as arrays of values, each rendered exactly as PostgreSQL's text format does (the same text psql shows). A SQL NULL is null. Omitted for commands that don't return rows (INSERT, UPDATE, DELETE, etc.)"
	resultSetSchema.Properties["rows"].Examples = []any{[][]any{{"1", "alice", "2024-01-01 00:00:00+00"}, {"2", nil, "2024-01-02 00:00:00+00"}}}

	resultSetSchema.Properties["rows_affected"].Description = "Number of rows affected. For SELECT, this is the total number of rows the query produced; when truncated is true this exceeds the number of rows actually returned in this response. For INSERT/UPDATE/DELETE, this is the number of rows modified. Returns 0 for statements that don't return or modify rows (e.g. CREATE TABLE)."
	resultSetSchema.Properties["rows_affected"].Examples = []any{5, 42, 1000}

	resultSetSchema.Properties["truncated"].Description = "True when this result set was capped (by the configured mcp_max_rows row limit or the overall response size limit) and additional rows exist that were not returned. Refine the query in SQL to get the data you need."

	schema.Properties["execution_time"].Description = "Execution time as a human-readable duration string"
	schema.Properties["execution_time"].Examples = []any{"123ms", "1.5s", "45.2µs"}

	schema.Properties["truncated"].Description = "True when any result set was truncated to limit the amount of data returned. See notice for guidance."

	schema.Properties["notice"].Description = "Present only when results were truncated. Actionable guidance for getting the needed data via SQL (aggregate, filter, paginate) instead of re-running the query."

	schema.Properties["warning"].Description = "Present when connection pooling was requested for a read replica that has none; the query ran over a direct connection instead."

	return schema
}

func newDBQueryTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolDBQuery,
		Title: "Execute SQL Query",
		Description: `Execute SQL queries against a service database.

Connects to a PostgreSQL database service in Tiger Cloud and executes the provided SQL query, returning the results with column information, row data, and execution metadata. Pass the SQL as 'query', or point 'file' at a .sql file on disk to run its contents.

Multi-statement queries (semicolon-separated) are supported when no parameters are provided. All result sets are returned. By default, statements execute in an implicit transaction that automatically commits on success or rolls back on error. Explicit transactions (opened with BEGIN) must be explicitly committed with COMMIT, or they roll back when the connection closes.

Process data in the database, not in your context: aggregate, filter, sort/limit, and join in SQL rather than fetching raw rows.

WARNING: Can execute any SQL statement including INSERT, UPDATE, DELETE, and DDL commands. Always review queries before execution.`,
		InputSchema:  DBQueryInput{}.Schema(),
		OutputSchema: DBQueryOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // Can execute destructive SQL
			IdempotentHint:  false,     // Queries may have side effects
			OpenWorldHint:   new(true),
			Title:           "Execute SQL Query",
		},
	}
}

// handleDBQuery handles the db_query MCP tool
func (s *Server) handleDBQuery(ctx context.Context, req *mcp.CallToolRequest, input DBQueryInput) (*mcp.CallToolResult, DBQueryOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, DBQueryOutput{}, err
	}

	query, err := resolveQueryInput(input.Query, input.File)
	if err != nil {
		return nil, DBQueryOutput{}, err
	}

	// Convert timeout in seconds to time.Duration
	timeout := time.Duration(input.TimeoutSeconds) * time.Second

	s.logger.Info("MCP: Executing database query",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.String("file", input.File),
		slog.Duration("timeout", timeout),
		slog.String("role", input.Role),
		slog.Bool("pooled", input.Pooled),
		slog.String("read_only", string(cfg.ReadOnly)),
	)

	// service_id may name a service or one of its read replicas.
	target, err := common.ResolveConnectionTargetByID(ctx, client, projectID, input.ServiceID)
	if err != nil {
		return nil, DBQueryOutput{}, err
	}

	// Under prod mode the resolved target's tag decides.
	readOnlySession := common.CheckReadOnly(cfg, common.ServiceEnvironmentTag(target.ConnectionService)) != nil

	// A replica without a pooler connects directly; surface that as a warning.
	poolerWarning := common.ReplicaPoolerWarning(target, input.Pooled)

	// Create query context with timeout
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Bound how much data this call returns to the model's context. The CLI
	// command passes no caps; only an agent's context needs protecting.
	maxRows := resolveMaxRows(cfg.MCPMaxRows)

	result, err := common.ExecuteQuery(queryCtx, cfg, target, common.ExecuteQueryArgs{
		Query:      query,
		Parameters: input.Parameters,
		Role:       input.Role,
		Pooled:     input.Pooled,
		ReadOnly:   readOnlySession,
		MaxRows:    maxRows,
		MaxBytes:   mcpMaxResponseBytes,
	})
	if err != nil {
		return nil, DBQueryOutput{}, handleDatabaseError(err)
	}

	output := DBQueryOutput{
		ResultSets:    result.ResultSets,
		ExecutionTime: result.ExecutionTime.String(),
		Warning:       poolerWarning,
	}
	if result.Truncated {
		output.Truncated = true
		output.Notice = truncationNotice(maxRows)
	}

	return nil, output, nil
}

// resolveQueryInput returns the SQL to run, read from file when that's what the
// caller passed instead of inline query text.
func resolveQueryInput(query, file string) (string, error) {
	if (query == "") == (file == "") {
		return "", errors.New("exactly one of 'query' or 'file' must be provided")
	}
	if file == "" {
		return query, nil
	}

	data, err := os.ReadFile(util.ExpandPath(file))
	if err != nil {
		return "", fmt.Errorf("failed to read SQL file: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("SQL file %s is empty", file)
	}
	// Whitespace and comments go through to the database, which reports on
	// them the way it would for any other client.
	return string(data), nil
}

// resolveMaxRows returns the row cap from mcp_max_rows, falling back to the
// default for non-positive config-file/env values (which skip set validation).
func resolveMaxRows(configured int) int {
	if configured <= 0 {
		return config.DefaultMCPMaxRows
	}
	return configured
}

// truncationNotice builds the actionable guidance returned to the model when a
// response is truncated.
func truncationNotice(maxRows int) string {
	return fmt.Sprintf("Results were truncated to limit the amount of data returned (the configured mcp_max_rows=%d per result set, plus an overall response size cap). More rows exist. Do the work in the database instead of re-running this query: aggregate (GROUP BY, COUNT, SUM, AVG), filter (WHERE), or paginate (LIMIT/OFFSET).", maxRows)
}
