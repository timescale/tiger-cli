package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchResizeCmd(t *testing.T) {
	setupResize := func(req api.BranchResizeInput, status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveBranch(m, "br-12345", sampleBranch())
			branch := sampleBranch(branchStatus(status))
			m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", req).
				Return(&api.ResizeBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}
	req2000 := api.BranchResizeInput{CPUMillis: new("2000"), MemoryGbs: new("16")}
	noWaitStderr := "Resizing branch 'test-branch' (br-12345) to 2 CPU/16 GB...\n" +
		"Resize request accepted for branch 'test-branch' (br-12345).\n" +
		"Use 'tiger branch get' to check branch status.\n"

	runCmdTests(t, []cmdTest{
		{
			name:    "zero wait timeout",
			args:    []string{"branch", "resize", "br-12345", "--cpu", "2000", "--wait-timeout", "0s"},
			opts:    []runOption{withExperimental()},
			wantErr: "wait timeout must be positive, got 0s",
		},
		{
			name:    "negative wait timeout",
			args:    []string{"branch", "resize", "br-12345", "--cpu", "2000", "--wait-timeout=-30m"},
			opts:    []runOption{withExperimental()},
			wantErr: "wait timeout must be positive, got -30m0s",
		},
		{
			name:    "not logged in",
			args:    []string{"branch", "resize", "br-12345", "--cpu", "2000"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			// No default fallback: service_id only ever holds a service ID.
			name:    "missing branch id",
			args:    []string{"branch", "resize", "--cpu", "2000"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: "branch name or ID is required",
		},
		{
			name:    "missing cpu and memory",
			args:    []string{"branch", "resize", "br-12345"},
			opts:    []runOption{withExperimental()},
			wantErr: "must specify at least one of --cpu, --memory, or --iops",
		},
		{
			name:    "invalid cpu/memory combination",
			args:    []string{"branch", "resize", "br-12345", "--cpu", "2000", "--memory", "8"},
			opts:    []runOption{withExperimental()},
			wantErr: "invalid CPU/Memory combination. Allowed combinations: 0.25 CPU/2 GB, 0.5 CPU/4 GB, 1 CPU/8 GB, 2 CPU/16 GB, 4 CPU/32 GB, 8 CPU/64 GB, 16 CPU/128 GB, 32 CPU/256 GB",
		},
		{
			name:    "read-only all refuses",
			args:    []string{"branch", "resize", "br-12345", "--cpu", "2000"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			args:       []string{"branch", "resize", "br-12345", "--cpu", "2000", "--no-wait"},
			opts:       []runOption{withExperimental(), withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupResize(req2000, api.DeployStatusCONFIGURING),
			wantStderr: noWaitStderr,
		},
		{
			name: "network error",
			args: []string{"branch", "resize", "br-12345", "--cpu", "2000"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", req2000).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to resize branch: connection refused",
			wantStderr: "Resizing branch 'test-branch' (br-12345) to 2 CPU/16 GB...\n" +
				"Error: failed to resize branch: connection refused\n",
		},
		{
			name: "nil response body",
			args: []string{"branch", "resize", "br-12345", "--cpu", "2000"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", req2000).
					Return(&api.ResizeBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
			wantStderr: "Resizing branch 'test-branch' (br-12345) to 2 CPU/16 GB...\n" +
				"Error: empty response from API\n",
		},
		{
			// CPU and memory are left out, so the API keeps the current ones.
			name: "iops only omits cpu and memory",
			args: []string{"branch", "resize", "br-12345", "--iops", "16000", "--no-wait"},
			opts: []runOption{withExperimental()},
			mock: setupResize(api.BranchResizeInput{Iops: new(16000)}, api.DeployStatusCONFIGURING),
			wantStderr: "Resizing branch 'test-branch' (br-12345) to 16000 IOPS...\n" +
				"Resize request accepted for branch 'test-branch' (br-12345).\n" +
				"Use 'tiger branch get' to check branch status.\n",
		},
		{
			name: "cpu and iops",
			args: []string{"branch", "resize", "br-12345", "--cpu", "2000", "--iops", "16000", "--no-wait"},
			opts: []runOption{withExperimental()},
			mock: setupResize(api.BranchResizeInput{CPUMillis: new("2000"), MemoryGbs: new("16"), Iops: new(16000)}, api.DeployStatusCONFIGURING),
			wantStderr: "Resizing branch 'test-branch' (br-12345) to 2 CPU/16 GB, 16000 IOPS...\n" +
				"Resize request accepted for branch 'test-branch' (br-12345).\n" +
				"Use 'tiger branch get' to check branch status.\n",
		},
		{
			name:       "no-wait with cpu only",
			args:       []string{"branch", "resize", "br-12345", "--cpu", "2000", "--no-wait"},
			opts:       []runOption{withExperimental()},
			mock:       setupResize(req2000, api.DeployStatusCONFIGURING),
			wantStderr: noWaitStderr,
		},
		{
			name:     "wait polls until ready",
			synctest: true,
			args:     []string{"branch", "resize", "br-12345", "--memory", "16"},
			opts:     []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupResize(req2000, api.DeployStatusCONFIGURING)(m)
				expectGetBranch(m, sampleBranch())
			},
			wantStderr: "Resizing branch 'test-branch' (br-12345) to 2 CPU/16 GB...\n" +
				"Resize request accepted for branch 'test-branch' (br-12345).\n" +
				"Waiting for resize to complete (timeout: 10m0s)...\n" +
				"⢎  Branch status: CONFIGURING\n" +
				"Branch resized to 2 CPU/16 GB.\n",
		},
	})
}
