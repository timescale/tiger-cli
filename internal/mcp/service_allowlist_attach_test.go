package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceAllowListAttachTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "allow_list_id": "1234567890"}

	// The tool is experimental-gated (see the first case), so every other
	// case registers it explicitly.
	experimental := withExperimental()

	attached := api.Service{}
	wantOutput := map[string]any{
		"message": "Service 'e6ue9697jf' attached to IP allow list '1234567890'.",
	}
	expectAttach := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().AttachServiceToAllowListWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceAllowListInput{AllowListID: "1234567890"}).
			Return(&api.AttachServiceToAllowListResponse{
				HTTPResponse: httpResponse(http.StatusAccepted),
				JSON202:      &attached,
			}, nil)
	}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolServiceAllowListAttach,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "service_allowlist_attach"`,
		},
		{
			name:    "not logged in",
			tool:    toolServiceAllowListAttach,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceAllowListAttach,
			args:    map[string]any{"service_id": "NOPE", "allow_list_id": "1234567890"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "NOPE" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "missing allow list id",
			tool:    toolServiceAllowListAttach,
			args:    map[string]any{"service_id": "e6ue9697jf"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: required: missing properties: ["allow_list_id"]`,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceAllowListAttach,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// Only the tag lookup is registered: an attempted attach fails as an
			// unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceAllowListAttach,
			args:    args,
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name: "read-only prod allows DEV service",
			tool: toolServiceAllowListAttach,
			args: args,
			opts: []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				expectAttach(m)
			},
			wantOutput: wantOutput,
		},
		{
			name: "network error",
			tool: toolServiceAllowListAttach,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().AttachServiceToAllowListWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceAllowListInput{AllowListID: "1234567890"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to attach service to IP allow list: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceAllowListAttach,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().AttachServiceToAllowListWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceAllowListInput{AllowListID: "1234567890"}).
					Return(&api.AttachServiceToAllowListResponse{
						HTTPResponse: httpResponse(http.StatusConflict),
						JSON4XX:      &api.ClientError{Message: new("service already attached to an IP allow list")},
					}, nil)
			},
			wantErr: "service already attached to an IP allow list",
		},
		{
			name: "nil response body",
			tool: toolServiceAllowListAttach,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().AttachServiceToAllowListWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceAllowListInput{AllowListID: "1234567890"}).
					Return(&api.AttachServiceToAllowListResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "service attached",
			tool:       toolServiceAllowListAttach,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectAttach,
			wantOutput: wantOutput,
		},
	})
}
