package mcp

import (
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceMetricsDetails(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "metric_name": "pg_stat_activity_count"}

	expectDetails := func(details api.MetricDetails) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "e6ue9697jf", "pg_stat_activity_count").
				Return(&api.GetServiceMetricDetailsResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &details,
				}, nil)
		}
	}

	details := api.MetricDetails{
		Name:        "pg_stat_activity_count",
		Type:        new(api.MetricTypeGAUGE),
		DefaultAgg:  new(api.MetricsAggFnAVG),
		Description: "Number of active backend connections.",
		Labels:      []api.MetricLabelDetails{{Name: "role"}},
	}

	runToolTests(t, []toolTest{
		{
			name:         "fetches metric details",
			tool:         toolServiceMetricsDetails,
			experimental: true,
			args:         args,
			setupMock:    expectDetails(details),
			wantOutput: map[string]any{
				"details": map[string]any{
					"name":        "pg_stat_activity_count",
					"type":        "GAUGE",
					"default_agg": "AVG",
					"description": "Number of active backend connections.",
					"labels":      []any{map[string]any{"name": "role", "description": ""}},
				},
			},
		},
		{
			name:              "sends a progress notification naming the metric and service",
			tool:              toolServiceMetricsDetails,
			experimental:      true,
			withProgressToken: true,
			args:              args,
			setupMock:         expectDetails(details),
			wantOutput: map[string]any{
				"details": map[string]any{
					"name":        "pg_stat_activity_count",
					"type":        "GAUGE",
					"default_agg": "AVG",
					"description": "Number of active backend connections.",
					"labels":      []any{map[string]any{"name": "role", "description": ""}},
				},
			},
			wantProgress: []string{
				`Fetching details for metric "pg_stat_activity_count" on service e6ue9697jf...`,
			},
		},
		{
			name:         "no progress notification without a client-supplied token",
			tool:         toolServiceMetricsDetails,
			experimental: true,
			args:         args,
			setupMock:    expectDetails(details),
			wantOutput: map[string]any{
				"details": map[string]any{
					"name":        "pg_stat_activity_count",
					"type":        "GAUGE",
					"default_agg": "AVG",
					"description": "Number of active backend connections.",
					"labels":      []any{map[string]any{"name": "role", "description": ""}},
				},
			},
			wantProgress: nil,
		},
	})
}
