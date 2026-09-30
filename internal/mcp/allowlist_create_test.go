package mcp

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestAllowListCreateTool(t *testing.T) {
	args := map[string]any{
		"description": "Office network",
		"cidr_blocks": []any{"203.0.113.0/24"},
	}

	experimental := withExperimental()

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolAllowListCreate,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "allowlist_create"`,
		},
		{
			name:    "not logged in",
			tool:    toolAllowListCreate,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolAllowListCreate,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name: "network error",
			tool: toolAllowListCreate,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateAllowListWithResponse(validCtx, testProjectID, api.AllowListCreate{
					Description: "Office network",
					CidrBlocks:  []string{"203.0.113.0/24"},
				}).Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create IP allow list: connection refused",
		},
		{
			name: "API error",
			tool: toolAllowListCreate,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateAllowListWithResponse(validCtx, testProjectID, api.AllowListCreate{
					Description: "Office network",
					CidrBlocks:  []string{"203.0.113.0/24"},
				}).Return(&api.CreateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusBadRequest),
					JSON4XX:      &api.ClientError{Message: new("cidr_blocks: too many blocks for your plan")},
				}, nil)
			},
			wantErr: "cidr_blocks: too many blocks for your plan",
		},
		{
			name: "nil response body",
			tool: toolAllowListCreate,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateAllowListWithResponse(validCtx, testProjectID, api.AllowListCreate{
					Description: "Office network",
					CidrBlocks:  []string{"203.0.113.0/24"},
				}).Return(&api.CreateAllowListResponse{HTTPResponse: httpResponse(http.StatusCreated)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "success",
			tool: toolAllowListCreate,
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
				m.EXPECT().CreateAllowListWithResponse(validCtx, testProjectID, api.AllowListCreate{
					Description: "Office network",
					CidrBlocks:  []string{"203.0.113.0/24"},
				}).Return(&api.CreateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusCreated),
					JSON201:      &allowList,
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
