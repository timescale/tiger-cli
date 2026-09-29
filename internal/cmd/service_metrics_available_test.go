package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceMetricsAvailableCmd(t *testing.T) {
	names := []string{"pg_stat_activity_count", "timescale_cloud_system_cpu_usage_millicores"}

	setupAvailable := func(names []string) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "svc-12345").
				Return(&api.GetServiceMetricsAvailableSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &names,
				}, nil)
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "metrics", "available", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing service id",
			args:    []string{"service", "metrics", "available"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"service", "metrics", "available", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list metric series: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "metrics", "available", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceMetricsAvailableSeriesResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name:       "table output",
			args:       []string{"service", "metrics", "available", "svc-12345"},
			mock:       setupAvailable(names),
			wantStdout: "pg_stat_activity_count\ntimescale_cloud_system_cpu_usage_millicores\n",
		},
		{
			name:       "default service id from config",
			args:       []string{"service", "metrics", "available"},
			opts:       []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			mock:       setupAvailable(names),
			wantStdout: "pg_stat_activity_count\ntimescale_cloud_system_cpu_usage_millicores\n",
		},
		{
			name:       "json output",
			args:       []string{"service", "metrics", "available", "svc-12345", "-o", "json"},
			mock:       setupAvailable(names),
			wantStdout: "[\n  \"pg_stat_activity_count\",\n  \"timescale_cloud_system_cpu_usage_millicores\"\n]\n",
		},
	})
}
