package mcp

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceLogsInput represents input for service_logs
type ServiceLogsInput struct {
	ServiceID string    `json:"service_id"`
	Node      *int      `json:"node,omitempty"`
	Tail      int       `json:"tail,omitempty"`
	Since     time.Time `json:"since,omitzero"`
	Until     time.Time `json:"until,omitzero"`
}

func (ServiceLogsInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceLogsInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["node"].Description = "Specific service node to fetch logs from (for services with HA replicas). If not provided, logs from the primary node will be fetched."
	schema.Properties["node"].Minimum = new(0.0)
	schema.Properties["node"].Examples = []any{0, 1, 2}

	setLogsInputSchemaProperties(schema)

	return schema
}

// ServiceLogsOutput represents output for service_logs
type ServiceLogsOutput struct {
	Logs []string `json:"logs"`
}

func (ServiceLogsOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceLogsOutput](nil))
	setLogsOutputSchemaProperties(schema)
	return schema
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
			OpenWorldHint: new(false),
			Title:         "Get Service Logs",
		},
	}
}

// handleServiceLogs handles the service_logs MCP tool
func (s *Server) handleServiceLogs(ctx context.Context, req *mcp.CallToolRequest, input ServiceLogsInput) (*mcp.CallToolResult, ServiceLogsOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, ServiceLogsOutput{}, err
	}

	s.logger.Info("MCP: Fetching service logs",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.Any("node", input.Node),
		slog.Int("tail", input.Tail),
		slog.Any("since", input.Since),
		slog.Any("until", input.Until),
	)

	// Fetch logs with pagination support
	entries, err := common.FetchServiceLogs(ctx, common.FetchServiceLogsArgs{
		Client:    client,
		ProjectID: projectID,
		ServiceID: input.ServiceID,
		Tail:      input.Tail,
		Since:     input.Since,
		Until:     input.Until,
		Node:      input.Node,
	})
	if err != nil {
		return nil, ServiceLogsOutput{}, err
	}

	return nil, ServiceLogsOutput{Logs: formatLogLines(entries)}, nil
}
