package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchRenameTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"branch_id": "br4nch1d9x", "name": "feature/rbac"}
	expectRename := func(m *mocks.MockClientWithResponsesInterface) {
		renamed := sampleBranch(func(b *api.Branch) { b.Name = "feature/rbac" })
		expectResolveBranch(m, sampleBranch())
		m.EXPECT().RenameBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", api.BranchRename{Name: "feature/rbac"}).
			Return(&api.RenameBranchResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &renamed,
			}, nil)
	}
	renamed := map[string]any{
		"message":   `Branch "br4nch1d9x" renamed to "feature/rbac".`,
		"branch_id": "br4nch1d9x",
		"name":      "feature/rbac",
	}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchRename,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_rename"`,
		},
		{
			name:    "read-only all refuses without an API call",
			tool:    toolBranchRename,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			tool:       toolBranchRename,
			args:       args,
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectRename,
			wantOutput: renamed,
		},
		{
			name:    "empty name",
			tool:    toolBranchRename,
			args:    map[string]any{"branch_id": "br4nch1d9x", "name": "  "},
			opts:    []runOption{experimental},
			wantErr: "new name cannot be empty",
		},
		{
			name: "network error",
			tool: toolBranchRename,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().RenameBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", api.BranchRename{Name: "feature/rbac"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to rename branch: connection refused",
		},
		{
			name: "nil response body",
			tool: toolBranchRename,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().RenameBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", api.BranchRename{Name: "feature/rbac"}).
					Return(&api.RenameBranchResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "success",
			tool:       toolBranchRename,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectRename,
			wantOutput: renamed,
		},
	})
}
