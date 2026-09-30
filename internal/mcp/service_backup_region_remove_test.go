package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceBackupRegionRemoveTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "region_code": "eu-central-1"}

	// The tool is experimental-gated, so every case registers it explicitly.
	experimental := withExperimental()

	expectRemove := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().DeleteBackupRegionWithResponse(validCtx, testProjectID, "e6ue9697jf", "eu-central-1").
			Return(&api.DeleteBackupRegionResponse{HTTPResponse: httpResponse(http.StatusNoContent)}, nil)
	}
	expectTaggedServiceAndRemove := func(tag string) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectTaggedService(tag, 1)(m)
			expectRemove(m)
		}
	}

	removed := map[string]any{
		"removed": true,
		"message": "Backups for service 'e6ue9697jf' will no longer be copied to 'eu-central-1'.",
	}
	cancelled := map[string]any{
		"removed": false,
		"message": `Removal cancelled: the user did not confirm removing backup region "eu-central-1" from PROD service "e6ue9697jf" by typing its ID.`,
	}
	const noElicitationMsg = "removing backup region eu-central-1 from service e6ue9697jf requires the user's confirmation because it is tagged PROD, " +
		"but this MCP client does not support elicitation; run 'tiger service backup region remove e6ue9697jf --region eu-central-1' from the CLI instead"

	// The prompt raised for a PROD service, as the client receives it after
	// the JSON round trip (which is where the SDK fills in the mode).
	prompt := &mcp.ElicitParams{
		Mode:    "form",
		Message: `Stop copying PRODUCTION service "test-service" (e6ue9697jf) backups to "eu-central-1"? Backup copies in that region will be deleted.`,
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
			tool:    toolServiceBackupRegionRemove,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceBackupRegionRemove,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name: "service lookup fails",
			tool: toolServiceBackupRegionRemove,
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
			// Only the tag lookup is registered: an attempted removal fails as
			// an unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceBackupRegionRemove,
			args:    args,
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			tool:       toolServiceBackupRegionRemove,
			args:       args,
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectTaggedServiceAndRemove("DEV"),
			wantOutput: removed,
		},
		{
			// The client here can't prompt, which proves DEV removals never try.
			name:       "removes DEV service region without prompting",
			tool:       toolServiceBackupRegionRemove,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectTaggedServiceAndRemove("DEV"),
			wantOutput: removed,
		},
		{
			// The handler runs twice — once to raise the prompt, once with the
			// answer — and fetches the service on both runs.
			name: "removes PROD service region once the user confirms",
			tool: toolServiceBackupRegionRemove,
			args: args,
			opts: []runOption{experimental, confirm("e6ue9697jf")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("PROD", 2)(m)
				expectRemove(m)
			},
			wantPrompt: prompt,
			wantOutput: removed,
		},
		{
			name:       "PROD removal cancelled when the typed ID does not match",
			tool:       toolServiceBackupRegionRemove,
			args:       args,
			opts:       []runOption{experimental, confirm("u8me885b93")},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD removal cancelled when the user declines",
			tool:       toolServiceBackupRegionRemove,
			args:       args,
			opts:       []runOption{experimental, withElicitation(&mcp.ElicitResult{Action: "decline"})},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD removal cancelled when the user dismisses the prompt",
			tool:       toolServiceBackupRegionRemove,
			args:       args,
			opts:       []runOption{experimental, withElicitation(&mcp.ElicitResult{Action: "cancel"})},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:    "PROD removal refused when the client lacks elicitation",
			tool:    toolServiceBackupRegionRemove,
			args:    args,
			opts:    []runOption{experimental},
			mock:    expectTaggedService("PROD", 1),
			wantErr: noElicitationMsg,
		},
		{
			name: "PROD removal refused when the client supports only url elicitation",
			tool: toolServiceBackupRegionRemove,
			args: args,
			opts: []runOption{experimental, withClientCapabilities(&mcp.ClientCapabilities{
				Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}},
			})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: noElicitationMsg,
		},
		{
			name: "network error",
			tool: toolServiceBackupRegionRemove,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				m.EXPECT().DeleteBackupRegionWithResponse(validCtx, testProjectID, "e6ue9697jf", "eu-central-1").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to remove backup region: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceBackupRegionRemove,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				m.EXPECT().DeleteBackupRegionWithResponse(validCtx, testProjectID, "e6ue9697jf", "eu-central-1").
					Return(&api.DeleteBackupRegionResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
		},
	})
}
