package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// MetricLabelFilterInput mirrors api.MetricLabelFilter for the tool schema.
type MetricLabelFilterInput struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	MatchType string `json:"match_type,omitempty"`
}

// ServiceMetricsSeriesInput represents input for service_metrics_series
type ServiceMetricsSeriesInput struct {
	ServiceID     string                   `json:"service_id"`
	MetricName    string                   `json:"metric_name"`
	From          string                   `json:"from"`
	To            string                   `json:"to"`
	Role          string                   `json:"role,omitempty"`
	Filters       []MetricLabelFilterInput `json:"filters,omitempty"`
	BucketSeconds int                      `json:"bucket_seconds,omitempty"`
	Fn            string                   `json:"fn,omitempty"`
	GroupBy       []string                 `json:"group_by,omitempty"`
}

func (ServiceMetricsSeriesInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceMetricsSeriesInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["metric_name"].Description = "Name of the metric series to fetch. Use service_metrics_available to discover valid names."
	schema.Properties["metric_name"].Examples = []any{
		"timescale_cloud_system_cpu_usage_millicores",
		"timescale_cloud_system_memory_usage_bytes",
		"timescale_cloud_system_disk_usage_bytes",
	}

	schema.Properties["from"].Description = "Start of the time window (RFC3339 format)."
	schema.Properties["from"].Examples = []any{"2026-05-13T00:00:00Z", "2026-05-13T09:00:00Z"}

	schema.Properties["to"].Description = "End of the time window (RFC3339 format)."
	schema.Properties["to"].Examples = []any{"2026-05-13T01:00:00Z", "2026-05-13T10:00:00Z"}

	schema.Properties["role"].Description = "Convenience filter for the 'role' label. Omit to include all roles. Equivalent to passing {key:\"role\", value:\"primary\"|\"replica\"} via filters."
	schema.Properties["role"].Enum = []any{"PRIMARY", "REPLICA"}

	schema.Properties["filters"].Description = "Arbitrary label filters applied to the series query. Recognized label names depend on the metric."
	schema.Properties["filters"].Examples = []any{
		[]MetricLabelFilterInput{{Key: "ordinal", Value: "0"}},
		[]MetricLabelFilterInput{{Key: "job_id", Value: "1000"}},
		[]MetricLabelFilterInput{{Key: "role", Value: "replica", MatchType: "NOT_EQUAL"}},
	}
	schema.Properties["filters"].Items.Properties["match_type"].Description = "How to compare the label's value. EQUAL: the series must have the label equal to value. NOT_EQUAL: the series must not have the label equal to value; NOT_EQUAL also matches a series missing the label entirely. To exclude multiple values of the same label, pass multiple filters with that key. Defaults to EQUAL."
	schema.Properties["filters"].Items.Properties["match_type"].Enum = []any{"EQUAL", "NOT_EQUAL"}
	schema.Properties["filters"].Items.Properties["match_type"].Default = util.Must(json.Marshal("EQUAL"))

	schema.Properties["bucket_seconds"].Description = "Aggregation bucket size in seconds. Optional — when omitted, the server picks a default matched to the window (roughly 1m for windows up to 1h, 1h for up to 30d, 1d beyond that). Minimum 60s."
	schema.Properties["bucket_seconds"].Minimum = new(60.0)
	schema.Properties["bucket_seconds"].Examples = []any{60, 300, 3600}

	schema.Properties["fn"].Description = fmt.Sprintf("Aggregation function applied per bucket. Not accepted on these metrics (returns INVALID_REQUEST): %s. When omitted, the server picks a sensible default for the metric (typically LAST). MIN_TOTAL and MAX_TOTAL add every series' samples together first and take the extremum of that total, for a metric with per-series labels where the combined total is the meaningful quantity (e.g. peak of total connections across all databases); MIN and MAX give the single most extreme individual series instead.", strings.Join(common.LegacyMetrics, ", "))
	schema.Properties["fn"].Enum = []any{"RATE", "INCREASE", "SUM", "AVG", "MIN", "MAX", "MIN_TOTAL", "MAX_TOTAL", "COUNT", "P50", "P90", "P99", "LAST"}

	schema.Properties["group_by"].Description = "Label keys to break the result into one series per distinct value combination. Not accepted on the same metrics that reject fn (returns INVALID_REQUEST). Omit, or pass an empty list, to collapse all matching label sets into a single series."
	schema.Properties["group_by"].Examples = []any{[]string{"role"}, []string{"role", "ordinal"}}

	return schema
}

// ServiceMetricsSeriesOutput is the response from service_metrics_series. The
// endpoint returns one MetricSeries per distinct label set (e.g. one per
// replica).
type ServiceMetricsSeriesOutput struct {
	Series []api.MetricSeries `json:"series"`
}

func (ServiceMetricsSeriesOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceMetricsSeriesOutput](nil))
}

func newServiceMetricsSeriesTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceMetricsSeries,
		Title: "Get Metric Series Data",
		Description: `Fetch time-series data for a named metric over a specified time window.

Use service_metrics_available first to discover valid metric names.

The response groups data points by their label set — a single request may
return multiple labeled series (e.g. one per replica, one per worker ordinal).
Each series contains its full list of raw data points. Use group_by to
control which label keys define that grouping.

Available metrics include: CPU usage/allocation, memory usage/total, disk usage, and disk I/O (read/write bytes and ops).`,
		InputSchema:  ServiceMetricsSeriesInput{}.Schema(),
		OutputSchema: ServiceMetricsSeriesOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "Get Metric Series Data",
		},
	}
}

// handleServiceMetricsSeries handles the service_metrics_series MCP tool
func (s *Server) handleServiceMetricsSeries(ctx context.Context, req *mcp.CallToolRequest, input ServiceMetricsSeriesInput) (*mcp.CallToolResult, any, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, nil, err
	}

	s.logger.Info("MCP: Fetching metric series",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.String("metric", input.MetricName),
		slog.String("from", input.From),
		slog.String("to", input.To),
	)

	fromTime, err := time.Parse(time.RFC3339, input.From)
	if err != nil {
		return nil, nil, fmt.Errorf("from must be RFC3339 (e.g., 2026-05-13T00:00:00Z): %w", err)
	}
	toTime, err := time.Parse(time.RFC3339, input.To)
	if err != nil {
		return nil, nil, fmt.Errorf("to must be RFC3339 (e.g., 2026-05-13T01:00:00Z): %w", err)
	}

	filters := buildMetricFilters(input.Role, input.Filters)

	body := api.MetricsSeriesRequest{
		Name: input.MetricName,
		From: fromTime,
		To:   toTime,
	}
	if input.BucketSeconds > 0 {
		bs := input.BucketSeconds
		body.BucketSeconds = &bs
	}
	if input.Fn != "" {
		fn := api.MetricsAggFn(strings.ToUpper(input.Fn))
		body.Fn = &fn
	}
	if len(filters) > 0 {
		body.Filters = &filters
	}
	if len(input.GroupBy) > 0 {
		body.GroupBy = &input.GroupBy
	}

	resp, err := client.GetServiceMetricsSeriesWithResponse(ctx, projectID, input.ServiceID, body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch metric series: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	// Default to a non-nil slice so an empty result marshals to `[]` rather
	// than `null`, which would fail the required-array output-schema validation.
	series := []api.MetricSeries{}
	if resp.JSON200 != nil && *resp.JSON200 != nil {
		series = *resp.JSON200
	}

	return nil, ServiceMetricsSeriesOutput{Series: series}, nil
}

// buildMetricFilters merges the convenience Role input with the arbitrary
// Filters slice into the preview label filter list. Role values are
// lowercased to match the gateway's response normalization.
func buildMetricFilters(role string, filters []MetricLabelFilterInput) []api.MetricLabelFilter {
	var out []api.MetricLabelFilter
	if role != "" {
		out = append(out, api.MetricLabelFilter{Key: "role", Value: strings.ToLower(role)})
	}
	for _, f := range filters {
		// Schema defaults/required apply to object properties, not array
		// items, so these can still arrive empty.
		if f.Key == "" || f.Value == "" {
			continue
		}
		filter := api.MetricLabelFilter{Key: f.Key, Value: f.Value}
		if f.MatchType != "" {
			matchType := api.MetricMatchType(f.MatchType)
			filter.MatchType = &matchType
		}
		out = append(out, filter)
	}
	return out
}
