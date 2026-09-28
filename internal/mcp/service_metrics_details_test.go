package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceMetricsDetailsTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "metric_name": "pg_stat_activity_count"}

	expectDetails := func(metric string, details *api.MetricDetails) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "e6ue9697jf", metric).
				Return(&api.GetServiceMetricDetailsResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      details,
				}, nil)
		}
	}

	// A documented metric: its own labels carry no description yet, while the
	// region/role/ordinal labels every registry-backed metric gets do.
	documented := &api.MetricDetails{
		Name:        "pg_stat_activity_count",
		Type:        new(api.MetricTypeGAUGE),
		DefaultAgg:  new(api.MetricsAggFnMAXTOTAL),
		Description: "Number of connections in pg_stat_activity, grouped by state, connected role, and backend type.",
		Labels: []api.MetricLabelDetails{
			{Name: "datname"},
			{Name: "region", Description: "Region the service runs in."},
		},
	}
	wantDocumented := map[string]any{"details": map[string]any{
		"name":        "pg_stat_activity_count",
		"type":        "GAUGE",
		"default_agg": "MAX_TOTAL",
		"description": "Number of connections in pg_stat_activity, grouped by state, connected role, and backend type.",
		"labels": []any{
			map[string]any{"name": "datname", "description": ""},
			map[string]any{"name": "region", "description": "Region the service runs in."},
		},
	}}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceMetricsDetails,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceMetricsDetails,
			args:    map[string]any{"service_id": "NOPE", "metric_name": "pg_stat_activity_count"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "NOPE" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "missing metric name",
			tool:    toolServiceMetricsDetails,
			args:    map[string]any{"service_id": "e6ue9697jf"},
			wantErr: `validating "arguments": validating root: required: missing properties: ["metric_name"]`,
		},
		{
			name: "network error",
			tool: toolServiceMetricsDetails,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "e6ue9697jf", "pg_stat_activity_count").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get metric details: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceMetricsDetails,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "e6ue9697jf", "pg_stat_activity_count").
					Return(&api.GetServiceMetricDetailsResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "API error without a message body",
			tool: toolServiceMetricsDetails,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "e6ue9697jf", "pg_stat_activity_count").
					Return(&api.GetServiceMetricDetailsResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
		},
		{
			name:    "nil response body",
			tool:    toolServiceMetricsDetails,
			args:    args,
			mock:    expectDetails("pg_stat_activity_count", nil),
			wantErr: "empty response from API",
		},
		{
			name:       "documented metric",
			tool:       toolServiceMetricsDetails,
			args:       args,
			mock:       expectDetails("pg_stat_activity_count", documented),
			wantOutput: wantDocumented,
		},
		{
			// An undocumented metric still returns every key, with the type
			// and default aggregation null and the labels absent.
			name: "undocumented metric",
			tool: toolServiceMetricsDetails,
			args: map[string]any{"service_id": "e6ue9697jf", "metric_name": "some_new_metric"},
			mock: expectDetails("some_new_metric", &api.MetricDetails{Name: "some_new_metric"}),
			wantOutput: map[string]any{"details": map[string]any{
				"name":        "some_new_metric",
				"type":        nil,
				"default_agg": nil,
				"description": "",
				"labels":      nil,
			}},
		},
	})
}
