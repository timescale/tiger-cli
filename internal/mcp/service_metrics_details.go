package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceMetricsDetailsInput represents input for service_metrics_details
type ServiceMetricsDetailsInput struct {
	ServiceID  string `json:"service_id"`
	MetricName string `json:"metric_name"`
}

func (ServiceMetricsDetailsInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceMetricsDetailsInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["metric_name"].Description = "Name of the metric to describe. Use service_metrics_available to discover valid names."
	schema.Properties["metric_name"].Examples = []any{
		"timescale_cloud_system_cpu_usage_millicores",
		"timescale_cloud_system_memory_usage_bytes",
	}

	return schema
}

// ServiceMetricsDetailsOutput represents output for service_metrics_details
type ServiceMetricsDetailsOutput struct {
	Details api.MetricDetails `json:"details"`
}

func (ServiceMetricsDetailsOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceMetricsDetailsOutput](nil))
}

func newServiceMetricsDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceMetricsDetails,
		Title: "Get Metric Details",
		Description: `Get descriptive metadata for a metric: what it measures, its type, default
aggregation function, and available labels.

Use service_metrics_available to discover metric names, this tool to inspect
one, then service_metrics_series to fetch its data.`,
		InputSchema:  ServiceMetricsDetailsInput{}.Schema(),
		OutputSchema: ServiceMetricsDetailsOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "Get Metric Details",
		},
	}
}

// handleServiceMetricsDetails handles the service_metrics_details MCP tool
func (s *Server) handleServiceMetricsDetails(ctx context.Context, req *mcp.CallToolRequest, input ServiceMetricsDetailsInput) (*mcp.CallToolResult, ServiceMetricsDetailsOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, ServiceMetricsDetailsOutput{}, err
	}

	s.logger.Info("MCP: Getting metric details",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.String("metric", input.MetricName),
	)

	resp, err := client.GetServiceMetricDetailsWithResponse(ctx, projectID, input.ServiceID, input.MetricName)
	if err != nil {
		return nil, ServiceMetricsDetailsOutput{}, fmt.Errorf("failed to get metric details: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, ServiceMetricsDetailsOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, ServiceMetricsDetailsOutput{}, fmt.Errorf("empty response from API")
	}

	return nil, ServiceMetricsDetailsOutput{Details: *resp.JSON200}, nil
}
