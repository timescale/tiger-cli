package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchDeleteTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"branch_id": "br4nch1d9x"}
	expectDelete := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveBranch(m, sampleBranch())
		m.EXPECT().DeleteBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x").
			Return(&api.DeleteBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
	}
	deleted := map[string]any{
		"branch_id": "br4nch1d9x",
		"message":   `Branch "br4nch1d9x" has been deleted.`,
	}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchDelete,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_delete"`,
		},
		{
			name:    "not logged in",
			tool:    toolBranchDelete,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "read-only all refuses without an API call",
			tool:    toolBranchDelete,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// A branch always counts as DEV, so prod neither refuses nor
			// prompts.
			name:       "read-only prod deletes without a prompt",
			tool:       toolBranchDelete,
			args:       args,
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectDelete,
			wantOutput: deleted,
		},
		{
			name: "network error",
			tool: toolBranchDelete,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().DeleteBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to delete branch: connection refused",
		},
		{
			name: "API error",
			tool: toolBranchDelete,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().DeleteBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x").
					Return(&api.DeleteBranchResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("branch not found")},
					}, nil)
			},
			wantErr: "branch not found",
		},
		{
			name:       "success",
			tool:       toolBranchDelete,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectDelete,
			wantOutput: deleted,
		},
	})
}
