package mcp

import (
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceStop(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}
	waitArgs := map[string]any{"service_id": "e6ue9697jf", "wait": true}

	expectStop := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := sampleService(func(s *api.Service) { s.Status = status })
			m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.StopServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}
	// expectPoll registers the wait loop's single status check.
	expectPoll := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := sampleService(func(s *api.Service) { s.Status = status })
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
			svc := sampleService(func(s *api.Service) { s.Status = status })
			m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.GetServiceResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &svc,
				}, nil).
				AnyTimes()
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
			name:      "not logged in",
			tool:      toolServiceStop,
			args:      args,
			clientErr: errNotLoggedIn,
			wantErr:   errNotLoggedIn.Error(),
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
			name:             "read-only all refuses without an API call",
			tool:             toolServiceStop,
			args:             args,
			configAfterStart: map[string]any{"read_only": "all"},
			wantErr:          "this operation is not allowed in read-only mode",
		},
		{
			// Only the tag lookup is registered: an attempted stop fails as an
			// unexpected call.
			name:      "read-only prod refuses PROD service",
			tool:      toolServiceStop,
			args:      args,
			config:    map[string]any{"read_only": "prod"},
			setupMock: expectTaggedService("PROD", 1),
			wantErr:   `service e6ue9697jf: this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:   "read-only prod allows DEV service",
			tool:   toolServiceStop,
			args:   args,
			config: map[string]any{"read_only": "prod"},
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				expectStop(api.DeployStatusPAUSING)(m)
			},
			wantOutput: accepted,
		},
		{
			name: "stop API error",
			tool: toolServiceStop,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.StopServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "empty response body",
			tool: toolServiceStop,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.StopServiceResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "stops without waiting by default",
			tool:       toolServiceStop,
			args:       args,
			setupMock:  expectStop(api.DeployStatusPAUSING),
			wantOutput: accepted,
		},
		{
			name: "wait polls until the service is paused",
			tool: toolServiceStop,
			args: waitArgs,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStop(api.DeployStatusPAUSING)(m)
				expectPoll(api.DeployStatusPAUSED)(m)
			},
			synctest:   true,
			wantOutput: stopped,
		},
		{
			// Already at the target status, so the wait returns without polling.
			name:       "wait returns immediately when the service is already paused",
			tool:       toolServiceStop,
			args:       waitArgs,
			setupMock:  expectStop(api.DeployStatusPAUSED),
			wantOutput: stopped,
		},
		{
			name: "wait reports a failed poll in the message",
			tool: toolServiceStop,
			args: waitArgs,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStop(api.DeployStatusPAUSING)(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			synctest: true,
			wantOutput: map[string]any{
				"status":  "PAUSING",
				"message": "Error: no response body returned from API",
			},
		},
		{
			// The full 10-minute timeout elapses instantly in the bubble.
			name: "wait reports a timeout in the message",
			tool: toolServiceStop,
			args: waitArgs,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStop(api.DeployStatusPAUSING)(m)
				expectPollUntilTimeout(api.DeployStatusPAUSING)(m)
			},
			synctest: true,
			wantOutput: map[string]any{
				"status":  "PAUSING",
				"message": "Error: wait timeout reached after 10m0s - service may still be stopping",
			},
		},
	})
}
