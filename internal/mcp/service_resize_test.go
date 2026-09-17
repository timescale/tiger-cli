package mcp

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceResize(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "2 CPU/8 GB"}
	waitArgs := map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "2 CPU/8 GB", "wait": true}

	// The resized service, as the API reports it once the new spec is applied.
	resizedService := func(status api.DeployStatus) api.Service {
		return sampleService(func(s *api.Service) {
			s.Status = status
			s.Resources = []api.Resource{{
				Spec: &api.ResourceSpec{CPUMillis: new(2000), MemoryGbs: new(8)},
			}}
		})
	}
	expectResize := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := resizedService(status)
			m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ResizeInput{
				CPUMillis: "2000",
				MemoryGbs: "8",
			}).
				Return(&api.ResizeServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}
	// expectPoll registers the wait loop's single status check.
	expectPoll := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := resizedService(status)
			m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.GetServiceResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &svc,
				}, nil)
		}
	}
	// expectPollUntilTimeout keeps the service short of the target status,
	// so the loop polls once a second until the wait times out. AnyTimes
	// because the call count is whatever the timeout divided by the poll
	// interval works out to, not a number the case is asserting.
	expectPollUntilTimeout := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := resizedService(status)
			m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.GetServiceResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &svc,
				}, nil).
				AnyTimes()
		}
	}

	resources := map[string]any{"cpu": "2 cores", "memory": "8 GB"}
	accepted := map[string]any{
		"status":    "CONFIGURING",
		"resources": resources,
		"message":   "Resize request accepted. The service may still be resizing.",
	}
	resized := map[string]any{
		"status":    "READY",
		"resources": resources,
		"message":   "Service resized.",
	}
	const enumErr = `validating "arguments": validating root: validating /properties/cpu_memory: enum: %s does not equal any of: ` +
		`[0.5 CPU/2 GB 1 CPU/4 GB 2 CPU/8 GB 4 CPU/16 GB 8 CPU/32 GB 16 CPU/64 GB 32 CPU/128 GB]`

	runToolTests(t, []toolTest{
		{
			name:      "not logged in",
			tool:      toolServiceResize,
			args:      args,
			clientErr: errNotLoggedIn,
			wantErr:   errNotLoggedIn.Error(),
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
			name:    "rejects an unsupported CPU/memory combination",
			tool:    toolServiceResize,
			args:    map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "3 CPU/12 GB"},
			wantErr: fmt.Sprintf(enumErr, "3 CPU/12 GB"),
		},
		{
			// ParseCPUMemory accepts shared, but a service can't be resized onto
			// the free tier, so the enum leaves it out.
			name:    "rejects resizing to shared resources",
			tool:    toolServiceResize,
			args:    map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "shared/shared"},
			wantErr: fmt.Sprintf(enumErr, "shared/shared"),
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:             "read-only all refuses without an API call",
			tool:             toolServiceResize,
			args:             args,
			configAfterStart: map[string]any{"read_only": "all"},
			wantErr:          "this operation is not allowed in read-only mode",
		},
		{
			// Only the tag lookup is registered: an attempted resize fails as an
			// unexpected call.
			name:      "read-only prod refuses PROD service",
			tool:      toolServiceResize,
			args:      args,
			config:    map[string]any{"read_only": "prod"},
			setupMock: expectTaggedService("PROD", 1),
			wantErr:   `service e6ue9697jf: this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:   "read-only prod allows DEV service",
			tool:   toolServiceResize,
			args:   args,
			config: map[string]any{"read_only": "prod"},
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				expectResize(api.DeployStatusCONFIGURING)(m)
			},
			wantOutput: accepted,
		},
		{
			name: "resize API error",
			tool: toolServiceResize,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ResizeInput{
					CPUMillis: "2000",
					MemoryGbs: "8",
				}).
					Return(&api.ResizeServiceResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.ClientError{Message: new("service is not running")},
					}, nil)
			},
			wantErr: "service is not running",
		},
		{
			name: "empty response body",
			tool: toolServiceResize,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ResizeInput{
					CPUMillis: "2000",
					MemoryGbs: "8",
				}).
					Return(&api.ResizeServiceResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "resizes without waiting by default",
			tool:       toolServiceResize,
			args:       args,
			setupMock:  expectResize(api.DeployStatusCONFIGURING),
			wantOutput: accepted,
		},
		{
			name: "wait polls until the service is ready",
			tool: toolServiceResize,
			args: waitArgs,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResize(api.DeployStatusCONFIGURING)(m)
				expectPoll(api.DeployStatusREADY)(m)
			},
			synctest:   true,
			wantOutput: resized,
		},
		{
			// Already at the target status, so the wait returns without polling.
			name:       "wait returns immediately when the service is already ready",
			tool:       toolServiceResize,
			args:       waitArgs,
			setupMock:  expectResize(api.DeployStatusREADY),
			wantOutput: resized,
		},
		{
			// The full 10-minute timeout elapses instantly in the bubble.
			name: "wait reports a timeout in the message",
			tool: toolServiceResize,
			args: waitArgs,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResize(api.DeployStatusCONFIGURING)(m)
				expectPollUntilTimeout(api.DeployStatusCONFIGURING)(m)
			},
			synctest: true,
			wantOutput: map[string]any{
				"status":    "CONFIGURING",
				"resources": resources,
				"message":   "Error: wait timeout reached after 10m0s - service may still be resizing",
			},
		},
	})
}
