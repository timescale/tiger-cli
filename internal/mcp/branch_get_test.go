package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchGetTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"branch_id": "br4nch1d9x"}

	// getBranch registers resolving branch and then fetching its parent, for
	// the parent's name.
	getBranch := func(branch api.Branch) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveBranch(m, branch)
			expectGetService(m, "e6ue9697jf", sampleService())
		}
	}
	withServiceName := func(detail map[string]any) map[string]any {
		detail["service_name"] = "test-service"
		return detail
	}

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
				m.EXPECT().ResolveRefWithResponse(validCtx, testProjectID, &api.ResolveRefParams{Ref: "br4nch1d9x", Type: branchType}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to resolve branch 'br4nch1d9x': connection refused",
		},
		{
			name: "branch not found",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveStatus(m, "br4nch1d9x", branchType, http.StatusNotFound, &api.ClientError{Message: new("resource not found")})
			},
			wantErr: "branch 'br4nch1d9x' not found",
		},
		{
			name: "nil response body",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveStatus(m, "br4nch1d9x", branchType, http.StatusOK, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// The tools are IDs-only, but the resolve endpoint also matches
			// names, so a name shaped like an ID resolves to some other
			// branch. That match is refused.
			name: "refuses a match by name",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolve(m, "br4nch1d9x", branchType, sampleBranch(func(b *api.Branch) {
					b.ID = "0th3rbr4nc"
					b.Name = "br4nch1d9x"
				}))
			},
			wantErr: "branch 'br4nch1d9x' not found",
		},
		{
			name:       "success",
			tool:       toolBranchGet,
			args:       args,
			opts:       []runOption{experimental},
			mock:       getBranch(sampleBranch()),
			wantOutput: map[string]any{"branch": withServiceName(sampleBranchDetail(api.DeployStatusREADY))},
		},
		{
			// The branch has its own password, so its parent's isn't used.
			name: "with password",
			tool: toolBranchGet,
			args: map[string]any{"branch_id": "br4nch1d9x", "with_password": true},
			opts: []runOption{
				experimental,
				withStoredPassword(common.BranchPasswordTarget(sampleBranch()), "branch-pw"),
				withStoredPassword(common.ServicePasswordTarget(sampleService()), "parent-pw"),
			},
			mock: getBranch(sampleBranch()),
			wantOutput: map[string]any{"branch": func() map[string]any {
				detail := withServiceName(sampleBranchDetail(api.DeployStatusREADY))
				detail["password"] = "branch-pw"
				detail["connection_string"] = "postgresql://tsdbadmin:branch-pw@br4nch1d9x.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require"
				return detail
			}()},
		},
		{
			// Refused before the parent's name is fetched.
			name: "with password unavailable",
			tool: toolBranchGet,
			args: map[string]any{"branch_id": "br4nch1d9x", "with_password": true},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
			},
			wantErr: "requested password but password not available",
		},
		{
			// The IOPS go in resources, as they do for services.
			name: "returns the IOPS in resources",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: getBranch(sampleBranch(func(b *api.Branch) { b.Iops = new(16000) })),
			wantOutput: map[string]any{"branch": func() map[string]any {
				detail := withServiceName(sampleBranchDetail(api.DeployStatusREADY))
				detail["resources"] = map[string]any{"cpu": "0.25 cores", "memory": "2 GB", "iops": float64(16000)}
				return detail
			}()},
		},
		{
			// The parent's name is the only thing the second fetch adds, so
			// without it the branch is still returned.
			name: "parent fetch fails",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantOutput: map[string]any{"branch": sampleBranchDetail(api.DeployStatusREADY)},
		},
		{
			// A branch counts as DEV, so only read_only=all makes its
			// connection string read-only.
			name: "read-only all makes the connection string read-only",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental, withConfig(map[string]any{"read_only": "all"})},
			mock: getBranch(sampleBranch()),
			wantOutput: map[string]any{"branch": func() map[string]any {
				detail := withServiceName(sampleBranchDetail(api.DeployStatusREADY))
				detail["connection_string"] = "postgresql://tsdbadmin@br4nch1d9x.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require&options=-c%20tsdb_admin.read_only_connection%3Dtrue"
				return detail
			}()},
		},
		{
			// No port is guessed, so without one there's no endpoint to show
			// or connect to.
			name: "endpoint without a port",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: getBranch(sampleBranch(func(b *api.Branch) { b.Endpoint.Port = nil })),
			wantOutput: map[string]any{"branch": map[string]any{
				"id":           "br4nch1d9x",
				"name":         "test-branch",
				"service_id":   "e6ue9697jf",
				"service_name": "test-service",
				"status":       "READY",
				"created":      "2025-01-16T09:00:00Z",
				"resources":    map[string]any{"cpu": "0.25 cores", "memory": "2 GB"},
			}},
		},
		{
			// As for a service, the failure to build a connection string is
			// logged, and the rest of the branch is still returned.
			name: "branch without an endpoint",
			tool: toolBranchGet,
			args: args,
			opts: []runOption{experimental},
			mock: getBranch(sampleBranch(branchStatus(api.DeployStatusQUEUED), func(b *api.Branch) { b.Endpoint = nil })),
			wantOutput: map[string]any{"branch": map[string]any{
				"id":           "br4nch1d9x",
				"name":         "test-branch",
				"service_id":   "e6ue9697jf",
				"service_name": "test-service",
				"status":       "QUEUED",
				"created":      "2025-01-16T09:00:00Z",
				"resources":    map[string]any{"cpu": "0.25 cores", "memory": "2 GB"},
			}},
		},
	})
}
