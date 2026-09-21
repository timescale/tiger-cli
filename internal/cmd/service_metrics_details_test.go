package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceMetricsDetailsCmd(t *testing.T) {
	// The command is experimental-gated (see the gate test in service_test.go),
	// so every case registers it explicitly.
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	avg := api.MetricsAggFnAVG
	gauge := api.MetricTypeGAUGE

	fullDetails := api.MetricDetails{
		Name:        "pg_locks_count",
		Type:        &gauge,
		DefaultAgg:  &avg,
		Description: "Number of locks currently held, grouped by relation and lock mode.",
		Labels: []api.MetricLabelDetails{
			{Name: "datname", Description: "The database this row's values belong to."},
			{Name: "mode", Description: ""},
		},
	}

	// A metric with no documented metadata yet: type/default_agg are nil,
	// description and labels are empty.
	undocumentedDetails := api.MetricDetails{
		Name: "some_new_metric",
	}

	const fullDetailsTable = `┌─────────────────────┬────────────────────────────────────────────────────────────────────┐
│      PROPERTY       │                               VALUE                                │
├─────────────────────┼────────────────────────────────────────────────────────────────────┤
│ Name                │ pg_locks_count                                                     │
│ Type                │ GAUGE                                                              │
│ Default Aggregation │ AVG                                                                │
│ Description         │ Number of locks currently held, grouped by relation and lock mode. │
└─────────────────────┴────────────────────────────────────────────────────────────────────┘

┌─────────┬───────────────────────────────────────────┐
│  LABEL  │                DESCRIPTION                │
├─────────┼───────────────────────────────────────────┤
│ datname │ The database this row's values belong to. │
│ mode    │ undocumented                              │
└─────────┴───────────────────────────────────────────┘
`

	const undocumentedTable = `┌─────────────────────┬─────────────────┐
│      PROPERTY       │      VALUE      │
├─────────────────────┼─────────────────┤
│ Name                │ some_new_metric │
│ Type                │ undocumented    │
│ Default Aggregation │ undocumented    │
│ Description         │ undocumented    │
└─────────────────────┴─────────────────┘
`

	setupDetails := func(metric string, details api.MetricDetails) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "svc-12345", metric).
				Return(&api.GetServiceMetricDetailsResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &details,
				}, nil)
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_locks_count"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing metric flag",
			args:    []string{"service", "metrics", "details", "svc-12345"},
			opts:    []runOption{experimental},
			wantErr: `required flag(s) "metric" not set`,
		},
		{
			name:    "missing service id",
			args:    []string{"service", "metrics", "details", "--metric", "pg_locks_count"},
			opts:    []runOption{experimental},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_locks_count"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "svc-12345", "pg_locks_count").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get metric details: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_locks_count"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "svc-12345", "pg_locks_count").
					Return(&api.GetServiceMetricDetailsResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_locks_count"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "svc-12345", "pg_locks_count").
					Return(&api.GetServiceMetricDetailsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "table output with labels",
			args:       []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_locks_count"},
			opts:       []runOption{experimental},
			setup:      setupDetails("pg_locks_count", fullDetails),
			wantStdout: fullDetailsTable,
		},
		{
			name:       "default service id from config",
			args:       []string{"service", "metrics", "details", "--metric", "pg_locks_count"},
			opts:       []runOption{experimental, withConfig(map[string]any{"service_id": "svc-12345"})},
			setup:      setupDetails("pg_locks_count", fullDetails),
			wantStdout: fullDetailsTable,
		},
		{
			// Undocumented fields (type, default_agg, description) render as
			// "undocumented", and the second table is skipped when there are no
			// labels.
			name:       "table output without labels",
			args:       []string{"service", "metrics", "details", "svc-12345", "--metric", "some_new_metric"},
			opts:       []runOption{experimental},
			setup:      setupDetails("some_new_metric", undocumentedDetails),
			wantStdout: undocumentedTable,
		},
		{
			name:  "json output",
			args:  []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_locks_count", "-o", "json"},
			opts:  []runOption{experimental},
			setup: setupDetails("pg_locks_count", fullDetails),
			wantStdout: `{
  "default_agg": "AVG",
  "description": "Number of locks currently held, grouped by relation and lock mode.",
  "labels": [
    {
      "description": "The database this row's values belong to.",
      "name": "datname"
    },
    {
      "description": "",
      "name": "mode"
    }
  ],
  "name": "pg_locks_count",
  "type": "GAUGE"
}
`,
		},
		{
			name:  "yaml output",
			args:  []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_locks_count", "-o", "yaml"},
			opts:  []runOption{experimental},
			setup: setupDetails("pg_locks_count", fullDetails),
			wantStdout: `default_agg: AVG
description: Number of locks currently held, grouped by relation and lock mode.
labels:
  - description: The database this row's values belong to.
    name: datname
  - description: ""
    name: mode
name: pg_locks_count
type: GAUGE
`,
		},
	})
}
