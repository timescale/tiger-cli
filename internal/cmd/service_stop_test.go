package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceStopCmd(t *testing.T) {
	setupStop := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := sampleService(func(s *api.Service) { s.Status = status })
			m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "svc-12345").
				Return(&api.StopServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}

	wantExitCode := func(code int) func(t *testing.T, result cmdResult) {
		return func(t *testing.T, result cmdResult) {
			var exitErr common.ExitCodeError
			if !errors.As(result.err, &exitErr) {
				t.Fatalf("expected ExitCodeError, got %T: %v", result.err, result.err)
			}
			if exitErr.ExitCode() != code {
				t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), code)
			}
		}
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "stop", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: "authentication required: not logged in. Please run 'tiger auth login'",
			check:   wantExitCode(common.ExitAuthenticationError),
		},
		{
			name:    "read-only mode",
			args:    []string{"service", "stop", "svc-12345"},
			opts:    []runOption{withConfig(map[string]any{"read_only": true})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "missing service id",
			args:    []string{"service", "stop"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"service", "stop", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to stop Service: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "stop", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.StopServiceResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.Error{Message: new("service is already stopped")},
					}, nil)
			},
			wantErr: "service is already stopped",
			check:   wantExitCode(common.ExitInvalidParameters),
		},
		{
			name: "service not found",
			args: []string{"service", "stop", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.StopServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			check:   wantExitCode(common.ExitServiceNotFound),
		},
		{
			name: "nil response body",
			args: []string{"service", "stop", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.StopServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:  "no-wait",
			args:  []string{"service", "stop", "svc-12345", "--no-wait"},
			setup: setupStop(api.DeployStatusPAUSING),
			wantStderr: "⏹️  Stop request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service get' to check service status.\n",
		},
		{
			name:  "wait returns immediately when already paused",
			args:  []string{"service", "stop", "svc-12345"},
			setup: setupStop(api.DeployStatusPAUSED),
			wantStderr: "⏹️  Stop request accepted for service 'svc-12345'.\n" +
				"⏳ Waiting for service to stop (timeout: 10m0s)...\n" +
				"✅ Service has been successfully stopped!\n",
		},
		{
			name: "wait polls until paused",
			args: []string{"service", "stop", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupStop(api.DeployStatusPAUSING)(m)
				paused := sampleService(func(s *api.Service) { s.Status = api.DeployStatusPAUSED })
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &paused,
					}, nil)
			},
			wantStderr: "⏹️  Stop request accepted for service 'svc-12345'.\n" +
				"⏳ Waiting for service to stop (timeout: 10m0s)...\n" +
				"⢎  Service status: PAUSING\n" +
				"✅ Service has been successfully stopped!\n",
		},
		{
			name:    "wait timeout",
			args:    []string{"service", "stop", "svc-12345", "--wait-timeout", "1ms"},
			setup:   setupStop(api.DeployStatusPAUSING),
			wantErr: "wait timeout reached after 1ms - service may still be stopping",
			wantStderr: "⏹️  Stop request accepted for service 'svc-12345'.\n" +
				"⏳ Waiting for service to stop (timeout: 1ms)...\n" +
				"⢎  Service status: PAUSING\n" +
				"❌ Error: wait timeout reached after 1ms - service may still be stopping\n",
			check: wantExitCode(common.ExitTimeout),
		},
		{
			name:  "default service id from config",
			args:  []string{"service", "stop", "--no-wait"},
			opts:  []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup: setupStop(api.DeployStatusPAUSING),
			wantStderr: "⏹️  Stop request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service get' to check service status.\n",
		},
		{
			name:  "pause alias",
			args:  []string{"service", "pause", "svc-12345", "--no-wait"},
			setup: setupStop(api.DeployStatusPAUSING),
			wantStderr: "⏹️  Stop request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service get' to check service status.\n",
		},
	}

	runCmdTests(t, tests)
}
