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

	setupAvailableBody := func(body *[]string) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRefID(m, "svc-12345")
			m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "svc-12345").
				Return(&api.GetServiceMetricsAvailableSeriesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      body,
				}, nil)
		}
	}
	setupAvailable := func(names []string) func(m *mocks.MockClientWithResponsesInterface) {
		return setupAvailableBody(&names)
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
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "ambiguous name refused",
			args: []string{"service", "metrics", "available", "my-api-db"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "network error",
			args: []string{"service", "metrics", "available", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetServiceMetricsAvailableSeriesWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to call API: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "metrics", "available", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
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
			name:    "nil response body",
			args:    []string{"service", "metrics", "available", "svc-12345"},
			mock:    setupAvailableBody(nil),
			wantErr: "empty response from API",
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
		{
			// A JSON `null` array is normalized to an empty one, so the output
			// stays a valid array.
			name:       "null series array json",
			args:       []string{"service", "metrics", "available", "svc-12345", "-o", "json"},
			mock:       setupAvailableBody(new([]string)),
			wantStdout: "[]\n",
		},
	})
}
