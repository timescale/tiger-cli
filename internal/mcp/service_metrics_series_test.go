package mcp

import (
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceMetricsSeries(t *testing.T) {
	fromTime := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	toTime := time.Date(2026, 5, 13, 1, 0, 0, 0, time.UTC)

	args := func(filter map[string]any) map[string]any {
		return map[string]any{
			"service_id":  "e6ue9697jf",
			"metric_name": "some_metric",
			"from":        "2026-05-13T00:00:00Z",
			"to":          "2026-05-13T01:00:00Z",
			"filters":     []map[string]any{filter},
		}
	}

	// expectSeries registers the mock for the given filters, returning an
	// empty result — the test only cares about the request body sent.
	expectSeries := func(filters []api.MetricLabelFilter) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			empty := []api.MetricSeries{}
			body := api.MetricsSeriesRequest{
				Name: "some_metric",
				From: fromTime,
				To:   toTime,
			}
			if len(filters) > 0 {
				body.Filters = &filters
			}
			m.EXPECT().GetServiceMetricsSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf", body).
				Return(&api.GetServiceMetricsSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &empty,
				}, nil)
		}
	}

	wantEmptySeries := map[string]any{"series": []any{}}

	runToolTests(t, []toolTest{
		{
			// Array-item schema defaults aren't SDK-applied, so match_type
			// must stay nil here, not an empty string.
			name:         "filter without match_type omits MatchType rather than sending an empty one",
			tool:         toolServiceMetricsSeries,
			experimental: true,
			args:         args(map[string]any{"key": "ordinal", "value": "0"}),
			setupMock:    expectSeries([]api.MetricLabelFilter{{Key: "ordinal", Value: "0"}}),
			wantOutput:   wantEmptySeries,
		},
		{
			name:         "explicit NOT_EQUAL match_type",
			tool:         toolServiceMetricsSeries,
			experimental: true,
			args:         args(map[string]any{"key": "role", "value": "replica", "match_type": "NOT_EQUAL"}),
			setupMock:    expectSeries([]api.MetricLabelFilter{{Key: "role", Value: "replica", MatchType: new(api.MetricMatchTypeNOTEQUAL)}}),
			wantOutput:   wantEmptySeries,
		},
		{
			// buildMetricFilters silently drops a filter with an empty key or
			// value rather than erroring — pre-existing behavior, unchanged here.
			name:         "filter with an empty value is dropped",
			tool:         toolServiceMetricsSeries,
			experimental: true,
			args:         args(map[string]any{"key": "role", "value": ""}),
			setupMock:    expectSeries(nil),
			wantOutput:   wantEmptySeries,
		},
		{
			name:         "group_by builds a GroupBy request",
			tool:         toolServiceMetricsSeries,
			experimental: true,
			args: map[string]any{
				"service_id":  "e6ue9697jf",
				"metric_name": "some_metric",
				"from":        "2026-05-13T00:00:00Z",
				"to":          "2026-05-13T01:00:00Z",
				"group_by":    []string{"role", "ordinal"},
			},
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				empty := []api.MetricSeries{}
				groupBy := []string{"role", "ordinal"}
				m.EXPECT().GetServiceMetricsSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf", api.MetricsSeriesRequest{
					Name:    "some_metric",
					From:    fromTime,
					To:      toTime,
					GroupBy: &groupBy,
				}).Return(&api.GetServiceMetricsSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &empty,
				}, nil)
			},
			wantOutput: wantEmptySeries,
		},
	})
}
