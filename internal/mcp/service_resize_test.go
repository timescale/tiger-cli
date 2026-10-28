package mcp

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceResizeTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "2 CPU/8 GB"}
	waitArgs := map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "2 CPU/8 GB", "wait": true}

	resizeReq := api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("8")}

	// The resized service, as the API reports it once the new spec is applied.
	resizedService := func(status api.DeployStatus) api.Service {
		return sampleService(func(s *api.Service) {
			s.Status = status
			s.Resources = []api.Resource{{
				Spec: &api.ResourceSpec{CPUMillis: new(2000), MemoryGbs: new(8)},
			}}
		})
	}
	// expectLookup registers the fetch of the service being resized, whose
	// platform decides the combinations cpu_memory is checked against.
	expectLookup := func(m *mocks.MockClientWithResponsesInterface) {
		expectGetService(m, "e6ue9697jf", sampleService())
	}
	expectResize := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectLookup(m)
			svc := resizedService(status)
			m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", resizeReq).
				Return(&api.ResizeServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}

	resources := map[string]any{"cpu": "2 cores", "memory": "8 GB"}
	accepted := map[string]any{
		"status":    "CONFIGURING",
		"resources": resources,
		"message":   "Service resize request accepted. The service may still be resizing.",
	}
	resized := map[string]any{
		"status":    "READY",
		"resources": resources,
		"message":   "Service resized.",
	}
	const enumMsg = `validating "arguments": validating root: validating /properties/cpu_memory: enum: %s does not equal any of: ` +
		`[0.5 CPU/2 GB 1 CPU/4 GB 2 CPU/8 GB 4 CPU/16 GB 8 CPU/32 GB 16 CPU/64 GB 32 CPU/128 GB]`

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceResize,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "rejects a malformed service ID",
			tool:    toolServiceResize,
			args:    map[string]any{"service_id": "nope", "cpu_memory": "2 CPU/8 GB"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "nope" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "requires cpu_memory",
			tool:    toolServiceResize,
			args:    map[string]any{"service_id": "e6ue9697jf"},
			wantErr: `validating "arguments": validating root: required: missing properties: ["cpu_memory"]`,
		},
		{
			// iops is a preview parameter, so without the gate it isn't in the
			// schema, which rejects unknown properties.
			name:    "iops rejected without the experimental gate",
			tool:    toolServiceResize,
			args:    map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "2 CPU/8 GB", "iops": 12000},
			wantErr: `validating "arguments": validating root: unexpected additional properties ["iops"]`,
		},
		{
			name:    "rejects an unsupported CPU/memory combination",
			tool:    toolServiceResize,
			args:    map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "3 CPU/12 GB"},
			wantErr: fmt.Sprintf(enumMsg, "3 CPU/12 GB"),
		},
		{
			// The fluid platform is a preview, so its combinations are left out
			// of the enum without the experimental gate.
			name:    "rejects a fluid combination without the experimental gate",
			tool:    toolServiceResize,
			args:    map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "2 CPU/16 GB"},
			wantErr: fmt.Sprintf(enumMsg, "2 CPU/16 GB"),
		},
		{
			// A service can't be resized onto the free tier, so the enum leaves
			// shared out.
			name:    "rejects resizing to shared resources",
			tool:    toolServiceResize,
			args:    map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "shared/shared"},
			wantErr: fmt.Sprintf(enumMsg, "shared/shared"),
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceResize,
			args:    args,
			opts:    []runOption{withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// Only the lookup is registered: an attempted resize fails as an
			// unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceResize,
			args:    args,
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name: "read-only prod allows DEV service",
			tool: toolServiceResize,
			args: args,
			opts: []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				// The one lookup serves both the read-only gate and the
				// cpu_memory check.
				expectTaggedService("DEV", 1)(m)
				svc := resizedService(api.DeployStatusCONFIGURING)
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", resizeReq).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantOutput: accepted,
		},
		{
			name: "service lookup error",
			tool: toolServiceResize,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			// Not gated on experimental: only a fluid service gets these.
			name: "cpu_memory must match the service's platform",
			tool: toolServiceResize,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) {
					s.Platform = new(api.ServicePlatformFLUID)
				}))
			},
			wantErr: "invalid CPU/Memory specification: invalid CPU/Memory combination '2 CPU/8 GB'. Valid options: 0.25 CPU/2 GB, 0.5 CPU/4 GB, 1 CPU/8 GB, 2 CPU/16 GB, 4 CPU/32 GB, 8 CPU/64 GB, 16 CPU/128 GB, 32 CPU/256 GB",
		},
		{
			// Behind the gate cpu_memory is optional, so the handler requires
			// at least one of the two after the lookup.
			name:    "experimental requires cpu_memory or iops",
			tool:    toolServiceResize,
			args:    map[string]any{"service_id": "e6ue9697jf"},
			opts:    []runOption{withExperimental()},
			mock:    expectLookup,
			wantErr: "at least one of cpu_memory or iops is required",
		},
		{
			name: "network error",
			tool: toolServiceResize,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectLookup(m)
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", resizeReq).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to resize service: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceResize,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectLookup(m)
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", resizeReq).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.ClientError{Message: new("service is not running")},
					}, nil)
			},
			wantErr: "service is not running",
		},
		{
			name: "nil response body",
			tool: toolServiceResize,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectLookup(m)
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", resizeReq).
					Return(&api.ResizeServiceResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "resizes without waiting by default",
			tool:       toolServiceResize,
			args:       args,
			mock:       expectResize(api.DeployStatusCONFIGURING),
			wantOutput: accepted,
		},
		{
			name: "experimental passes a fluid combination through for a fluid service",
			tool: toolServiceResize,
			args: map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "2 CPU/16 GB"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) {
					s.Platform = new(api.ServicePlatformFLUID)
				}))
				svc := sampleService(func(s *api.Service) {
					s.Status = api.DeployStatusCONFIGURING
					s.Platform = new(api.ServicePlatformFLUID)
					s.Resources = []api.Resource{{
						Spec: &api.ResourceSpec{CPUMillis: new(2000), MemoryGbs: new(16)},
					}}
				})
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ResizeInput{CPUMillis: new("2000"), MemoryGbs: new("16")}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantOutput: map[string]any{
				"status":    "CONFIGURING",
				"resources": map[string]any{"cpu": "2 cores", "memory": "16 GB"},
				"message":   "Service resize request accepted. The service may still be resizing.",
			},
		},
		{
			// CPU and memory are omitted, which the API leaves unchanged. The
			// API validates the IOPS value.
			name: "experimental iops only omits cpu and memory",
			tool: toolServiceResize,
			args: map[string]any{"service_id": "e6ue9697jf", "iops": 12000},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				fluid := func(iops int) api.Service {
					return sampleService(func(s *api.Service) {
						s.Platform = new(api.ServicePlatformFLUID)
						s.Resources = []api.Resource{{
							Spec: &api.ResourceSpec{CPUMillis: new(2000), MemoryGbs: new(16), Iops: new(iops)},
						}}
					})
				}
				expectGetService(m, "e6ue9697jf", fluid(6000))
				svc := fluid(12000)
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ResizeInput{Iops: new(12000)}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantOutput: map[string]any{
				"status":    "READY",
				"resources": map[string]any{"cpu": "2 cores", "memory": "16 GB", "iops": float64(12000)},
				"message":   "Service resize request accepted. The service may still be resizing.",
			},
		},
		{
			// Already at the target status, so the wait returns without polling.
			name:       "wait returns immediately when the service is already ready",
			tool:       toolServiceResize,
			args:       waitArgs,
			mock:       expectResize(api.DeployStatusREADY),
			wantOutput: resized,
		},
		{
			name:     "wait polls until the service is ready",
			synctest: true,
			tool:     toolServiceResize,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResize(api.DeployStatusCONFIGURING)(m)
				expectGetService(m, "e6ue9697jf", resizedService(api.DeployStatusREADY))
			},
			wantOutput: resized,
		},
		{
			// A failed wait is reported in the message rather than as an error:
			// the resize was accepted either way.
			name:     "wait reports a failed poll in the message",
			synctest: true,
			tool:     toolServiceResize,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResize(api.DeployStatusCONFIGURING)(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusNotFound)}, nil)
			},
			wantOutput: map[string]any{
				"status":    "CONFIGURING",
				"resources": resources,
				"message":   "Error: service not found",
			},
		},
		{
			// The full 10-minute timeout elapses instantly in the bubble.
			// AnyTimes because the loop polls once a second for the whole of
			// it: the count is timer-driven, not something the case asserts.
			name:     "wait reports a timeout in the message",
			synctest: true,
			tool:     toolServiceResize,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResize(api.DeployStatusCONFIGURING)(m)
				expectGetService(m, "e6ue9697jf", resizedService(api.DeployStatusCONFIGURING)).AnyTimes()
			},
			wantOutput: map[string]any{
				"status":    "CONFIGURING",
				"resources": resources,
				"message":   "Error: wait timeout reached after 10m0s - service may still be resizing",
			},
		},
	})
}
