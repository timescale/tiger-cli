package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceMetricsAvailableInput represents input for service_metrics_available
type ServiceMetricsAvailableInput struct {
	ServiceID string `json:"service_id"`
}

func (ServiceMetricsAvailableInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceMetricsAvailableInput](nil))
	setServiceIDSchemaProperties(schema)
	return schema
}

// ServiceMetricsAvailableOutput represents output for service_metrics_available
type ServiceMetricsAvailableOutput struct {
	Series []string `json:"series"`
}

func (ServiceMetricsAvailableOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceMetricsAvailableOutput](nil))
}

func newServiceMetricsAvailableTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceMetricsAvailable,
		Title: "List Available Metric Series",
		Description: "List the names of all metric series available for a service. " +
			"Call this first to discover what metrics exist before fetching data with service_metrics_series.",
		InputSchema:  ServiceMetricsAvailableInput{}.Schema(),
		OutputSchema: ServiceMetricsAvailableOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: util.Ptr(false),
			Title:         "List Available Metric Series",
		},
	}
}

// handleServiceMetricsAvailable handles the service_metrics_available MCP tool
func (s *Server) handleServiceMetricsAvailable(ctx context.Context, req *mcp.CallToolRequest, input ServiceMetricsAvailableInput) (*mcp.CallToolResult, ServiceMetricsAvailableOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, ServiceMetricsAvailableOutput{}, err
	}

	s.logger.Info("MCP: Listing available metric series",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
	)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.GetServiceMetricsAvailableSeriesWithResponse(ctx, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceMetricsAvailableOutput{}, fmt.Errorf("failed to list metric series: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, ServiceMetricsAvailableOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, ServiceMetricsAvailableOutput{Series: []string{}}, nil
	}

	return nil, ServiceMetricsAvailableOutput{Series: *resp.JSON200}, nil
}
