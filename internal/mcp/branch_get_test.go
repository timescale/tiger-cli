package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchGetTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"branch_id": "br4nch1d9x"}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchGet,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_get"`,
		},
		{
			name:    "not logged in",
			tool:    toolBranchGet,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "rejects a malformed branch ID",
			tool:    toolBranchGet,
			args:    map[string]any{"branch_id": "nope"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/branch_id: pattern: "nope" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name: "network error",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBranchWithResponse(validCtx, testProjectID, "br4nch1d9x").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch branch details: connection refused",
		},
		{
			name: "branch not found",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBranchWithResponse(validCtx, testProjectID, "br4nch1d9x").
					Return(&api.GetBranchResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("branch not found")},
					}, nil)
			},
			wantErr: "branch not found",
		},
		{
			name: "nil response body",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBranchWithResponse(validCtx, testProjectID, "br4nch1d9x").
					Return(&api.GetBranchResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "success",
			tool:       toolBranchGet,
			args:       args,
			opts:       []runOption{experimental},
			mock:       func(m *mocks.MockClientWithResponsesInterface) { expectGetBranch(m, sampleBranch()) },
			wantOutput: map[string]any{"branch": sampleBranchDetail(api.DeployStatusREADY)},
		},
		{
			// A branch counts as DEV, so only read_only=all makes its
			// connection string read-only.
			name: "read-only all makes the connection string read-only",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental, withConfig(map[string]any{"read_only": "all"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) { expectGetBranch(m, sampleBranch()) },
			wantOutput: map[string]any{"branch": func() map[string]any {
				detail := sampleBranchDetail(api.DeployStatusREADY)
				detail["connection_string"] = "postgresql://tsdbadmin@br4nch1d9x.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require&options=-c%20tsdb_admin.read_only_connection%3Dtrue"
				return detail
			}()},
		},
		{
			name: "provisioning branch without an endpoint",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetBranch(m, sampleBranch(branchStatus(api.DeployStatusQUEUED), func(b *api.Branch) { b.Endpoint = nil }))
			},
			wantOutput: map[string]any{"branch": map[string]any{
				"id":         "br4nch1d9x",
				"name":       "test-branch",
				"service_id": "e6ue9697jf",
				"status":     "QUEUED",
				"created":    "2025-01-16T09:00:00Z",
				"resources":  map[string]any{"cpu": "0.25 cores", "memory": "2 GB"},
			}},
		},
	})
}
