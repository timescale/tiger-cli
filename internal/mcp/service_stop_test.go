package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceStop(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}
	waitArgs := map[string]any{"service_id": "e6ue9697jf", "wait": true}

	withStatus := func(status api.DeployStatus) api.Service {
		return sampleService(func(s *api.Service) { s.Status = status })
	}
	expectStop := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := withStatus(status)
			m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.StopServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}

	accepted := map[string]any{
		"status":  "PAUSING",
		"message": "Service stop request accepted. The service may still be stopping.",
	}
	stopped := map[string]any{
		"status":  "PAUSED",
		"message": "Service stopped.",
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceStop,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "rejects a malformed service ID",
			tool:    toolServiceStop,
			args:    map[string]any{"service_id": "nope"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "nope" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceStop,
			args:    args,
			opts:    []runOption{withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// Only the tag lookup is registered: an attempted stop fails as an
			// unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceStop,
			args:    args,
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `service e6ue9697jf: this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name: "read-only prod allows DEV service",
			tool: toolServiceStop,
			args: args,
			opts: []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				expectStop(api.DeployStatusPAUSING)(m)
			},
			wantOutput: accepted,
		},
		{
			name: "network error",
			tool: toolServiceStop,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to stop service: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceStop,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.StopServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "nil response body",
			tool: toolServiceStop,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.StopServiceResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "stops without waiting by default",
			tool:       toolServiceStop,
			args:       args,
			mock:       expectStop(api.DeployStatusPAUSING),
			wantOutput: accepted,
		},
		{
			// Already at the target status, so the wait returns without polling.
			name:       "wait returns immediately when the service is already paused",
			tool:       toolServiceStop,
			args:       waitArgs,
			mock:       expectStop(api.DeployStatusPAUSED),
			wantOutput: stopped,
		},
		{
			name:     "wait polls until the service is paused",
			synctest: true,
			tool:     toolServiceStop,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStop(api.DeployStatusPAUSING)(m)
				expectGetService(m, "e6ue9697jf", withStatus(api.DeployStatusPAUSED))
			},
			wantOutput: stopped,
		},
		{
			// A failed wait is reported in the message rather than as an error:
			// the stop was accepted either way.
			name:     "wait reports a failed poll in the message",
			synctest: true,
			tool:     toolServiceStop,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStop(api.DeployStatusPAUSING)(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantOutput: map[string]any{
				"status":  "PAUSING",
				"message": "Error: no response body returned from API",
			},
		},
		{
			// The full 10-minute timeout elapses instantly in the bubble.
			// AnyTimes because the loop polls once a second for the whole of
			// it: the count is timer-driven, not something the case asserts.
			name:     "wait reports a timeout in the message",
			synctest: true,
			tool:     toolServiceStop,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStop(api.DeployStatusPAUSING)(m)
				expectGetService(m, "e6ue9697jf", withStatus(api.DeployStatusPAUSING)).AnyTimes()
			},
			wantOutput: map[string]any{
				"status":  "PAUSING",
				"message": "Error: wait timeout reached after 10m0s - service may still be stopping",
			},
		},
	})
}
