package mcp

import (
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceDelete(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	expectDelete := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
			Return(&api.DeleteServiceResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
	}
	expectTaggedServiceAndDelete := func(tag string) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectTaggedService(tag, 1)(m)
			expectDelete(m)
		}
	}

	deleted := map[string]any{
		"service_id": "e6ue9697jf",
		"deleted":    true,
		"message":    `Service "e6ue9697jf" has been deleted.`,
	}
	cancelled := map[string]any{
		"service_id": "e6ue9697jf",
		"deleted":    false,
		"message":    `Deletion cancelled: the user did not confirm deleting PROD service "e6ue9697jf" by typing its ID.`,
	}
	const noElicitationMsg = "deleting service e6ue9697jf requires the user's confirmation because it is tagged PROD, " +
		"but this MCP client does not support elicitation; run 'tiger service delete e6ue9697jf' from the CLI instead"

	// The prompt raised for a PROD service, as the client receives it after
	// the JSON round trip (which is where the SDK fills in the mode).
	prompt := &mcp.ElicitParams{
		Mode:    "form",
		Message: `Delete PRODUCTION service "test-service" (e6ue9697jf)? This permanently destroys the service and all of its data, and cannot be undone.`,
		RequestedSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"service_id": map[string]any{
					"type":        "string",
					"title":       "Service ID",
					"description": "Type the service ID e6ue9697jf to confirm",
				},
			},
			"required": []any{"service_id"},
		},
	}
	// confirm answers the prompt by typing id back.
	confirm := func(id string) runOption {
		return withElicitation(&mcp.ElicitResult{Action: "accept", Content: map[string]any{"service_id": id}})
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceDelete,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceDelete,
			args:    args,
			opts:    []runOption{withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name: "service lookup fails",
			tool: toolServiceDelete,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			// Only the tag lookup is registered: an attempted deletion fails as
			// an unexpected call.
			name:      "read-only prod refuses PROD service",
			tool:      toolServiceDelete,
			args:      args,
			opts:      []runOption{withConfig(map[string]any{"read_only": "prod"})},
			setupMock: expectTaggedService("PROD", 1),
			wantErr:   `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			setupMock:  expectTaggedServiceAndDelete("DEV"),
			wantOutput: deleted,
		},
		{
			// The client here can't prompt, which proves DEV deletions never try.
			name:       "deletes DEV service without prompting",
			tool:       toolServiceDelete,
			args:       args,
			setupMock:  expectTaggedServiceAndDelete("DEV"),
			wantOutput: deleted,
		},
		{
			// The handler runs twice — once to raise the prompt, once with the
			// answer — and fetches the service on both runs.
			name: "deletes PROD service once the user confirms",
			tool: toolServiceDelete,
			args: args,
			opts: []runOption{confirm("e6ue9697jf")},
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("PROD", 2)(m)
				expectDelete(m)
			},
			wantPrompt: prompt,
			wantOutput: deleted,
		},
		{
			name:       "PROD deletion cancelled when the typed ID does not match",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{confirm("u8me885b93")},
			setupMock:  expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			// A non-ID answer must reach the handler as a cancellation, not
			// fail schema validation in the SDK, so the field has no pattern.
			name:       "PROD deletion cancelled when the typed ID is not an ID at all",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{confirm("fail")},
			setupMock:  expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD deletion cancelled when the user declines",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{withElicitation(&mcp.ElicitResult{Action: "decline"})},
			setupMock:  expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD deletion cancelled when the user dismisses the prompt",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{withElicitation(&mcp.ElicitResult{Action: "cancel"})},
			setupMock:  expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:      "PROD deletion refused when the client lacks elicitation",
			tool:      toolServiceDelete,
			args:      args,
			setupMock: expectTaggedService("PROD", 1),
			wantErr:   noElicitationMsg,
		},
		{
			name: "PROD deletion refused when the client supports only url elicitation",
			tool: toolServiceDelete,
			args: args,
			opts: []runOption{withClientCapabilities(&mcp.ClientCapabilities{
				Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}},
			})},
			setupMock: expectTaggedService("PROD", 1),
			wantErr:   noElicitationMsg,
		},
		{
			name: "delete API error",
			tool: toolServiceDelete,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.DeleteServiceResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
		},
	})
}
