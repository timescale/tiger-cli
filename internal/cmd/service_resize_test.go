package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceResizeCmd(t *testing.T) {
	svc := sampleService()

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only mode",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			opts:    []runOption{withConfig(map[string]any{"read_only": true})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "missing service id",
			args:    []string{"service", "resize", "--cpu", "2000", "--memory", "8"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "invalid cpu/memory combination",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "3000", "--memory", "8"},
			wantErr: "invalid CPU/Memory combination. Allowed combinations: shared/shared, 0.5 CPU/2 GB, 1 CPU/4 GB, 2 CPU/8 GB, 4 CPU/16 GB, 8 CPU/32 GB, 16 CPU/64 GB, 32 CPU/128 GB",
		},
		{
			name:    "missing cpu and memory",
			args:    []string{"service", "resize", "svc-12345"},
			wantErr: "must specify at least one of --cpu or --memory",
		},
		{
			name: "network error",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: "2000", MemoryGbs: "8"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr:    "failed to resize service: connection refused",
			wantStderr: "📐 Resizing service 'svc-12345' to 2 CPU/8 GB...\nError: failed to resize service: connection refused\n",
		},
		{
			name: "API error",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: "2000", MemoryGbs: "8"}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr:    "service not found",
			wantStderr: "📐 Resizing service 'svc-12345' to 2 CPU/8 GB...\nError: service not found\n",
			checks:     []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: "2000", MemoryGbs: "8"}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantErr:    "empty response from API",
			wantStderr: "📐 Resizing service 'svc-12345' to 2 CPU/8 GB...\nError: empty response from API\n",
		},
		{
			name: "success with wait, service immediately ready",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: "2000", MemoryGbs: "8"}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `📐 Resizing service 'svc-12345' to 2 CPU/8 GB...
✅ Resize request accepted for service 'svc-12345'!
⏳ Waiting for resize to complete (timeout: 10m0s)...
🎉 Service 'svc-12345' has been successfully resized to 2 CPU/8 GB!
`,
		},
		{
			name: "no wait",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8", "--no-wait"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: "2000", MemoryGbs: "8"}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `📐 Resizing service 'svc-12345' to 2 CPU/8 GB...
✅ Resize request accepted for service 'svc-12345'!
💡 Use 'tiger service get' to check service status.
`,
		},
		{
			name: "service id from config, cpu only auto-configures memory",
			args: []string{"service", "resize", "--cpu", "2000", "--no-wait"},
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: "2000", MemoryGbs: "8"}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `📐 Resizing service 'svc-12345' to 2 CPU/8 GB...
✅ Resize request accepted for service 'svc-12345'!
💡 Use 'tiger service get' to check service status.
`,
		},
		{
			name: "memory only auto-configures cpu",
			args: []string{"service", "resize", "svc-12345", "--memory", "16", "--no-wait"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: "4000", MemoryGbs: "16"}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `📐 Resizing service 'svc-12345' to 4 CPU/16 GB...
✅ Resize request accepted for service 'svc-12345'!
💡 Use 'tiger service get' to check service status.
`,
		},
		{
			name: "wait timeout",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8", "--wait-timeout", "50ms"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				configuring := sampleService(func(s *api.Service) {
					s.Status = api.DeployStatusCONFIGURING
				})
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: "2000", MemoryGbs: "8"}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &configuring,
					}, nil)
				// The 50ms deadline fires before the 1s poll tick, so no
				// GetService call is expected — but allow it in case the test
				// runs slowly, keeping the service unready either way.
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &configuring,
					}, nil).AnyTimes()
			},
			wantErr: "wait timeout reached after 50ms - service may still be resizing",
			// SilenceErrors is set after the wait fails, so Cobra doesn't
			// print the usual "Error:" line.
			wantStderr: `📐 Resizing service 'svc-12345' to 2 CPU/8 GB...
✅ Resize request accepted for service 'svc-12345'!
⏳ Waiting for resize to complete (timeout: 50ms)...
⢎  Service status: CONFIGURING
❌ Error: wait timeout reached after 50ms - service may still be resizing
`,
			checks: []checkFunc{checkExitCode(common.ExitTimeout)},
		},
	}

	runCmdTests(t, tests)
}
