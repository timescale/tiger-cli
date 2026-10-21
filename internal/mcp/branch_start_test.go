package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchStartTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"branch_id": "br4nch1d9x"}
	expectStart := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			branch := sampleBranch(branchStatus(status))
			expectResolveBranch(m, sampleBranch())
			m.EXPECT().StartBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x").
				Return(&api.StartBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}
	accepted := map[string]any{
		"status":  "RESUMING",
		"message": "Branch start request accepted. The branch may still be starting.",
	}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchStart,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_start"`,
		},
		{
			name:    "read-only all refuses without an API call",
			tool:    toolBranchStart,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			tool:       toolBranchStart,
			args:       args,
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectStart(api.DeployStatusRESUMING),
			wantOutput: accepted,
		},
		{
			name: "network error",
			tool: toolBranchStart,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().StartBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to start branch: connection refused",
		},
		{
			name:       "starts without waiting by default",
			tool:       toolBranchStart,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectStart(api.DeployStatusRESUMING),
			wantOutput: accepted,
		},
		{
			name:     "wait polls until ready",
			synctest: true,
			tool:     toolBranchStart,
			args:     map[string]any{"branch_id": "br4nch1d9x", "wait": true},
			opts:     []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStart(api.DeployStatusRESUMING)(m)
				expectGetBranch(m, sampleBranch())
			},
			wantOutput: map[string]any{"status": "READY", "message": "Branch started."},
		},
	})
}
