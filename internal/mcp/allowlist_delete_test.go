package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestAllowListDeleteTool(t *testing.T) {
	args := map[string]any{"allow_list_id": "1234567890"}

	experimental := withExperimental()

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolAllowListDelete,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "allowlist_delete"`,
		},
		{
			name:    "not logged in",
			tool:    toolAllowListDelete,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolAllowListDelete,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name: "network error",
			tool: toolAllowListDelete,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().DeleteAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to delete IP allow list: connection refused",
		},
		{
			name: "still attached refused",
			tool: toolAllowListDelete,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().DeleteAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.DeleteAllowListResponse{
						HTTPResponse: httpResponse(http.StatusConflict),
						JSON4XX:      &api.ClientError{Message: new("IP allow list is still attached to a service")},
					}, nil)
			},
			wantErr: "IP allow list is still attached to a service",
		},
		{
			name: "success",
			tool: toolAllowListDelete,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().DeleteAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.DeleteAllowListResponse{HTTPResponse: httpResponse(http.StatusNoContent)}, nil)
			},
			wantOutput: map[string]any{"message": "IP allow list '1234567890' deleted."},
		},
	})
}
