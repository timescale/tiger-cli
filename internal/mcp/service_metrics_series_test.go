package mcp

import (
	"errors"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceMetricsSeriesTool(t *testing.T) {
	baseArgs := map[string]any{
		"service_id":  "e6ue9697jf",
		"metric_name": "timescale_cloud_system_cpu_usage_millicores",
		"from":        "2026-05-13T00:00:00Z",
		"to":          "2026-05-13T01:00:00Z",
	}
	args := func(extra map[string]any) map[string]any {
		out := maps.Clone(baseArgs)
		maps.Copy(out, extra)
		return out
	}

	// The tool is experimental-gated (see the first case), so every other
	// case registers it explicitly.
	experimental := withExperimental()

	from := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 13, 1, 0, 0, 0, time.UTC)
	baseBody := api.MetricsSeriesRequest{
		Name: "timescale_cloud_system_cpu_usage_millicores",
		From: from,
		To:   to,
	}
	// body is baseBody with the optional fields the case expects filled in.
	body := func(apply func(*api.MetricsSeriesRequest)) api.MetricsSeriesRequest {
		b := baseBody
		apply(&b)
		return b
	}

	expectSeries := func(b api.MetricsSeriesRequest, series *[]api.MetricSeries) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceMetricsSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf", b).
				Return(&api.GetServiceMetricsSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      series,
				}, nil)
		}
	}
	expectError := func(status int, clientErr *api.ClientError) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceMetricsSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf", baseBody).
				Return(&api.GetServiceMetricsSeriesResponse{
					HTTPResponse: httpResponse(status),
					JSON4XX:      clientErr,
				}, nil)
		}
	}

	// One series per label set, with a gap bucket to cover the nullable value.
	series := []api.MetricSeries{
		{
			Labels: map[string]string{"role": "primary", "ordinal": "0"},
			Data: []api.MetricDataPoint{
				{Time: from, Value: new(247.5)},
				{Time: from.Add(time.Minute)},
			},
		},
		{
			Labels: map[string]string{"role": "replica", "ordinal": "1"},
			Data:   []api.MetricDataPoint{{Time: from, Value: new(102.25)}},
		},
	}
	wantSeries := map[string]any{"series": []any{
		map[string]any{
			"labels": map[string]any{"role": "primary", "ordinal": "0"},
			"data": []any{
				map[string]any{"time": "2026-05-13T00:00:00Z", "value": 247.5},
				map[string]any{"time": "2026-05-13T00:01:00Z", "value": nil},
			},
		},
		map[string]any{
			"labels": map[string]any{"role": "replica", "ordinal": "1"},
			"data":   []any{map[string]any{"time": "2026-05-13T00:00:00Z", "value": 102.25}},
		},
	}}
	noSeries := map[string]any{"series": []any{}}

	runToolTests(t, []toolTest{
		{
			// The tool is gated at registration, so without the experimental
			// gate the server never advertises it and the SDK refuses the call
			// itself — a transport error rather than a result.
			name:        "not registered without the experimental gate",
			tool:        toolServiceMetricsSeries,
			args:        baseArgs,
			wantCallErr: `calling "tools/call": unknown tool "service_metrics_series"`,
		},
		{
			name:    "not logged in",
			tool:    toolServiceMetricsSeries,
			args:    baseArgs,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "no arguments",
			tool:    toolServiceMetricsSeries,
			args:    map[string]any{},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: required: missing properties: ["service_id" "metric_name" "from" "to"]`,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceMetricsSeries,
			args:    args(map[string]any{"service_id": "NOPE"}),
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "NOPE" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "role outside the enum",
			tool:    toolServiceMetricsSeries,
			args:    args(map[string]any{"role": "primary"}),
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/role: enum: primary does not equal any of: [PRIMARY REPLICA]`,
		},
		{
			name:    "aggregation function outside the enum",
			tool:    toolServiceMetricsSeries,
			args:    args(map[string]any{"fn": "MEDIAN"}),
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/fn: enum: MEDIAN does not equal any of: [RATE INCREASE SUM AVG MIN MAX MIN_TOTAL MAX_TOTAL COUNT P50 P90 P99 LAST]`,
		},
		{
			name:    "bucket below the minimum",
			tool:    toolServiceMetricsSeries,
			args:    args(map[string]any{"bucket_seconds": 30}),
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/bucket_seconds: minimum: 30/1 is less than 60.000000`,
		},
		{
			// The time window is plain strings in the schema, so the handler is
			// what rejects a non-RFC3339 value.
			name:    "from is not RFC3339",
			tool:    toolServiceMetricsSeries,
			args:    args(map[string]any{"from": "yesterday"}),
			opts:    []runOption{experimental},
			wantErr: `from must be RFC3339 (e.g., 2026-05-13T00:00:00Z): parsing time "yesterday" as "2006-01-02T15:04:05Z07:00": cannot parse "yesterday" as "2006"`,
		},
		{
			name:    "to is not RFC3339",
			tool:    toolServiceMetricsSeries,
			args:    args(map[string]any{"to": "2026-05-13"}),
			opts:    []runOption{experimental},
			wantErr: `to must be RFC3339 (e.g., 2026-05-13T01:00:00Z): parsing time "2026-05-13" as "2006-01-02T15:04:05Z07:00": cannot parse "" as "T"`,
		},
		{
			name: "network error",
			tool: toolServiceMetricsSeries,
			args: baseArgs,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricsSeriesWithResponse(validCtx, testProjectID, "e6ue9697jf", baseBody).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch metric series: connection refused",
		},
		{
			name:    "API error",
			tool:    toolServiceMetricsSeries,
			args:    baseArgs,
			opts:    []runOption{experimental},
			mock:    expectError(http.StatusBadRequest, &api.ClientError{Message: new("unknown metric name")}),
			wantErr: "unknown metric name",
		},
		{
			name:    "API error without a message body",
			tool:    toolServiceMetricsSeries,
			args:    baseArgs,
			opts:    []runOption{experimental},
			mock:    expectError(http.StatusInternalServerError, nil),
			wantErr: "unknown error",
		},
		{
			// A 200 with no parsed body is reported as no series, not an error.
			name:       "nil response body",
			tool:       toolServiceMetricsSeries,
			args:       baseArgs,
			opts:       []runOption{experimental},
			mock:       expectSeries(baseBody, nil),
			wantOutput: noSeries,
		},
		{
			// A JSON `null` array is normalized to an empty one, so the output
			// stays a valid array.
			name:       "null series array",
			tool:       toolServiceMetricsSeries,
			args:       baseArgs,
			opts:       []runOption{experimental},
			mock:       expectSeries(baseBody, new([]api.MetricSeries)),
			wantOutput: noSeries,
		},
		{
			name:       "no data in the window",
			tool:       toolServiceMetricsSeries,
			args:       baseArgs,
			opts:       []runOption{experimental},
			mock:       expectSeries(baseBody, &[]api.MetricSeries{}),
			wantOutput: noSeries,
		},
		{
			name:       "series returned",
			tool:       toolServiceMetricsSeries,
			args:       baseArgs,
			opts:       []runOption{experimental},
			mock:       expectSeries(baseBody, &series),
			wantOutput: wantSeries,
		},
		{
			// The convenience role is sent as a lowercased label filter.
			name: "role filter",
			tool: toolServiceMetricsSeries,
			args: args(map[string]any{"role": "REPLICA"}),
			opts: []runOption{experimental},
			mock: expectSeries(body(func(b *api.MetricsSeriesRequest) {
				b.Filters = &[]api.MetricLabelFilter{{Key: "role", Value: "replica"}}
			}), &[]api.MetricSeries{}),
			wantOutput: noSeries,
		},
		{
			// Explicit filters follow the role filter, and blank ones are dropped.
			name: "role and explicit filters merged",
			tool: toolServiceMetricsSeries,
			args: args(map[string]any{
				"role": "PRIMARY",
				"filters": []any{
					map[string]any{"key": "ordinal", "value": "0"},
					map[string]any{"key": "", "value": "1000"},
					map[string]any{"key": "job_id", "value": ""},
				},
			}),
			opts: []runOption{experimental},
			mock: expectSeries(body(func(b *api.MetricsSeriesRequest) {
				b.Filters = &[]api.MetricLabelFilter{
					{Key: "role", Value: "primary"},
					{Key: "ordinal", Value: "0"},
				}
			}), &[]api.MetricSeries{}),
			wantOutput: noSeries,
		},
		{
			name: "bucket size and aggregation function",
			tool: toolServiceMetricsSeries,
			args: args(map[string]any{"bucket_seconds": 3600, "fn": "AVG"}),
			opts: []runOption{experimental},
			mock: expectSeries(body(func(b *api.MetricsSeriesRequest) {
				b.BucketSeconds = new(3600)
				b.Fn = new(api.MetricsAggFnAVG)
			}), &[]api.MetricSeries{}),
			wantOutput: noSeries,
		},
		{
			// An empty filter list leaves the request body's filters unset.
			name:       "empty filter list",
			tool:       toolServiceMetricsSeries,
			args:       args(map[string]any{"filters": []any{}}),
			opts:       []runOption{experimental},
			mock:       expectSeries(baseBody, &[]api.MetricSeries{}),
			wantOutput: noSeries,
		},
	})
}
