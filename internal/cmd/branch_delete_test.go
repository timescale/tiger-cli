package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchDeleteCmd(t *testing.T) {
	setupDelete := func(m *mocks.MockClientWithResponsesInterface) {
		expectGetBranch(m, sampleBranch())
		m.EXPECT().DeleteBranchWithResponse(validCtx, testProjectID, "br-12345").
			Return(&api.DeleteBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
	}
	getOnly := func(m *mocks.MockClientWithResponsesInterface) {
		expectGetBranch(m, sampleBranch())
	}
	prompt := "Are you sure you want to delete branch 'test-branch' (br-12345)? This operation cannot be undone.\n" +
		"Type the branch ID 'br-12345' to confirm: "

	runCmdTests(t, []cmdTest{
		{
			// No default fallback, even with one configured.
			name:    "missing branch id",
			args:    []string{"branch", "delete"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"service_id": "br-12345"})},
			wantErr: "branch ID is required",
		},
		{
			name:    "not logged in",
			args:    []string{"branch", "delete", "br-12345", "--confirm"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only all refuses",
			args:    []string{"branch", "delete", "br-12345", "--confirm"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// A branch always counts as DEV.
			name:       "read-only prod allows",
			args:       []string{"branch", "delete", "br-12345", "--confirm"},
			opts:       []runOption{withExperimental(), withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupDelete,
			wantStderr: "Branch 'test-branch' (br-12345) deleted.\n",
		},
		{
			name: "branch not found",
			args: []string{"branch", "delete", "br-12345", "--confirm"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetBranchStatus(m, "br-12345", http.StatusNotFound, &api.Error{Message: new("branch not found")})
			},
			wantErr: "branch 'br-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name:    "non-TTY without confirm",
			args:    []string{"branch", "delete", "br-12345"},
			opts:    []runOption{withExperimental()},
			mock:    getOnly,
			wantErr: "TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt",
		},
		{
			name:       "confirmation mismatch",
			args:       []string{"branch", "delete", "br-12345"},
			opts:       []runOption{withExperimental(), withIsTerminal(true), withStdin("test-branch\n")},
			mock:       getOnly,
			wantStderr: prompt + "Delete operation cancelled.\n",
		},
		{
			name:       "confirmation match",
			args:       []string{"branch", "delete", "br-12345"},
			opts:       []runOption{withExperimental(), withIsTerminal(true), withStdin("br-12345\n")},
			mock:       setupDelete,
			wantStderr: prompt + "Branch 'test-branch' (br-12345) deleted.\n",
		},
		{
			name: "network error",
			args: []string{"branch", "delete", "br-12345", "--confirm"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetBranch(m, sampleBranch())
				m.EXPECT().DeleteBranchWithResponse(validCtx, testProjectID, "br-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to delete branch: connection refused",
		},
		{
			name: "API error",
			args: []string{"branch", "delete", "br-12345", "--confirm"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetBranch(m, sampleBranch())
				m.EXPECT().DeleteBranchWithResponse(validCtx, testProjectID, "br-12345").
					Return(&api.DeleteBranchResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.Error{Message: new("branch is busy")},
					}, nil)
			},
			wantErr: "branch is busy",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:       "rm alias",
			args:       []string{"branch", "rm", "br-12345", "--confirm"},
			opts:       []runOption{withExperimental()},
			mock:       setupDelete,
			wantStderr: "Branch 'test-branch' (br-12345) deleted.\n",
		},
	})
}
