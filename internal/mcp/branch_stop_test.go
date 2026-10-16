package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchStopTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"branch_id": "br4nch1d9x"}
	expectStop := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			branch := sampleBranch(branchStatus(status))
			m.EXPECT().StopBranchWithResponse(validCtx, testProjectID, "br4nch1d9x").
				Return(&api.StopBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}
	accepted := map[string]any{
		"status":  "PAUSING",
		"message": "Branch stop request accepted. The branch may still be stopping.",
	}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchStop,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_stop"`,
		},
		{
			name:    "not logged in",
			tool:    toolBranchStop,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "read-only all refuses without an API call",
			tool:    toolBranchStop,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			tool:       toolBranchStop,
			args:       args,
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectStop(api.DeployStatusPAUSING),
			wantOutput: accepted,
		},
		{
			name: "network error",
			tool: toolBranchStop,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopBranchWithResponse(validCtx, testProjectID, "br4nch1d9x").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to stop branch: connection refused",
		},
		{
			name: "nil response body",
			tool: toolBranchStop,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().StopBranchWithResponse(validCtx, testProjectID, "br4nch1d9x").
					Return(&api.StopBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "stops without waiting by default",
			tool:       toolBranchStop,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectStop(api.DeployStatusPAUSING),
			wantOutput: accepted,
		},
		{
			name:     "wait polls until paused",
			synctest: true,
			tool:     toolBranchStop,
			args:     map[string]any{"branch_id": "br4nch1d9x", "wait": true},
			opts:     []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectStop(api.DeployStatusPAUSING)(m)
				expectGetBranch(m, sampleBranch(branchStatus(api.DeployStatusPAUSED)))
			},
			wantOutput: map[string]any{"status": "PAUSED", "message": "Branch stopped."},
		},
	})
}
