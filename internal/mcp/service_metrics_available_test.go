package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceMetricsAvailable(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	// The tool is experimental-gated (see the first case), so every other
	// case registers it explicitly.
	experimental := withExperimental()

	expectSeries := func(series *[]string) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.GetServiceMetricsAvailableSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      series,
				}, nil)
		}
	}
	noSeries := map[string]any{"series": []any{}}

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
			name:    "not logged in",
			tool:    toolServiceMetricsAvailable,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceMetricsAvailable,
			args:    map[string]any{"service_id": "NOPE"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "NOPE" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "missing service ID",
			tool:    toolServiceMetricsAvailable,
			args:    map[string]any{},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: required: missing properties: ["service_id"]`,
		},
		{
			name: "network error",
			tool: toolServiceMetricsAvailable,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list metric series: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceMetricsAvailable,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceMetricsAvailableSeriesResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "API error without a message body",
			tool: toolServiceMetricsAvailable,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceMetricsAvailableSeriesResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
		},
		{
			// A 200 with no parsed body is reported as no series, not an error.
			name:       "nil response body",
			tool:       toolServiceMetricsAvailable,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectSeries(nil),
			wantOutput: noSeries,
		},
		{
			// A JSON `null` array is normalized to an empty one, so the output
			// stays a valid array.
			name:       "null series array",
			tool:       toolServiceMetricsAvailable,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectSeries(new([]string)),
			wantOutput: noSeries,
		},
		{
			name:       "no series available",
			tool:       toolServiceMetricsAvailable,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectSeries(&[]string{}),
			wantOutput: noSeries,
		},
		{
			name: "series listed",
			tool: toolServiceMetricsAvailable,
			args: args,
			opts: []runOption{experimental},
			mock: expectSeries(&[]string{
				"timescale_cloud_system_cpu_usage_millicores",
				"timescale_cloud_system_memory_usage_bytes",
			}),
			wantOutput: map[string]any{"series": []any{
				"timescale_cloud_system_cpu_usage_millicores",
				"timescale_cloud_system_memory_usage_bytes",
			}},
		},
	})
}
