package mcp

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestAllowListListTool(t *testing.T) {
	experimental := withExperimental()

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolAllowListList,
			wantCallErr: `calling "tools/call": unknown tool "allowlist_list"`,
		},
		{
			name:    "not logged in",
			tool:    toolAllowListList,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name: "network error",
			tool: toolAllowListList,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list IP allow lists: connection refused",
		},
		{
			name: "API error",
			tool: toolAllowListList,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSON4XX:      &api.ClientError{Message: new("forbidden")},
					}, nil)
			},
			wantErr: "forbidden",
		},
		{
			name: "nil response body returns empty list",
			tool: toolAllowListList,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantOutput: map[string]any{"allow_lists": []any{}},
		},
		{
			name: "success",
			tool: toolAllowListList,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				allowLists := []api.AllowList{
					{
						AllowListID: "1111111111",
						ProjectID:   testProjectID,
						Description: "Office network",
						CidrBlocks:  []string{"203.0.113.0/24"},
						CreatedAt:   time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
					},
					{
						AllowListID: "2222222222",
						ProjectID:   testProjectID,
						Description: "VPN ranges",
						CidrBlocks:  []string{"198.51.100.0/24"},
						CreatedAt:   time.Date(2025, 6, 2, 12, 0, 0, 0, time.UTC),
					},
				}
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowLists,
					}, nil)
			},
			wantOutput: map[string]any{
				"allow_lists": []any{
					map[string]any{
						"allow_list_id": "1111111111",
						"project_id":    testProjectID,
						"description":   "Office network",
						"cidr_blocks":   []any{"203.0.113.0/24"},
						"created_at":    "2025-06-01T12:00:00Z",
					},
					map[string]any{
						"allow_list_id": "2222222222",
						"project_id":    testProjectID,
						"description":   "VPN ranges",
						"cidr_blocks":   []any{"198.51.100.0/24"},
						"created_at":    "2025-06-02T12:00:00Z",
					},
				},
			},
		},
	})
}
