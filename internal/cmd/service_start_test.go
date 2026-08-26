package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceStartCmd(t *testing.T) {
	setupStart := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := sampleService(func(s *api.Service) { s.Status = status })
			m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "svc-12345").
				Return(&api.StartServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "start", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: "authentication required: not logged in. Please run 'tiger auth login'",
			check:   checkExitCode(common.ExitAuthenticationError),
		},
		{
			name:    "read-only mode",
			args:    []string{"service", "start", "svc-12345"},
			opts:    []runOption{withConfig(map[string]any{"read_only": true})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "missing service id",
			args:    []string{"service", "start"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"service", "start", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to start Service: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "start", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.StartServiceResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.Error{Message: new("service is already running")},
					}, nil)
			},
			wantErr: "service is already running",
			check:   checkExitCode(common.ExitInvalidParameters),
		},
		{
			name: "service not found",
			args: []string{"service", "start", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.StartServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			check:   checkExitCode(common.ExitServiceNotFound),
		},
		{
			name: "nil response body",
			args: []string{"service", "start", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.StartServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:  "no-wait",
			args:  []string{"service", "start", "svc-12345", "--no-wait"},
			setup: setupStart(api.DeployStatusRESUMING),
			wantStderr: "▶️  Start request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service get' to check service status.\n",
		},
		{
			name:  "wait returns immediately when already ready",
			args:  []string{"service", "start", "svc-12345"},
			setup: setupStart(api.DeployStatusREADY),
			wantStderr: "▶️  Start request accepted for service 'svc-12345'.\n" +
				"⏳ Waiting for service to start (wait timeout: 10m0s)...\n" +
				"✅ Service has been successfully started!\n",
		},
		{
			name: "wait polls until ready",
			args: []string{"service", "start", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupStart(api.DeployStatusRESUMING)(m)
				ready := sampleService()
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &ready,
					}, nil)
			},
			wantStderr: "▶️  Start request accepted for service 'svc-12345'.\n" +
				"⏳ Waiting for service to start (wait timeout: 10m0s)...\n" +
				"⢎  Service status: RESUMING\n" +
				"✅ Service has been successfully started!\n",
		},
		{
			name:    "service fails during wait",
			args:    []string{"service", "start", "svc-12345"},
			setup:   setupStart(api.DeployStatus("FAILED")),
			wantErr: "service failed with status: FAILED",
			wantStderr: "▶️  Start request accepted for service 'svc-12345'.\n" +
				"⏳ Waiting for service to start (wait timeout: 10m0s)...\n" +
				"❌ Error: service failed with status: FAILED\n",
		},
		{
			name: "wait timeout",
			args: []string{"service", "start", "svc-12345", "--wait-timeout", "1ms"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupStart(api.DeployStatusRESUMING)(m)
				// The 1ms deadline fires before the 1s poll tick, so no
				// GetService call is expected — but allow it in case the test
				// runs slowly, keeping the service unready either way. (The
				// non-TTY spinner dedupes repeated messages, so this can't
				// add stderr lines.)
				resuming := sampleService(func(s *api.Service) { s.Status = api.DeployStatusRESUMING })
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &resuming,
					}, nil).AnyTimes()
			},
			wantErr: "wait timeout reached after 1ms - service may still be starting",
			wantStderr: "▶️  Start request accepted for service 'svc-12345'.\n" +
				"⏳ Waiting for service to start (wait timeout: 1ms)...\n" +
				"⢎  Service status: RESUMING\n" +
				"❌ Error: wait timeout reached after 1ms - service may still be starting\n",
			check: checkExitCode(common.ExitTimeout),
		},
		{
			name:  "default service id from config",
			args:  []string{"service", "start", "--no-wait"},
			opts:  []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup: setupStart(api.DeployStatusRESUMING),
			wantStderr: "▶️  Start request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service get' to check service status.\n",
		},
		{
			name:  "resume alias",
			args:  []string{"service", "resume", "svc-12345", "--no-wait"},
			setup: setupStart(api.DeployStatusRESUMING),
			wantStderr: "▶️  Start request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service get' to check service status.\n",
		},
	}

	runCmdTests(t, tests)
}
