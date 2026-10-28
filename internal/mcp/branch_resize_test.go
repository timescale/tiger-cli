package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchResizeTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"branch_id": "br4nch1d9x", "cpu_memory": "2 CPU/16 GB"}
	req := api.BranchResizeInput{CPUMillis: new("2000"), MemoryGbs: new("16")}
	// The resized branch, as the API reports it once the new spec is applied.
	resized := func(status api.DeployStatus) api.Branch {
		return sampleBranch(branchStatus(status), func(b *api.Branch) {
			b.CPUMillis = 2000
			b.MemoryGbs = 16
		})
	}
	expectResizeWith := func(req api.BranchResizeInput, branch api.Branch) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveBranch(m, sampleBranch())
			m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", req).
				Return(&api.ResizeBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}
	expectResize := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return expectResizeWith(req, resized(status))
	}
	accepted := map[string]any{
		"status":    "CONFIGURING",
		"resources": map[string]any{"cpu": "2 cores", "memory": "16 GB"},
		"message":   "Resize request accepted. The branch may still be resizing.",
	}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchResize,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_resize"`,
		},
		{
			name:    "missing cpu_memory and iops",
			tool:    toolBranchResize,
			args:    map[string]any{"branch_id": "br4nch1d9x"},
			opts:    []runOption{experimental},
			wantErr: "at least one of cpu_memory or iops is required",
		},
		{
			name:    "read-only all refuses without an API call",
			tool:    toolBranchResize,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			tool:       toolBranchResize,
			args:       args,
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectResize(api.DeployStatusCONFIGURING),
			wantOutput: accepted,
		},
		{
			name: "network error",
			tool: toolBranchResize,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", req).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to resize branch: connection refused",
		},
		{
			name: "nil response body",
			tool: toolBranchResize,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", req).
					Return(&api.ResizeBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// CPU and memory are left out, so the API keeps the current ones.
			name: "iops only omits cpu and memory",
			tool: toolBranchResize,
			args: map[string]any{"branch_id": "br4nch1d9x", "iops": 16000},
			opts: []runOption{experimental},
			mock: expectResizeWith(api.BranchResizeInput{Iops: new(16000)}, sampleBranch(branchStatus(api.DeployStatusCONFIGURING), func(b *api.Branch) {
				b.Iops = new(16000)
			})),
			wantOutput: map[string]any{
				"status":    "CONFIGURING",
				"resources": map[string]any{"cpu": "0.25 cores", "memory": "2 GB", "iops": float64(16000)},
				"message":   "Resize request accepted. The branch may still be resizing.",
			},
		},
		{
			name:       "resizes without waiting by default",
			tool:       toolBranchResize,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectResize(api.DeployStatusCONFIGURING),
			wantOutput: accepted,
		},
		{
			name:     "wait polls until ready",
			synctest: true,
			tool:     toolBranchResize,
			args:     map[string]any{"branch_id": "br4nch1d9x", "cpu_memory": "2 CPU/16 GB", "wait": true},
			opts:     []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResize(api.DeployStatusCONFIGURING)(m)
				expectGetBranch(m, resized(api.DeployStatusREADY))
			},
			wantOutput: map[string]any{
				"status":    "READY",
				"resources": map[string]any{"cpu": "2 cores", "memory": "16 GB"},
				"message":   "Branch resized.",
			},
		},
	})
}
