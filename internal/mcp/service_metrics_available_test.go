package mcp

import (
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceMetricsAvailable(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	expectSeries := func(series *[]string) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.GetServiceMetricsAvailableSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      series,
				}, nil)
		}
	}

	runToolTests(t, []toolTest{
		{
			// The tool is gated at registration, so without the experimental
			// gate the server never advertises it and the SDK refuses the call
			// itself — a transport error rather than a result.
			name:        "not registered without the experimental gate",
			tool:        toolServiceMetricsAvailable,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "service_metrics_available"`,
		},
		{
			name:         "not logged in",
			tool:         toolServiceMetricsAvailable,
			args:         args,
			experimental: true,
			clientErr:    errNotLoggedIn,
			wantErr:      errNotLoggedIn.Error(),
		},
		{
			name:         "service ID failing the schema pattern",
			tool:         toolServiceMetricsAvailable,
			args:         map[string]any{"service_id": "NOPE"},
			experimental: true,
			wantErr:      `validating "arguments": validating root: validating /properties/service_id: pattern: "NOPE" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:         "missing service ID",
			tool:         toolServiceMetricsAvailable,
			args:         map[string]any{},
			experimental: true,
			wantErr:      `validating "arguments": validating root: required: missing properties: ["service_id"]`,
		},
		{
			name:         "API error",
			tool:         toolServiceMetricsAvailable,
			args:         args,
			experimental: true,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceMetricsAvailableSeriesResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name:         "API error without a message body",
			tool:         toolServiceMetricsAvailable,
			args:         args,
			experimental: true,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceMetricsAvailableSeriesResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
		},
		{
			// A 200 with no parsed body is reported as no series, not an error.
			name:         "nil response body",
			tool:         toolServiceMetricsAvailable,
			args:         args,
			experimental: true,
			setupMock:    expectSeries(nil),
			wantOutput:   map[string]any{"series": []any{}},
		},
		{
			name:         "no series available",
			tool:         toolServiceMetricsAvailable,
			args:         args,
			experimental: true,
			setupMock:    expectSeries(&[]string{}),
			wantOutput:   map[string]any{"series": []any{}},
		},
		{
			name:         "series listed",
			tool:         toolServiceMetricsAvailable,
			args:         args,
			experimental: true,
			setupMock: expectSeries(&[]string{
				"timescale_cloud_system_cpu_usage_millicores",
				"timescale_cloud_system_memory_usage_bytes",
			}),
			wantOutput: map[string]any{"series": []any{
				"timescale_cloud_system_cpu_usage_millicores",
				"timescale_cloud_system_memory_usage_bytes",
			}},
		},
		{
			// A JSON `null` array parses to a non-nil pointer holding a nil
			// slice, which the handler passes straight through as a null
			// `series` rather than the empty array it emits for a nil body.
			name:         "null series array",
			tool:         toolServiceMetricsAvailable,
			args:         args,
			experimental: true,
			setupMock:    expectSeries(new([]string)),
			wantOutput:   map[string]any{"series": nil},
		},
	})
}
