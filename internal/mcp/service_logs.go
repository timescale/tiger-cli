package mcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/logging"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceLogsInput represents input for service_logs
type ServiceLogsInput struct {
	ServiceID string     `json:"service_id"`
	Node      *int       `json:"node,omitempty"`
	Tail      int        `json:"tail,omitempty"`
	Since     *time.Time `json:"since,omitempty"`
	Until     *time.Time `json:"until,omitempty"`
}

func (ServiceLogsInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceLogsInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["node"].Description = "Specific service node to fetch logs from (for services with HA replicas). If not provided, logs from the primary node will be fetched."
	schema.Properties["node"].Minimum = util.Ptr(0.0)
	schema.Properties["node"].Examples = []any{0, 1, 2}

	schema.Properties["tail"].Description = "Number of log lines to return. Defaults to 100."
	schema.Properties["tail"].Default = util.Must(json.Marshal(100))
	schema.Properties["tail"].Minimum = util.Ptr(1.0)
	schema.Properties["tail"].Examples = []any{50, 100, 1000}

	schema.Properties["since"].Description = "Fetch logs after this timestamp (RFC3339 format, e.g., '2024-01-15T09:00:00Z'). If not provided, only the tail parameter limits how far back logs are fetched."
	schema.Properties["since"].Examples = []any{"2024-01-15T09:00:00Z", "2025-01-16T08:00:00Z"}

	schema.Properties["until"].Description = "Fetch logs before this timestamp (RFC3339 format, e.g., '2024-01-15T10:00:00Z'). If not provided, fetches logs up to the current time."
	schema.Properties["until"].Examples = []any{"2024-01-15T10:00:00Z", "2025-01-16T08:30:00Z"}

	return schema
}

// ServiceLogsOutput represents output for service_logs
type ServiceLogsOutput struct {
	Logs []string `json:"logs" jsonschema:"Log lines ordered from oldest to newest. Each line is prefixed with an RFC3339 timestamp followed by the log message."`
}

func (ServiceLogsOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceLogsOutput](nil))
}

func newServiceLogsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceLogs,
		Title: "Get Service Logs",
		Description: `View logs for a database service.

Fetches and displays logs from the specified service. By default, shows the last 100 log entries.

Supports filtering by time (via since/until parameters) and node (for services with HA replicas).`,
		InputSchema:  ServiceLogsInput{}.Schema(),
		OutputSchema: ServiceLogsOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: util.Ptr(true),
			Title:         "Get Service Logs",
		},
	}
}

// handleServiceLogs handles the service_logs MCP tool
func (s *Server) handleServiceLogs(ctx context.Context, req *mcp.CallToolRequest, input ServiceLogsInput) (*mcp.CallToolResult, ServiceLogsOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceLogsOutput{}, err
	}

	logging.Debug("MCP: Fetching service logs",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID),
		zap.Intp("node", input.Node),
		zap.Int("tail", input.Tail),
		zap.Timep("since", input.Since),
		zap.Timep("until", input.Until),
	)

	// Fetch logs with pagination support
	logsCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	entries, err := common.FetchServiceLogs(logsCtx, cfg, input.ServiceID, input.Tail, input.Since, input.Until, input.Node)
	if err != nil {
		return nil, ServiceLogsOutput{}, err
	}

	logs := make([]string, len(entries))
	for i, e := range entries {
		if !e.Timestamp.IsZero() {
			logs[i] = e.Timestamp.UTC().Format("2006-01-02 15:04:05 UTC") + " " + e.Message
		} else {
			logs[i] = e.Message
		}
	}

	return nil, ServiceLogsOutput{Logs: logs}, nil
}
