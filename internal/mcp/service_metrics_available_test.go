package mcp

import (
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceMetricsAvailable(t *testing.T) {
	expectSeries := func(names []string) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.GetServiceMetricsAvailableSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &names,
				}, nil)
		}
	}

	args := map[string]any{"service_id": "e6ue9697jf"}

	runToolTests(t, []toolTest{
		{
			name:         "lists available series",
			tool:         toolServiceMetricsAvailable,
			experimental: true,
			args:         args,
			setupMock:    expectSeries([]string{"metric_a", "metric_b"}),
			wantOutput:   map[string]any{"series": []any{"metric_a", "metric_b"}},
		},
		{
			name:              "sends a progress notification naming the service",
			tool:              toolServiceMetricsAvailable,
			experimental:      true,
			withProgressToken: true,
			args:              args,
			setupMock:         expectSeries([]string{"metric_a"}),
			wantOutput:        map[string]any{"series": []any{"metric_a"}},
			wantProgress: []string{
				"Listing available metric series for service e6ue9697jf...",
			},
		},
		{
			name:         "no progress notification without a client-supplied token",
			tool:         toolServiceMetricsAvailable,
			experimental: true,
			args:         args,
			setupMock:    expectSeries([]string{"metric_a"}),
			wantOutput:   map[string]any{"series": []any{"metric_a"}},
			wantProgress: nil,
		},
	})
}
