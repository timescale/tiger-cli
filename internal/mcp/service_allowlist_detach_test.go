package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceAllowListDetachTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "allow_list_id": "1234567890"}

	// The tool is experimental-gated, so every case registers it explicitly.
	experimental := withExperimental()

	expectDetach := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().DetachServiceFromAllowListWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceAllowListInput{AllowListID: "1234567890"}).
			Return(&api.DetachServiceFromAllowListResponse{
				HTTPResponse: httpResponse(http.StatusAccepted),
				JSON202:      &api.Service{},
			}, nil)
	}
	expectTaggedServiceAndDetach := func(tag string) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectTaggedService(tag, 1)(m)
			expectDetach(m)
		}
	}

	detached := map[string]any{
		"detached": true,
		"message":  "Service 'e6ue9697jf' detached from IP allow list '1234567890'.",
	}
	cancelled := map[string]any{
		"detached": false,
		"message":  `Detach cancelled: the user did not confirm detaching PROD service "e6ue9697jf" from IP allow list "1234567890" by typing its ID.`,
	}
	const noElicitationMsg = "detaching service e6ue9697jf from IP allow list 1234567890 requires the user's confirmation because it is tagged PROD, " +
		"but this MCP client does not support elicitation; ask the user to run 'tiger service allowlist detach e6ue9697jf --allow-list 1234567890' instead"

	// The prompt raised for a PROD service, as the client receives it after
	// the JSON round trip (which is where the SDK fills in the mode).
	prompt := &mcp.ElicitParams{
		Mode:    "form",
		Message: `Remove the IP restriction from PRODUCTION service "test-service" (e6ue9697jf) by detaching it from IP allow list "1234567890"? The service will become reachable from any IP.`,
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
			tool:    toolServiceAllowListDetach,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceAllowListDetach,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name: "service lookup fails",
			tool: toolServiceAllowListDetach,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			// Only the tag lookup is registered: an attempted detach fails as
			// an unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceAllowListDetach,
			args:    args,
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			tool:       toolServiceAllowListDetach,
			args:       args,
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectTaggedServiceAndDetach("DEV"),
			wantOutput: detached,
		},
		{
			// The client here can't prompt, which proves DEV detaches never try.
			name:       "detaches DEV service without prompting",
			tool:       toolServiceAllowListDetach,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectTaggedServiceAndDetach("DEV"),
			wantOutput: detached,
		},
		{
			// The handler runs twice — once to raise the prompt, once with the
			// answer — and fetches the service on both runs.
			name: "detaches PROD service once the user confirms",
			tool: toolServiceAllowListDetach,
			args: args,
			opts: []runOption{experimental, confirm("e6ue9697jf")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("PROD", 2)(m)
				expectDetach(m)
			},
			wantPrompt: prompt,
			wantOutput: detached,
		},
		{
			name:       "PROD detach cancelled when the typed ID does not match",
			tool:       toolServiceAllowListDetach,
			args:       args,
			opts:       []runOption{experimental, confirm("u8me885b93")},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD detach cancelled when the user declines",
			tool:       toolServiceAllowListDetach,
			args:       args,
			opts:       []runOption{experimental, withElicitation(&mcp.ElicitResult{Action: "decline"})},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD detach cancelled when the user dismisses the prompt",
			tool:       toolServiceAllowListDetach,
			args:       args,
			opts:       []runOption{experimental, withElicitation(&mcp.ElicitResult{Action: "cancel"})},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:    "PROD detach refused when the client lacks elicitation",
			tool:    toolServiceAllowListDetach,
			args:    args,
			opts:    []runOption{experimental},
			mock:    expectTaggedService("PROD", 1),
			wantErr: noElicitationMsg,
		},
		{
			name: "PROD detach refused when the client supports only url elicitation",
			tool: toolServiceAllowListDetach,
			args: args,
			opts: []runOption{experimental, withClientCapabilities(&mcp.ClientCapabilities{
				Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}},
			})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: noElicitationMsg,
		},
		{
			name: "network error",
			tool: toolServiceAllowListDetach,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				m.EXPECT().DetachServiceFromAllowListWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceAllowListInput{AllowListID: "1234567890"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to detach service from IP allow list: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceAllowListDetach,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				m.EXPECT().DetachServiceFromAllowListWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceAllowListInput{AllowListID: "1234567890"}).
					Return(&api.DetachServiceFromAllowListResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
		},
	})
}
