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
	maxTotal := api.MetricsAggFnMAXTOTAL
	gauge := api.MetricTypeGAUGE

	// pg_stat_activity_count: its own labels (datname, state, usename,
	// backend_type) carry no description yet, plus the region/role/ordinal
	// labels every registry-backed metric gets, which do.
	fullDetails := api.MetricDetails{
		MetricName:  "pg_stat_activity_count",
		Type:        &gauge,
		DefaultAgg:  &maxTotal,
		Description: "Number of connections in pg_stat_activity, grouped by state, connected role, and backend type.",
		Labels: []api.MetricLabelDetails{
			{Name: "datname", Description: ""},
			{Name: "state", Description: ""},
			{Name: "usename", Description: ""},
			{Name: "backend_type", Description: ""},
			{Name: "region", Description: "Region the service runs in."},
			{Name: "role", Description: "Instance role: primary, replica, or standby_leader."},
			{Name: "ordinal", Description: "Per-pod ordinal within the service."},
		},
	}

	// A metric with no documented metadata yet: type/default_agg are nil,
	// description and labels are empty.
	undocumentedDetails := api.MetricDetails{
		MetricName: "some_new_metric",
	}

	const fullDetailsTable = `┌─────────────────────┬────────────────────────────────────────────────────────────────────────────────────────────────┐
│      PROPERTY       │                                             VALUE                                              │
├─────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────┤
│ Name                │ pg_stat_activity_count                                                                         │
│ Type                │ GAUGE                                                                                          │
│ Default Aggregation │ MAX_TOTAL                                                                                      │
│ Description         │ Number of connections in pg_stat_activity, grouped by state, connected role, and backend type. │
└─────────────────────┴────────────────────────────────────────────────────────────────────────────────────────────────┘

┌──────────────┬─────────────────────────────────────────────────────┐
│    LABEL     │                     DESCRIPTION                     │
├──────────────┼─────────────────────────────────────────────────────┤
│ datname      │ undocumented                                        │
│ state        │ undocumented                                        │
│ usename      │ undocumented                                        │
│ backend_type │ undocumented                                        │
│ region       │ Region the service runs in.                         │
│ role         │ Instance role: primary, replica, or standby_leader. │
│ ordinal      │ Per-pod ordinal within the service.                 │
└──────────────┴─────────────────────────────────────────────────────┘
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
			expectResolveRefID(m, "svc-12345")
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
			args:    []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_stat_activity_count"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing metric flag",
			args:    []string{"service", "metrics", "details", "svc-12345"},
			wantErr: `required flag(s) "metric" not set`,
		},
		{
			name:    "missing service id",
			args:    []string{"service", "metrics", "details", "--metric", "pg_stat_activity_count"},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "ambiguous name refused",
			args: []string{"service", "metrics", "details", "my-api-db", "--metric", "pg_stat_activity_count"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "network error",
			args: []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_stat_activity_count"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "svc-12345", "pg_stat_activity_count").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get metric details: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_stat_activity_count"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "svc-12345", "pg_stat_activity_count").
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
			args: []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_stat_activity_count"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetServiceMetricDetailsWithResponse(validCtx, testProjectID, "svc-12345", "pg_stat_activity_count").
					Return(&api.GetServiceMetricDetailsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "table output with labels",
			args:       []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_stat_activity_count"},
			mock:       setupDetails("pg_stat_activity_count", fullDetails),
			wantStdout: fullDetailsTable,
		},
		{
			name:       "default service id from config",
			args:       []string{"service", "metrics", "details", "--metric", "pg_stat_activity_count"},
			opts:       []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			mock:       setupDetails("pg_stat_activity_count", fullDetails),
			wantStdout: fullDetailsTable,
		},
		{
			// Undocumented fields (type, default_agg, description) render as
			// "undocumented", and the second table is skipped when there are no
			// labels.
			name:       "table output without labels",
			args:       []string{"service", "metrics", "details", "svc-12345", "--metric", "some_new_metric"},
			mock:       setupDetails("some_new_metric", undocumentedDetails),
			wantStdout: undocumentedTable,
		},
		{
			name: "json output",
			args: []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_stat_activity_count", "-o", "json"},
			mock: setupDetails("pg_stat_activity_count", fullDetails),
			wantStdout: `{
  "default_agg": "MAX_TOTAL",
  "description": "Number of connections in pg_stat_activity, grouped by state, connected role, and backend type.",
  "labels": [
    {
      "description": "",
      "name": "datname"
    },
    {
      "description": "",
      "name": "state"
    },
    {
      "description": "",
      "name": "usename"
    },
    {
      "description": "",
      "name": "backend_type"
    },
    {
      "description": "Region the service runs in.",
      "name": "region"
    },
    {
      "description": "Instance role: primary, replica, or standby_leader.",
      "name": "role"
    },
    {
      "description": "Per-pod ordinal within the service.",
      "name": "ordinal"
    }
  ],
  "metric_name": "pg_stat_activity_count",
  "type": "GAUGE"
}
`,
		},
		{
			name: "yaml output",
			args: []string{"service", "metrics", "details", "svc-12345", "--metric", "pg_stat_activity_count", "-o", "yaml"},
			mock: setupDetails("pg_stat_activity_count", fullDetails),
			wantStdout: `default_agg: MAX_TOTAL
description: Number of connections in pg_stat_activity, grouped by state, connected role, and backend type.
labels:
  - description: ""
    name: datname
  - description: ""
    name: state
  - description: ""
    name: usename
  - description: ""
    name: backend_type
  - description: Region the service runs in.
    name: region
  - description: 'Instance role: primary, replica, or standby_leader.'
    name: role
  - description: Per-pod ordinal within the service.
    name: ordinal
metric_name: pg_stat_activity_count
type: GAUGE
`,
		},
	})
}
