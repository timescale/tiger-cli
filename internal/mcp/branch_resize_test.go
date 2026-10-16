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
	req := api.ResizeInput{CPUMillis: "2000", MemoryGbs: "16"}
	expectResize := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			branch := sampleBranch(branchStatus(status))
			m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "br4nch1d9x", req).
				Return(&api.ResizeBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}
	// The response's resources are the sample's, since the resize hasn't
	// landed yet.
	resources := map[string]any{"cpu": "0.25 cores", "memory": "2 GB"}
	accepted := map[string]any{
		"status":    "CONFIGURING",
		"resources": resources,
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
			name:    "missing cpu_memory",
			tool:    toolBranchResize,
			args:    map[string]any{"branch_id": "br4nch1d9x"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: required: missing properties: ["cpu_memory"]`,
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
				m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "br4nch1d9x", req).
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
				m.EXPECT().ResizeBranchWithResponse(validCtx, testProjectID, "br4nch1d9x", req).
					Return(&api.ResizeBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
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
				expectGetBranch(m, sampleBranch(func(b *api.Branch) {
					b.CPUMillis = 2000
					b.MemoryGbs = 16
				}))
			},
			wantOutput: map[string]any{
				"status":    "READY",
				"resources": map[string]any{"cpu": "2 cores", "memory": "16 GB"},
				"message":   "Branch resized.",
			},
		},
	})
}
