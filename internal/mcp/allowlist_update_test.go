package mcp

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestAllowListUpdateTool(t *testing.T) {
	experimental := withExperimental()

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolAllowListUpdate,
			args:        map[string]any{"allow_list_id": "1234567890", "description": "New name"},
			wantCallErr: `calling "tools/call": unknown tool "allowlist_update"`,
		},
		{
			name:    "not logged in",
			tool:    toolAllowListUpdate,
			args:    map[string]any{"allow_list_id": "1234567890", "description": "New name"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolAllowListUpdate,
			args:    map[string]any{"allow_list_id": "1234567890", "description": "New name"},
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "neither field set",
			tool:    toolAllowListUpdate,
			args:    map[string]any{"allow_list_id": "1234567890"},
			opts:    []runOption{experimental},
			wantErr: "at least one of description or cidr_blocks is required",
		},
		{
			name: "network error",
			tool: toolAllowListUpdate,
			args: map[string]any{"allow_list_id": "1234567890", "description": "New name"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					Description: new("New name"),
				}).Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to update IP allow list: connection refused",
		},
		{
			name: "API error",
			tool: toolAllowListUpdate,
			args: map[string]any{"allow_list_id": "1234567890", "cidr_blocks": []any{"203.0.113.0/24"}},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					CidrBlocks: &[]string{"203.0.113.0/24"},
				}).Return(&api.UpdateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusNotFound),
					JSON4XX:      &api.ClientError{Message: new("IP allow list not found")},
				}, nil)
			},
			wantErr: "IP allow list not found",
		},
		{
			name: "nil response body",
			tool: toolAllowListUpdate,
			args: map[string]any{"allow_list_id": "1234567890", "description": "New name"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					Description: new("New name"),
				}).Return(&api.UpdateAllowListResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "success, both fields",
			tool: toolAllowListUpdate,
			args: map[string]any{
				"allow_list_id": "1234567890",
				"description":   "New name",
				"cidr_blocks":   []any{"203.0.113.0/24", "198.51.100.0/24"},
			},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				allowList := api.AllowList{
					AllowListID: "1234567890",
					ProjectID:   testProjectID,
					Description: "New name",
					CidrBlocks:  []string{"203.0.113.0/24", "198.51.100.0/24"},
					CreatedAt:   time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
				}
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					Description: new("New name"),
					CidrBlocks:  &[]string{"203.0.113.0/24", "198.51.100.0/24"},
				}).Return(&api.UpdateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &allowList,
				}, nil)
			},
			wantOutput: map[string]any{
				"allow_list_id": "1234567890",
				"project_id":    testProjectID,
				"description":   "New name",
				"cidr_blocks":   []any{"203.0.113.0/24", "198.51.100.0/24"},
				"created_at":    "2025-06-01T12:00:00Z",
			},
		},
	})
}
