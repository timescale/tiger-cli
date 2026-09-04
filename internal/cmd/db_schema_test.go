package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestDbSchemaCmd(t *testing.T) {
	setupGetWithStatus := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
				s.Status = status
			}))
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "schema", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "missing service id",
			args:    []string{"db", "schema"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			// Paused readiness stops the command before any connection attempt,
			// proving the config default reached the service lookup.
			name:    "default service id from config",
			args:    []string{"db", "schema"},
			opts:    []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup:   setupGetWithStatus(api.DeployStatusPAUSED),
			wantErr: pausedMsg("svc-12345"),
		},
		{
			name: "network error",
			args: []string{"db", "schema", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "API error",
			args: []string{"db", "schema", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"db", "schema", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:    "service paused",
			args:    []string{"db", "schema", "svc-12345"},
			setup:   setupGetWithStatus(api.DeployStatusPAUSED),
			wantErr: pausedMsg("svc-12345"),
		},
		{
			name:    "service pausing",
			args:    []string{"db", "schema", "svc-12345"},
			setup:   setupGetWithStatus(api.DeployStatusPAUSING),
			wantErr: pausedMsg("svc-12345"),
		},
		{
			name:    "service not ready",
			args:    []string{"db", "schema", "svc-12345"},
			setup:   setupGetWithStatus(api.DeployStatusQUEUED),
			wantErr: notReadyMsg("svc-12345"),
		},
		{
			name:    "pooled without pooler",
			args:    []string{"db", "schema", "svc-12345", "--pooled"},
			setup:   setupGetWithStatus(api.DeployStatusREADY),
			wantErr: "connection pooler not available for this service",
		},
		{
			// The replica has no pooler, so --pooled warns and falls back; the
			// not-ready status then stops the command before any connection.
			name: "replica pooled without pooler warns before readiness check",
			args: []string{"db", "schema", "rep-67890", "--pooled"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "rep-67890", sampleReplica(func(s *api.Service) {
					s.Status = api.DeployStatusQUEUED
				}))
				expectGetService(m, "svc-12345", sampleService())
			},
			wantErr:    notReadyMsg("rep-67890"),
			wantStderr: "⚠️  Warning: read replica \"replica-service\" has no connection pooler; connecting directly instead\nError: " + notReadyMsg("rep-67890") + "\n",
		},
	})
}
