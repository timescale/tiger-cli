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
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	// A fluid service at 1 CPU/8 GB.
	fluidSvc := sampleService(fluidPlatform, func(s *api.Service) {
		s.Resources[0].Spec.MemoryGbs = new(8)
	})

	runCmdTests(t, []cmdTest{
		{
			// The fluid platform and IOPS are a preview, so their help text is gated.
			name:       "help omits the fluid platform and IOPS",
			args:       []string{"service", "resize", "--help"},
			wantStdout: matchHelp("Allowed CPU/Memory Configurations:", "fluid", "IOPS", "--iops"),
		},
		{
			name: "experimental help lists the fluid platform configurations",
			args: []string{"service", "resize", "--help"},
			opts: []runOption{experimental},
			wantStdout: matchHelp(`Allowed CPU/Memory Configurations (fluid platform):
  0.25 CPU (250m) / 2GB  |  0.5 CPU (500m) / 4GB  |  1 CPU (1000m) / 8GB      |  2 CPU (2000m) / 16GB
  4 CPU (4000m) / 32GB   |  8 CPU (8000m) / 64GB  |  16 CPU (16000m) / 128GB  |  32 CPU (32000m) / 256GB`),
		},
		{
			name: "experimental help says resize changes IOPS",
			args: []string{"service", "resize", "--help"},
			opts: []runOption{experimental},
			wantStdout: matchHelp(`Resize a database service by changing its CPU, memory, and provisioned IOPS
allocation.`),
		},
		{
			name: "experimental help says where to find the allowed IOPS values",
			args: []string{"service", "resize", "--help"},
			opts: []runOption{experimental},
			wantStdout: matchHelp(`The allowed --iops values depend on your plan, the service's platform, and its
CPU/memory allocation. See the Tiger Data documentation for details. If only
--iops is given, the service keeps its current CPU and memory.`),
		},
		{
			name:    "zero wait timeout",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "2000", "--wait-timeout", "0s"},
			wantErr: "wait timeout must be positive, got 0s",
		},
		{
			name:    "negative wait timeout",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "2000", "--wait-timeout=-30m"},
			wantErr: "wait timeout must be positive, got -30m0s",
		},
		{
			name:    "not logged in",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only all refuses",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// prod judges the service by its environment tag, so the gate
			// fetches it. Only the tag lookup is registered: an attempted
			// mutation fails as an unexpected call.
			name:    "read-only prod refuses PROD service",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD"),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name: "read-only prod allows DEV service",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8", "--no-wait"},
			opts: []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV")(m)
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("8")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `Resizing service 'test-service' (svc-12345) to 2 CPU/8 GB...
Resize request accepted for service 'test-service' (svc-12345).
Use 'tiger service get svc-12345' to check service status.
`,
		},
		{
			name:    "missing service id",
			args:    []string{"service", "resize", "--cpu", "2000", "--memory", "8"},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "missing cpu and memory",
			args:    []string{"service", "resize", "svc-12345"},
			wantErr: "must specify at least one of --cpu or --memory",
		},
		{
			// --iops is a preview flag, so without the gate it doesn't exist.
			name:    "iops flag rejected without experimental",
			args:    []string{"service", "resize", "svc-12345", "--iops", "12000"},
			wantErr: "unknown flag: --iops",
		},
		{
			name:    "experimental missing cpu, memory, and iops",
			args:    []string{"service", "resize", "svc-12345"},
			opts:    []runOption{experimental},
			wantErr: "must specify at least one of --cpu, --memory, or --iops",
		},
		{
			name: "ambiguous name refused",
			args: []string{"service", "resize", "my-api-db", "--cpu", "2000", "--memory", "8"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			// The combination is validated against the service's platform, so
			// the service is fetched first.
			name: "invalid cpu/memory combination",
			args: []string{"service", "resize", "svc-12345", "--cpu", "3000", "--memory", "8"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
			},
			wantErr: "invalid CPU/Memory combination. Allowed combinations: 0.5 CPU/2 GB, 1 CPU/4 GB, 2 CPU/8 GB, 4 CPU/16 GB, 8 CPU/32 GB, 16 CPU/64 GB, 32 CPU/128 GB",
		},
		{
			name: "shared rejected",
			args: []string{"service", "resize", "svc-12345", "--cpu", "shared"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
			},
			wantErr: "invalid CPU/Memory combination. Allowed combinations: 0.5 CPU/2 GB, 1 CPU/4 GB, 2 CPU/8 GB, 4 CPU/16 GB, 8 CPU/32 GB, 16 CPU/64 GB, 32 CPU/128 GB",
		},
		{
			// Not gated on experimental: only a fluid service gets these.
			name: "fluid service rejects a standard combination",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService(fluidPlatform))
			},
			wantErr: "invalid CPU/Memory combination. Allowed combinations: 0.25 CPU/2 GB, 0.5 CPU/4 GB, 1 CPU/8 GB, 2 CPU/16 GB, 4 CPU/32 GB, 8 CPU/64 GB, 16 CPU/128 GB, 32 CPU/256 GB",
		},
		{
			name: "network error",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("8")}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr:    "failed to resize service: connection refused",
			wantStderr: "Resizing service 'test-service' (svc-12345) to 2 CPU/8 GB...\nError: failed to resize service: connection refused\n",
		},
		{
			name: "API error",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("8")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr:    "service not found",
			wantStderr: "Resizing service 'test-service' (svc-12345) to 2 CPU/8 GB...\nError: service not found\n",
			checks:     []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("8")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantErr:    "empty response from API",
			wantStderr: "Resizing service 'test-service' (svc-12345) to 2 CPU/8 GB...\nError: empty response from API\n",
		},
		{
			name: "success with wait, service immediately ready",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("8")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `Resizing service 'test-service' (svc-12345) to 2 CPU/8 GB...
Resize request accepted for service 'test-service' (svc-12345).
Waiting for resize to complete (timeout: 10m0s)...
Service resized to 2 CPU/8 GB.
`,
		},
		{
			name: "no wait",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8", "--no-wait"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("8")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `Resizing service 'test-service' (svc-12345) to 2 CPU/8 GB...
Resize request accepted for service 'test-service' (svc-12345).
Use 'tiger service get svc-12345' to check service status.
`,
		},
		{
			name: "service id from config, cpu only auto-configures memory",
			args: []string{"service", "resize", "--cpu", "2000", "--no-wait"},
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("8")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `Resizing service 'test-service' (svc-12345) to 2 CPU/8 GB...
Resize request accepted for service 'test-service' (svc-12345).
Use 'tiger service get svc-12345' to check service status.
`,
		},
		{
			name: "memory only auto-configures cpu",
			args: []string{"service", "resize", "svc-12345", "--memory", "16", "--no-wait"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("4000"), MemoryGbs: new("16")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `Resizing service 'test-service' (svc-12345) to 4 CPU/16 GB...
Resize request accepted for service 'test-service' (svc-12345).
Use 'tiger service get svc-12345' to check service status.
`,
		},
		{
			name: "fluid service cpu only auto-configures memory",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--no-wait"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService(fluidPlatform))
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("16")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStderr: `Resizing service 'test-service' (svc-12345) to 2 CPU/16 GB...
Resize request accepted for service 'test-service' (svc-12345).
Use 'tiger service get svc-12345' to check service status.
`,
		},
		{
			// CPU and memory are omitted, which the API leaves unchanged. The
			// API validates the IOPS value.
			name: "experimental iops only omits cpu and memory",
			args: []string{"service", "resize", "svc-12345", "--iops", "12000", "--no-wait"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", fluidSvc)
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{Iops: new(12000)}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &fluidSvc,
					}, nil)
			},
			wantStderr: `Resizing service 'test-service' (svc-12345) to 12000 IOPS...
Resize request accepted for service 'test-service' (svc-12345).
Use 'tiger service get svc-12345' to check service status.
`,
		},
		{
			name: "experimental iops with cpu, with wait",
			args: []string{"service", "resize", "svc-12345", "--cpu", "2000", "--iops", "16000"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", fluidSvc)
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("16"), Iops: new(16000)}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &fluidSvc,
					}, nil)
			},
			wantStderr: `Resizing service 'test-service' (svc-12345) to 2 CPU/16 GB, 16000 IOPS...
Resize request accepted for service 'test-service' (svc-12345).
Waiting for resize to complete (timeout: 10m0s)...
Service resized to 2 CPU/16 GB, 16000 IOPS.
`,
		},
		{
			name:     "wait timeout",
			synctest: true,
			args:     []string{"service", "resize", "svc-12345", "--cpu", "2000", "--memory", "8"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService())
				configuring := sampleService(func(s *api.Service) {
					s.Status = api.DeployStatusCONFIGURING
				})
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("8")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &configuring,
					}, nil)
				// The service never reaches the target state, so the wait
				// polls until the (virtual) deadline. The non-TTY spinner
				// dedupes repeated messages, so those polls add no stderr lines.
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &configuring,
					}, nil).AnyTimes()
			},
			wantErr: "wait timeout reached after 10m0s - service may still be resizing",
			// SilenceErrors is set after the wait fails, so Cobra doesn't
			// print the usual "Error:" line.
			wantStderr: `Resizing service 'test-service' (svc-12345) to 2 CPU/8 GB...
Resize request accepted for service 'test-service' (svc-12345).
Waiting for resize to complete (timeout: 10m0s)...
⢎  Service status: CONFIGURING
Error: wait timeout reached after 10m0s - service may still be resizing
`,
			checks: []checkFunc{checkExitCode(common.ExitTimeout)},
		},
	})
}
