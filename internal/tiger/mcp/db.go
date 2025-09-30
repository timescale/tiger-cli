package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// DBExecuteQueryInput represents input for tiger_db_execute_query
type DBExecuteQueryInput struct {
	ServiceID string `json:"service_id,omitempty"`
	Query     string `json:"query"`
	Timeout   *int   `json:"timeout,omitempty"`
}

func (DBExecuteQueryInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[DBExecuteQueryInput](nil))

	schema.Properties["service_id"].Description = "Service ID to execute query on (uses default if not provided)"

	schema.Properties["query"].Description = "SQL query to execute"

	schema.Properties["timeout"].Description = "Query timeout in seconds (default: 30)"
	schema.Properties["timeout"].Minimum = util.Ptr(0.0)
	schema.Properties["timeout"].Default = util.Must(json.Marshal(30))
	schema.Properties["timeout"].Examples = []any{10, 30, 60}

	return schema
}

// DBExecuteQueryOutput represents output for tiger_db_execute_query
type DBExecuteQueryOutput struct {
	Columns       []string `json:"columns"`
	Rows          [][]any  `json:"rows"`
	RowCount      int      `json:"row_count"`
	ExecutionTime string   `json:"execution_time"`
}

// registerDatabaseTools registers database operation tools with comprehensive schemas and descriptions
func (s *Server) registerDatabaseTools() {
	// tiger_db_execute_query
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "tiger_db_execute_query",
		Title: "Execute SQL Query",
		Description: `Execute a SQL query against a service database.

This tool connects to a database service and executes the provided SQL query, returning the results with column names, row data, and execution metadata. Perfect for data exploration, schema inspection, and database operations.

IMPORTANT: Use with caution - this tool can execute any SQL statement including INSERT, UPDATE, DELETE, and DDL commands. Always review queries before execution.

Perfect for:
- Querying data from tables and views
- Inspecting database schema
- Testing database connectivity with real queries
- Performing data analysis and exploration`,
		InputSchema: DBExecuteQueryInput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: util.Ptr(true), // Can execute destructive SQL
			Title:           "Execute SQL Query",
		},
	}, s.handleDBExecuteQuery)
}

// handleDBExecuteQuery handles the tiger_db_execute_query MCP tool
func (s *Server) handleDBExecuteQuery(ctx context.Context, req *mcp.CallToolRequest, input DBExecuteQueryInput) (*mcp.CallToolResult, DBExecuteQueryOutput, error) {
	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return nil, DBExecuteQueryOutput{}, err
	}

	// Load fresh config and validate project ID is set
	cfg, err := s.loadConfigWithProjectID()
	if err != nil {
		return nil, DBExecuteQueryOutput{}, err
	}

	// Get service ID (use default from config if not provided)
	serviceID := input.ServiceID
	if serviceID == "" {
		if cfg.ServiceID == "" {
			return nil, DBExecuteQueryOutput{}, fmt.Errorf("service ID is required. Please provide service_id or run 'tiger config set service_id <id>'")
		}
		serviceID = cfg.ServiceID
	}

	// Set default timeout if not provided
	timeout := 30 * time.Second
	if input.Timeout != nil {
		timeout = time.Duration(*input.Timeout) * time.Second
	}

	logging.Debug("MCP: Executing database query",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", serviceID),
		zap.Duration("timeout", timeout),
	)

	// Get service details to construct connection string
	serviceResp, err := apiClient.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, cfg.ProjectID, serviceID)
	if err != nil {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("failed to get service details: %w", err)
	}

	switch serviceResp.StatusCode() {
	case 200:
		if serviceResp.JSON200 == nil {
			return nil, DBExecuteQueryOutput{}, fmt.Errorf("empty response from API")
		}
	case 401:
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("authentication failed: invalid API key")
	case 403:
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("permission denied: insufficient access to service")
	case 404:
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("service '%s' not found in project '%s'", serviceID, cfg.ProjectID)
	default:
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("API request failed with status %d", serviceResp.StatusCode())
	}

	service := *serviceResp.JSON200

	// Build connection string with password (use direct connection, default role tsdbadmin)
	connString, err := s.buildConnectionString(service, false, "tsdbadmin")
	if err != nil {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("failed to build connection string: %w", err)
	}

	// Create query context with timeout
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Connect to database
	conn, err := pgx.Connect(queryCtx, connString)
	if err != nil {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer conn.Close(context.Background())

	// Execute query and measure time
	startTime := time.Now()
	rows, err := conn.Query(queryCtx, input.Query)
	if err != nil {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names from field descriptions
	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]string, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		columns[i] = string(fd.Name)
	}

	// Collect all rows
	var resultRows [][]any
	for rows.Next() {
		// Scan values into generic interface slice
		values, err := rows.Values()
		if err != nil {
			return nil, DBExecuteQueryOutput{}, fmt.Errorf("failed to scan row: %w", err)
		}
		resultRows = append(resultRows, values)
	}

	// Check for errors during iteration
	if rows.Err() != nil {
		return nil, DBExecuteQueryOutput{}, fmt.Errorf("error during row iteration: %w", rows.Err())
	}

	output := DBExecuteQueryOutput{
		Columns:       columns,
		Rows:          resultRows,
		RowCount:      len(resultRows),
		ExecutionTime: time.Since(startTime).String(),
	}

	return nil, output, nil
}

// buildConnectionString creates a PostgreSQL connection string from service details with password included
func (s *Server) buildConnectionString(service api.Service, pooled bool, role string) (string, error) {
	if service.Endpoint == nil {
		return "", fmt.Errorf("service endpoint not available")
	}

	var endpoint *api.Endpoint
	var host string
	var port int

	// Use pooler endpoint if requested and available, otherwise use direct endpoint
	if pooled && service.ConnectionPooler != nil && service.ConnectionPooler.Endpoint != nil {
		endpoint = service.ConnectionPooler.Endpoint
	} else {
		// If pooled was requested but no pooler is available, use direct endpoint
		endpoint = service.Endpoint
	}

	if endpoint.Host == nil {
		return "", fmt.Errorf("endpoint host not available")
	}
	host = *endpoint.Host

	if endpoint.Port != nil {
		port = *endpoint.Port
	} else {
		port = 5432 // Default PostgreSQL port
	}

	// Get password from storage
	storage := util.GetPasswordStorage()
	password, err := storage.Get(service)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve password: %w", err)
	}

	if password == "" {
		return "", fmt.Errorf("no password available for service")
	}

	// Database is always "tsdb" for TimescaleDB/PostgreSQL services
	database := "tsdb"

	// Build connection string with password included
	connectionString := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=require", role, password, host, port, database)

	return connectionString, nil
}
