package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchStartCmd(t *testing.T) {
	setupStart := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetBranch(m, sampleBranch(branchStatus(api.DeployStatusPAUSED)))
			branch := sampleBranch(branchStatus(status))
			m.EXPECT().StartBranchWithResponse(validCtx, testProjectID, "br-12345").
				Return(&api.StartBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}
	noWaitStderr := "Start request accepted for branch 'test-branch' (br-12345).\n" +
		"Use 'tiger branch get' to check branch status.\n"

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"branch", "start", "br-12345"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			// No default fallback: service_id only ever holds a service ID.
			name:    "missing branch id",
			args:    []string{"branch", "start"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: "branch ID is required",
		},
		{
			name:    "read-only all refuses",
			args:    []string{"branch", "start", "br-12345"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			args:       []string{"branch", "start", "br-12345", "--no-wait"},
			opts:       []runOption{withExperimental(), withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupStart(api.DeployStatusRESUMING),
			wantStderr: noWaitStderr,
		},
		{
			name: "network error",
			args: []string{"branch", "start", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetBranch(m, sampleBranch())
				m.EXPECT().StartBranchWithResponse(validCtx, testProjectID, "br-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to start branch: connection refused",
		},
		{
			name: "nil response body",
			args: []string{"branch", "start", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetBranch(m, sampleBranch())
				m.EXPECT().StartBranchWithResponse(validCtx, testProjectID, "br-12345").
					Return(&api.StartBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "no-wait",
			args:       []string{"branch", "start", "br-12345", "--no-wait"},
			opts:       []runOption{withExperimental()},
			mock:       setupStart(api.DeployStatusRESUMING),
			wantStderr: noWaitStderr,
		},
		{
			name:     "wait polls until ready",
			synctest: true,
			args:     []string{"branch", "start", "br-12345"},
			opts:     []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupStart(api.DeployStatusRESUMING)(m)
				expectGetBranch(m, sampleBranch())
			},
			wantStderr: "Start request accepted for branch 'test-branch' (br-12345).\n" +
				"Waiting for branch to start (timeout: 10m0s)...\n" +
				"⢎  Branch status: RESUMING\n" +
				"Branch started.\n",
		},
		{
			name:       "resume alias",
			args:       []string{"branch", "resume", "br-12345", "--no-wait"},
			opts:       []runOption{withExperimental()},
			mock:       setupStart(api.DeployStatusRESUMING),
			wantStderr: noWaitStderr,
		},
	})
}
