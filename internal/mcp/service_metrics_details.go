package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

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
		"pg_stat_activity_count",
	}

	return schema
}

// ServiceMetricsDetailsOutput represents output for service_metrics_details
type ServiceMetricsDetailsOutput struct {
	Details api.MetricDetails `json:"details"`
}

func (ServiceMetricsDetailsOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceMetricsDetailsOutput](nil))

	// These fields are always present in the response (hence "required" above),
	// but their content is only present once the metric is documented — every
	// undocumented metric still returns the key, just with a null/empty value.
	details := schema.Properties["details"]
	details.Properties["name"].Description = "Metric series name."
	details.Properties["type"].Description = "The shape of this metric's data, or null if undocumented."
	details.Properties["default_agg"].Description = "The aggregation function used by default when fn is omitted from a series query, or null if undocumented."
	details.Properties["description"].Description = "What this metric measures, or empty if undocumented."
	details.Properties["labels"].Description = "All labels this metric can be filtered or grouped by: its own labels (e.g. datname on pg_stat_database_*) plus the region/role/ordinal labels most metrics also carry. A few datasources only attach a subset of those — e.g. pgbouncer-sourced metrics only get region, not role/ordinal."
	details.Properties["labels"].Items.Properties["name"].Description = "The label's key."
	details.Properties["labels"].Items.Properties["description"].Description = "What this label identifies, or empty if undocumented."

	return schema
}

func newServiceMetricsDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceMetricsDetails,
		Title: "Get Metric Details",
		Description: fmt.Sprintf(`Get descriptive metadata for a metric: what it measures, its type, default
aggregation function, and available labels.

Use service_metrics_available to discover metric names, this tool to inspect
one, then service_metrics_series to fetch its data.

These metrics have no richer metadata (returns just the name, with type,
default aggregation, description, and labels all empty): %s.`, strings.Join(common.LegacyMetrics, ", ")),
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

	if token := req.Params.GetProgressToken(); token != nil {
		req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: token,
			Message:       fmt.Sprintf("Fetching details for metric %q on service %s...", input.MetricName, input.ServiceID),
		})
	}

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
