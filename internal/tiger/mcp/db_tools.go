package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// DBExecuteQueryInput represents input for db_execute_query
type DBExecuteQueryInput struct {
	ServiceID      string   `json:"service_id"`
	Query          string   `json:"query"`
	Parameters     []string `json:"parameters,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	Role           string   `json:"role,omitempty"`
	Pooled         bool     `json:"pooled,omitempty"`
}

func (DBExecuteQueryInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[DBExecuteQueryInput](nil))

	schema.Properties["service_id"].Description = "Unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs."
	schema.Properties["service_id"].Examples = []any{"e6ue9697jf", "u8me885b93"}
	schema.Properties["service_id"].Pattern = "^[a-z0-9]{10}$"

	schema.Properties["query"].Description = "PostgreSQL query to execute"

	schema.Properties["parameters"].Description = "Query parameters. Values are substituted for $1, $2, etc. placeholders in the query."
	schema.Properties["parameters"].Examples = []any{[]string{"1", "alice"}, []string{"2024-01-01", "100"}}

	schema.Properties["timeout_seconds"].Description = "Query timeout in seconds"
	schema.Properties["timeout_seconds"].Minimum = util.Ptr(0.0)
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

// DBExecuteQueryColumn represents a column in the query result
type DBExecuteQueryColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ResultSet represents a single query result set
type ResultSet struct {
	CommandTag   string                 `json:"command_tag"`
	Columns      []DBExecuteQueryColumn `json:"columns,omitempty"`
	Rows         *[][]any               `json:"rows,omitempty"`
	RowsAffected int64                  `json:"rows_affected"`
}

// DBExecuteQueryOutput represents output for db_execute_query
type DBExecuteQueryOutput struct {
	ResultSets    []ResultSet `json:"result_sets"`
	ExecutionTime string      `json:"execution_time"`
}

func (DBExecuteQueryOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[DBExecuteQueryOutput](nil))

	schema.Properties["result_sets"].Description = "Array of result sets returned. For single-statement queries, this array will contain one element. For multi-statement queries, this array will contain one element per statement."

	// Add descriptions for nested ResultSet fields
	resultSetSchema := schema.Properties["result_sets"].Items

	resultSetSchema.Properties["command_tag"].Description = "Identifies the type of command executed."
	resultSetSchema.Properties["command_tag"].Examples = []any{"SELECT 2", "INSERT 0 2"}

	resultSetSchema.Properties["columns"].Description = "Column metadata including name and PostgreSQL type. Omitted for commands that don't return rows (INSERT, UPDATE, DELETE, etc.)"
	resultSetSchema.Properties["columns"].Examples = []any{[]DBExecuteQueryColumn{
		{Name: "id", Type: "int4"},
		{Name: "name", Type: "text"},
		{Name: "created_at", Type: "timestamptz"},
	}}

	resultSetSchema.Properties["rows"].Description = "Result rows as arrays of values. Omitted for commands that don't return rows (INSERT, UPDATE, DELETE, etc.)"
	resultSetSchema.Properties["rows"].Examples = []any{[][]any{{1, "alice", "2024-01-01"}, {2, "bob", "2024-01-02"}}}

	resultSetSchema.Properties["rows_affected"].Description = "Number of rows affected. For SELECT, this is the number of rows returned. For INSERT/UPDATE/DELETE, this is the number of rows modified. Returns 0 for statements that don't return or modify rows (e.g. CREATE TABLE)."
	resultSetSchema.Properties["rows_affected"].Examples = []any{5, 42, 1000}

	schema.Properties["execution_time"].Description = "Execution time as a human-readable duration string"
	schema.Properties["execution_time"].Examples = []any{"123ms", "1.5s", "45.2µs"}

	return schema
}

// registerDatabaseTools registers database operation tools with comprehensive schemas and descriptions
func (s *Server) registerDatabaseTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "db_execute_query",
		Title: "Execute SQL Query",
		Description: `Execute SQL queries against a service database.

Connects to a PostgreSQL database service in Tiger Cloud and executes the provided SQL query, returning the results with column information, row data, and execution metadata.

Multi-statement queries (semicolon-separated) are supported when no parameters are provided. All result sets are returned. By default, statements execute in an implicit transaction that automatically commits on success or rolls back on error. Explicit transactions (opened with BEGIN) must be explicitly committed with COMMIT, or they roll back when the connection closes.

WARNING: Can execute any SQL statement including INSERT, UPDATE, DELETE, and DDL commands. Always review queries before execution.`,
		InputSchema:  DBExecuteQueryInput{}.Schema(),
		OutputSchema: DBExecuteQueryOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: util.Ptr(true), // Can execute destructive SQL
			IdempotentHint:  false,          // Queries may have side effects
			OpenWorldHint:   util.Ptr(true),
			Title:           "Execute SQL Query",
		},
	}, s.handleDBExecuteQuery)
}

// handleDBExecuteQuery handles the db_execute_query MCP tool
func (s *Server) handleDBExecuteQuery(ctx context.Context, req *mcp.CallToolRequest, input DBExecuteQueryInput) (*mcp.CallToolResult, DBExecuteQueryOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, DBExecuteQueryOutput{}, err
	}

	// Convert timeout in seconds to time.Duration
	timeout := time.Duration(input.TimeoutSeconds) * time.Second

	logging.Debug("MCP: Executing database query",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID),
		zap.Duration("timeout", timeout),
		zap.String("role", input.Role),
		zap.Bool("pooled", input.Pooled),
	)

	// Get service details to construct connection string
	serviceResp, err := cfg.Client.GetServiceWithResponse(ctx, cfg.ProjectID, input.ServiceID)
	if err != nil {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("failed to get service details: %w", err)
	}

	if serviceResp.StatusCode() != http.StatusOK {
		return nil, DBExecuteQueryOutput{}, serviceResp.JSON4XX
	}

	if serviceResp.JSON200 == nil {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("empty response from API")
	}

	service := *serviceResp.JSON200

	// Build connection string with password
	details, err := common.GetConnectionDetails(service, common.ConnectionDetailsOptions{
		Pooled:       input.Pooled,
		Role:         input.Role,
		WithPassword: true,
	})
	if err != nil {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("failed to build connection string: %w", err)
	}
	if input.Pooled && !details.IsPooler {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("connection pooler not available for service %s", input.ServiceID)
	}

	// Create query context with timeout
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Parse connection string into config
	connConfig, err := pgx.ParseConfig(details.String())
	if err != nil {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Choose query execution mode based on whether parameters are present.
	// Simple protocol supports multi-statement queries but interpolates
	// parameters client-side (which we don't want to do, for security's sake).
	// Extended protocol sends parameters separately but doesn't support
	// multi-statement queries. This means we don't support multi-statement
	// queries with parameters (pgx will return an error for them when using
	// QueryExecModeDescribeExec). See [pgx.QueryExecMode] for details.
	if len(input.Parameters) > 0 {
		// Use extended protocol to send parameters separately (more secure,
		// but doesn't support multi-statement queries).
		connConfig.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec
	} else {
		// Use simple protocol to support multi-statement queries when no
		// parameters are given.
		connConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	// Connect to database
	conn, err := pgx.ConnectConfig(queryCtx, connConfig)
	if err != nil {
		return nil, DBExecuteQueryOutput{}, err
	}
	defer conn.Close(context.Background())

	// Execute query and measure time
	startTime := time.Now()

	// Queue the query. When using QueryExecModeSimpleProtocol (no parameters),
	// it's valid to queue a single multi-statement SQL query as the batch.
	// See the [pgx.Batch.Queue] documentation for details. When using
	// QueryExecModeDescribeExec (with parameters), queueing a multi-statement
	// query here will result in an error when executing it below.
	batch := &pgx.Batch{}
	batch.Queue(input.Query, util.ConvertSliceToAny(input.Parameters)...)

	br := conn.SendBatch(queryCtx, batch)
	defer br.Close()

	// Process all result sets, collecting them all
	resultSets := make([]ResultSet, 0)
	for {
		rows, err := br.Query()
		if err != nil {
			// Check if we've reached the final result set and stop iteration.
			// NOTE: It would be nice if there was a real sentinel error type
			// we could check here instead of comparing error strings, but pgx
			// doesn't expose one. We will just need to verify that the error
			// message doesn't change when we update the pgx dependency.
			if err.Error() == "no more results in batch" {
				break
			}
			return nil, DBExecuteQueryOutput{}, err
		}

		// Process this result set
		result, err := processResultSet(conn, rows)
		if err != nil {
			return nil, DBExecuteQueryOutput{}, err
		}

		// Collect this result set
		resultSets = append(resultSets, result)
	}

	if err := br.Close(); err != nil {
		return nil, DBExecuteQueryOutput{}, err
	}

	// Build output from all result sets
	output := DBExecuteQueryOutput{
		ResultSets:    resultSets,
		ExecutionTime: time.Since(startTime).String(),
	}

	return nil, output, nil
}

// processResultSet reads all data from a pgx.Rows result set
func processResultSet(conn *pgx.Conn, rows pgx.Rows) (ResultSet, error) {
	defer rows.Close()

	// Get column metadata from field descriptions
	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]DBExecuteQueryColumn, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		// Get the type name from the connection's type map
		typeName := "unknown"
		dataType, ok := conn.TypeMap().TypeForOID(fd.DataTypeOID)
		if ok && dataType != nil {
			typeName = dataType.Name
		}
		columns[i] = DBExecuteQueryColumn{
			Name: fd.Name,
			Type: typeName,
		}
	}

	// Collect all rows from this result set
	var resultRows [][]any
	if len(columns) > 0 {
		// If any columns were returned, initialize resultRows to an empty
		// slice to ensure we always return a JSON array in the results, even
		// if empty (we want to be completely clear when a SELECT query returns
		// no rows). On the other hand, if no columns were returned, it's not a
		// result returning query (e.g. it's DDL or an INSERT/UPDATE/DELETE/etc.),
		// so we leave resultRows nil so it gets omitted from the JSON result.
		resultRows = make([][]any, 0)
	}
	for rows.Next() {
		// Scan values into generic interface slice
		values, err := rows.Values()
		if err != nil {
			return ResultSet{}, err
		}
		resultRows = append(resultRows, values)
	}

	// Check for errors during iteration
	if err := rows.Err(); err != nil {
		return ResultSet{}, err
	}

	commandTag := rows.CommandTag()
	return ResultSet{
		CommandTag:   commandTag.String(),
		Columns:      columns,
		Rows:         util.PtrIfNonNil(resultRows),
		RowsAffected: commandTag.RowsAffected(),
	}, nil
}
