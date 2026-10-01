package mcp

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestAllowListGetTool(t *testing.T) {
	args := map[string]any{"allow_list_id": "1234567890"}

	experimental := withExperimental()

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolAllowListGet,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "allowlist_get"`,
		},
		{
			name:    "not logged in",
			tool:    toolAllowListGet,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name: "network error",
			tool: toolAllowListGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get IP allow list: connection refused",
		},
		{
			name: "not found",
			tool: toolAllowListGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("IP allow list not found")},
					}, nil)
			},
			wantErr: "IP allow list not found",
		},
		{
			name: "nil response body",
			tool: toolAllowListGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "success",
			tool: toolAllowListGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				allowList := api.AllowList{
					AllowListID: "1234567890",
					ProjectID:   testProjectID,
					Description: "Office network",
					CidrBlocks:  []string{"203.0.113.0/24"},
					CreatedAt:   time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
				}
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowList,
					}, nil)
			},
			wantOutput: map[string]any{
				"allow_list_id": "1234567890",
				"project_id":    testProjectID,
				"description":   "Office network",
				"cidr_blocks":   []any{"203.0.113.0/24"},
				"created_at":    "2025-06-01T12:00:00Z",
			},
		},
	})
}
