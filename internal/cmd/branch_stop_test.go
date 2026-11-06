package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchStopCmd(t *testing.T) {
	setupStop := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveBranch(m, "br-12345", sampleBranch())
			branch := sampleBranch(branchStatus(status))
			m.EXPECT().StopBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345").
				Return(&api.StopBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}
	noWaitStderr := "Stop request accepted for branch 'test-branch' (br-12345).\n" +
		"Use 'tiger branch get' to check branch status.\n"

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"branch", "stop", "br-12345"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			// No default fallback: service_id only ever holds a service ID.
			name:    "missing branch id",
			args:    []string{"branch", "stop"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: "branch name or ID is required",
		},
		{
			name:    "read-only all refuses",
			args:    []string{"branch", "stop", "br-12345"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			args:       []string{"branch", "stop", "br-12345", "--no-wait"},
			opts:       []runOption{withExperimental(), withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupStop(api.DeployStatusPAUSING),
			wantStderr: noWaitStderr,
		},
		{
			name: "network error",
			args: []string{"branch", "stop", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().StopBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to stop branch: connection refused",
		},
		{
			name: "API error",
			args: []string{"branch", "stop", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().StopBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345").
					Return(&api.StopBranchResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.ClientError{Message: new("branch is already stopped")},
					}, nil)
			},
			wantErr: "branch is already stopped",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "nil response body",
			args: []string{"branch", "stop", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().StopBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345").
					Return(&api.StopBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "no-wait",
			args:       []string{"branch", "stop", "br-12345", "--no-wait"},
			opts:       []runOption{withExperimental()},
			mock:       setupStop(api.DeployStatusPAUSING),
			wantStderr: noWaitStderr,
		},
		{
			name:     "wait polls until paused",
			synctest: true,
			args:     []string{"branch", "stop", "br-12345"},
			opts:     []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupStop(api.DeployStatusPAUSING)(m)
				expectGetBranch(m, sampleBranch(branchStatus(api.DeployStatusPAUSED)))
			},
			wantStderr: "Stop request accepted for branch 'test-branch' (br-12345).\n" +
				"Waiting for branch to stop (timeout: 10m0s)...\n" +
				"⢎  Branch status: PAUSING\n" +
				"Branch stopped.\n",
		},
		{
			name:     "wait timeout",
			synctest: true,
			args:     []string{"branch", "stop", "br-12345"},
			opts:     []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupStop(api.DeployStatusPAUSING)(m)
				pausing := sampleBranch(branchStatus(api.DeployStatusPAUSING))
				m.EXPECT().GetBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345").
					Return(&api.GetBranchResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &pausing,
					}, nil).AnyTimes()
			},
			wantErr: "wait timeout reached after 10m0s - branch may still be stopping",
			wantStderr: "Stop request accepted for branch 'test-branch' (br-12345).\n" +
				"Waiting for branch to stop (timeout: 10m0s)...\n" +
				"⢎  Branch status: PAUSING\n" +
				"Error: wait timeout reached after 10m0s - branch may still be stopping\n",
			checks: []checkFunc{checkExitCode(common.ExitTimeout)},
		},
		{
			name:       "pause alias",
			args:       []string{"branch", "pause", "br-12345", "--no-wait"},
			opts:       []runOption{withExperimental()},
			mock:       setupStop(api.DeployStatusPAUSING),
			wantStderr: noWaitStderr,
		},
	})
}
