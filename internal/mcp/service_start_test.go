package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceStart(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}
	waitArgs := map[string]any{"service_id": "e6ue9697jf", "wait": true}

	withStatus := func(status api.DeployStatus) api.Service {
		return sampleService(func(s *api.Service) { s.Status = status })
	}
	expectStart := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := withStatus(status)
			m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.StartServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}

	accepted := map[string]any{
		"status":  "QUEUED",
		"message": "Service start request accepted. The service may still be starting.",
	}
	started := map[string]any{
		"status":  "READY",
		"message": "Service started.",
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceStart,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "rejects a malformed service ID",
			tool:    toolServiceStart,
			args:    map[string]any{"service_id": "nope"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "nope" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceStart,
			args:    args,
			opts:    []runOption{withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// Only the tag lookup is registered: an attempted start fails as an
			// unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceStart,
			args:    args,
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `service e6ue9697jf: this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name: "read-only prod allows DEV service",
			tool: toolServiceStart,
			args: args,
			opts: []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				expectStart(api.DeployStatusQUEUED)(m)
			},
			wantOutput: accepted,
		},
		{
			name: "network error",
			tool: toolServiceStart,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to start service: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceStart,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.StartServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "nil response body",
			tool: toolServiceStart,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.StartServiceResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "starts without waiting by default",
			tool:       toolServiceStart,
			args:       args,
			mock:       expectStart(api.DeployStatusQUEUED),
			wantOutput: accepted,
		},
		{
			// Already at the target status, so the wait returns without polling.
			name:       "wait returns immediately when the service is already ready",
			tool:       toolServiceStart,
			args:       waitArgs,
			mock:       expectStart(api.DeployStatusREADY),
			wantOutput: started,
		},
		{
			name:     "wait polls until the service is ready",
			synctest: true,
			tool:     toolServiceStart,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStart(api.DeployStatusRESUMING)(m)
				expectGetService(m, "e6ue9697jf", withStatus(api.DeployStatusREADY))
			},
			wantOutput: started,
		},
		{
			// A failed wait is reported in the message rather than as an error:
			// the start was accepted either way.
			name:     "wait reports a failed poll in the message",
			synctest: true,
			tool:     toolServiceStart,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStart(api.DeployStatusRESUMING)(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusNotFound)}, nil)
			},
			wantOutput: map[string]any{
				"status":  "RESUMING",
				"message": "Error: service not found",
			},
		},
		{
			// The full 10-minute timeout elapses instantly in the bubble.
			// AnyTimes because the loop polls once a second for the whole of
			// it: the count is timer-driven, not something the case asserts.
			name:     "wait reports a timeout in the message",
			synctest: true,
			tool:     toolServiceStart,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStart(api.DeployStatusRESUMING)(m)
				expectGetService(m, "e6ue9697jf", withStatus(api.DeployStatusRESUMING)).AnyTimes()
			},
			wantOutput: map[string]any{
				"status":  "RESUMING",
				"message": "Error: wait timeout reached after 10m0s - service may still be starting",
			},
		},
	})
}
