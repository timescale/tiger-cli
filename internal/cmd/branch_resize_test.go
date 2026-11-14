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
	// setupResize answers the resize with the branch at the new size,
	// including the IOPS the API applies.
	setupResize := func(req api.BranchResizeInput, cpuMillis, memoryGBs, iops int) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveBranch(m, "br-12345", sampleBranch())
			branch := sampleBranch(branchStatus(api.DeployStatusCONFIGURING), func(b *api.Branch) {
				b.CPUMillis = cpuMillis
				b.MemoryGbs = memoryGBs
				b.Iops = new(iops)
			})
			m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", req).
				Return(&api.ResizeBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}
	req2000 := api.BranchResizeInput{CPUMillis: new("2000"), MemoryGbs: new("16")}
	acceptedStderr := "Resizing branch 'test-branch' (br-12345) to 2 CPU/16 GB, 6000 IOPS.\n" +
		"The resize is in progress and may take several minutes to complete.\n"

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"branch", "resize", "br-12345", "--cpu", "2"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			// No default fallback: service_id only ever holds a service ID.
			name:    "missing branch id",
			args:    []string{"branch", "resize", "--cpu", "2"},
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
			args:    []string{"branch", "resize", "br-12345", "--cpu", "2", "--memory", "8"},
			opts:    []runOption{withExperimental()},
			wantErr: "invalid CPU/Memory combination. Allowed combinations: 0.25 CPU/2 GB, 0.5 CPU/4 GB, 1 CPU/8 GB, 2 CPU/16 GB, 4 CPU/32 GB, 8 CPU/64 GB, 16 CPU/128 GB, 32 CPU/256 GB",
		},
		{
			name:    "read-only all refuses",
			args:    []string{"branch", "resize", "br-12345", "--cpu", "2"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			args:       []string{"branch", "resize", "br-12345", "--cpu", "2"},
			opts:       []runOption{withExperimental(), withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupResize(req2000, 2000, 16, 6000),
			wantStderr: acceptedStderr,
		},
		{
			name: "network error",
			args: []string{"branch", "resize", "br-12345", "--cpu", "2"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", req2000).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to resize branch: connection refused",
		},
		{
			name: "nil response body",
			args: []string{"branch", "resize", "br-12345", "--cpu", "2"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", req2000).
					Return(&api.ResizeBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// CPU and memory are left out, so the API keeps the current ones.
			name: "iops only omits cpu and memory",
			args: []string{"branch", "resize", "br-12345", "--iops", "16000"},
			opts: []runOption{withExperimental()},
			mock: setupResize(api.BranchResizeInput{Iops: new(16000)}, 250, 2, 16000),
			wantStderr: "Resizing branch 'test-branch' (br-12345) to 0.25 CPU/2 GB, 16000 IOPS.\n" +
				"The resize is in progress and may take several minutes to complete.\n",
		},
		{
			name: "cpu and iops",
			args: []string{"branch", "resize", "br-12345", "--cpu", "2", "--iops", "16000"},
			opts: []runOption{withExperimental()},
			mock: setupResize(api.BranchResizeInput{CPUMillis: new("2000"), MemoryGbs: new("16"), Iops: new(16000)}, 2000, 16, 16000),
			wantStderr: "Resizing branch 'test-branch' (br-12345) to 2 CPU/16 GB, 16000 IOPS.\n" +
				"The resize is in progress and may take several minutes to complete.\n",
		},
		{
			name: "response without iops omits them",
			args: []string{"branch", "resize", "br-12345", "--cpu", "2"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", req2000).
					Return(&api.ResizeBranchResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202: new(sampleBranch(func(b *api.Branch) {
							b.CPUMillis = 2000
							b.MemoryGbs = 16
						})),
					}, nil)
			},
			wantStderr: "Resizing branch 'test-branch' (br-12345) to 2 CPU/16 GB.\n" +
				"The resize is in progress and may take several minutes to complete.\n",
		},
		{
			// The IOPS weren't requested, so they come from the response.
			name:       "memory only auto-configures cpu",
			args:       []string{"branch", "resize", "br-12345", "--memory", "16"},
			opts:       []runOption{withExperimental()},
			mock:       setupResize(req2000, 2000, 16, 6000),
			wantStderr: acceptedStderr,
		},
		{
			name: "fractional cores",
			args: []string{"branch", "resize", "br-12345", "--cpu", "0.25"},
			opts: []runOption{withExperimental()},
			mock: setupResize(api.BranchResizeInput{CPUMillis: new("250"), MemoryGbs: new("2")}, 250, 2, 3000),
			wantStderr: "Resizing branch 'test-branch' (br-12345) to 0.25 CPU/2 GB, 3000 IOPS.\n" +
				"The resize is in progress and may take several minutes to complete.\n",
		},
		{
			// Millicores are deprecated but still accepted for backwards
			// compatibility.
			name:       "deprecated millicores",
			args:       []string{"branch", "resize", "br-12345", "--cpu", "2000"},
			opts:       []runOption{withExperimental()},
			mock:       setupResize(req2000, 2000, 16, 6000),
			wantStderr: acceptedStderr,
		},
	})
}
