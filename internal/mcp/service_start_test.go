package mcp

import (
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceStart(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}
	waitArgs := map[string]any{"service_id": "e6ue9697jf", "wait": true}

	expectStart := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := sampleService(func(s *api.Service) { s.Status = status })
			m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.StartServiceResponse{
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
		"status":  "QUEUED",
		"message": "Service start request accepted. The service may still be starting.",
	}
	started := map[string]any{
		"status":  "READY",
		"message": "Service started.",
	}

	runToolTests(t, []toolTest{
		{
			name:      "not logged in",
			tool:      toolServiceStart,
			args:      args,
			clientErr: errNotLoggedIn,
			wantErr:   errNotLoggedIn.Error(),
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
			name:             "read-only all refuses without an API call",
			tool:             toolServiceStart,
			args:             args,
			configAfterStart: map[string]any{"read_only": "all"},
			wantErr:          "this operation is not allowed in read-only mode",
		},
		{
			// Only the tag lookup is registered: an attempted start fails as an
			// unexpected call.
			name:      "read-only prod refuses PROD service",
			tool:      toolServiceStart,
			args:      args,
			config:    map[string]any{"read_only": "prod"},
			setupMock: expectTaggedService("PROD", 1),
			wantErr:   `service e6ue9697jf: this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:   "read-only prod allows DEV service",
			tool:   toolServiceStart,
			args:   args,
			config: map[string]any{"read_only": "prod"},
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				expectStart(api.DeployStatusQUEUED)(m)
			},
			wantOutput: accepted,
		},
		{
			name: "start API error",
			tool: toolServiceStart,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.StartServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "empty response body",
			tool: toolServiceStart,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.StartServiceResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "starts without waiting by default",
			tool:       toolServiceStart,
			args:       args,
			setupMock:  expectStart(api.DeployStatusQUEUED),
			wantOutput: accepted,
		},
		{
			name: "wait polls until the service is ready",
			tool: toolServiceStart,
			args: waitArgs,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStart(api.DeployStatusRESUMING)(m)
				expectPoll(api.DeployStatusREADY)(m)
			},
			synctest:   true,
			wantOutput: started,
		},
		{
			// Already at the target status, so the wait returns without polling.
			name:       "wait returns immediately when the service is already ready",
			tool:       toolServiceStart,
			args:       waitArgs,
			setupMock:  expectStart(api.DeployStatusREADY),
			wantOutput: started,
		},
		{
			name: "wait reports a failed poll in the message",
			tool: toolServiceStart,
			args: waitArgs,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStart(api.DeployStatusRESUMING)(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusNotFound)}, nil)
			},
			synctest: true,
			wantOutput: map[string]any{
				"status":  "RESUMING",
				"message": "Error: service not found",
			},
		},
		{
			// The full 10-minute timeout elapses instantly in the bubble.
			name: "wait reports a timeout in the message",
			tool: toolServiceStart,
			args: waitArgs,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStart(api.DeployStatusRESUMING)(m)
				expectPollUntilTimeout(api.DeployStatusRESUMING)(m)
			},
			synctest: true,
			wantOutput: map[string]any{
				"status":  "RESUMING",
				"message": "Error: wait timeout reached after 10m0s - service may still be starting",
			},
		},
	})
}
