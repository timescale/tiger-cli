package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchCreateTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"service_id": "e6ue9697jf"}
	baseReq := api.BranchCreate{}

	// The create response carries the branch's new password, once.
	created := sampleBranch(branchStatus(api.DeployStatusQUEUED), func(b *api.Branch) {
		b.InitialPassword = new("branch-pw")
	})

	// expectCreate registers creating a branch of the sample service with req,
	// answered with branch.
	expectCreate := func(req api.BranchCreate, branch api.Branch) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", req).
				Return(&api.CreateBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}

	acceptedMsg := "Branch creation request accepted. The branch may still be provisioning."
	storedPassword := map[string]any{
		"success": true,
		"method":  "keyring",
		"message": "Password saved to system keyring",
	}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchCreate,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_create"`,
		},
		{
			name:    "not logged in",
			tool:    toolBranchCreate,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "rejects a combination a branch can't have",
			tool:    toolBranchCreate,
			args:    map[string]any{"service_id": "e6ue9697jf", "cpu_memory": "1 CPU/4 GB"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/cpu_memory: enum: 1 CPU/4 GB does not equal any of: [0.25 CPU/2 GB 0.5 CPU/4 GB 1 CPU/8 GB 2 CPU/16 GB 4 CPU/32 GB 8 CPU/64 GB 16 CPU/128 GB 32 CPU/256 GB]`,
		},
		{
			name:    "read-only all refuses without an API call",
			tool:    toolBranchCreate,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// A branch always counts as DEV, so prod allows branching even a
			// PROD service, without looking up the service's tag.
			name: "read-only prod allows branching",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock: expectCreate(baseReq, created),
			wantOutput: map[string]any{
				"branch":           sampleBranchDetail(api.DeployStatusQUEUED),
				"message":          acceptedMsg,
				"password_storage": storedPassword,
			},
		},
		{
			name: "network error",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", baseReq).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create branch: connection refused",
		},
		{
			name: "API error",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", baseReq).
					Return(&api.CreateBranchResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.ClientError{Message: new("service is not a Fluid Service")},
					}, nil)
			},
			wantErr: "service is not a Fluid Service",
		},
		{
			name: "nil response body",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, "e6ue9697jf", baseReq).
					Return(&api.CreateBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// Only service IDs go in the default, so a branch never becomes it.
			name: "saves the password and leaves the default alone",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental},
			mock: expectCreate(baseReq, created),
			wantOutput: map[string]any{
				"branch":           sampleBranchDetail(api.DeployStatusQUEUED),
				"message":          acceptedMsg,
				"password_storage": storedPassword,
			},
			checks: []checkFunc{checkStoredPassword("br4nch1d9x", "branch-pw"), checkDefaultService("")},
		},
		{
			// A storage failure isn't fatal: the branch exists either way, so
			// the tool reports the failure rather than erroring.
			name: "reports a password storage failure",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental, withKeyringError(errors.New("keyring is locked"))},
			mock: expectCreate(baseReq, created),
			wantOutput: map[string]any{
				"branch":  sampleBranchDetail(api.DeployStatusQUEUED),
				"message": acceptedMsg,
				"password_storage": map[string]any{
					"success": false,
					"method":  "keyring",
					"message": "Failed to save password to keyring: keyring is locked",
				},
			},
		},
		{
			name: "no initial password",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental},
			mock: expectCreate(baseReq, sampleBranch(branchStatus(api.DeployStatusQUEUED))),
			wantOutput: map[string]any{
				"branch":  sampleBranchDetail(api.DeployStatusQUEUED),
				"message": acceptedMsg,
			},
		},
		{
			name: "custom name and compute",
			tool: toolBranchCreate,
			args: map[string]any{"service_id": "e6ue9697jf", "name": "ci-4821", "cpu_memory": "2 CPU/16 GB"},
			opts: []runOption{experimental},
			mock: expectCreate(api.BranchCreate{
				Name:      new("ci-4821"),
				CPUMillis: new(2000),
				MemoryGbs: new(16),
			}, created),
			wantOutput: map[string]any{
				"branch":           sampleBranchDetail(api.DeployStatusQUEUED),
				"message":          acceptedMsg,
				"password_storage": storedPassword,
			},
		},
		{
			name: "iops",
			tool: toolBranchCreate,
			args: map[string]any{"service_id": "e6ue9697jf", "iops": 16000},
			opts: []runOption{experimental},
			mock: expectCreate(api.BranchCreate{Iops: new(16000)}, created),
			wantOutput: map[string]any{
				"branch":           sampleBranchDetail(api.DeployStatusQUEUED),
				"message":          acceptedMsg,
				"password_storage": storedPassword,
			},
		},
		{
			// The polls never carry the password, so waiting must keep the
			// create response's.
			name:     "with password survives the wait",
			synctest: true,
			tool:     toolBranchCreate,
			args:     map[string]any{"service_id": "e6ue9697jf", "wait": true, "with_password": true},
			opts:     []runOption{experimental, withConfig(map[string]any{"password_storage": "none"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectCreate(baseReq, created)(m)
				expectGetBranch(m, sampleBranch())
			},
			wantOutput: map[string]any{
				"branch": func() map[string]any {
					detail := sampleBranchDetail(api.DeployStatusREADY)
					detail["password"] = "branch-pw"
					detail["connection_string"] = "postgresql://tsdbadmin:branch-pw@br4nch1d9x.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require"
					return detail
				}(),
				"message": "Branch is ready.",
				"password_storage": map[string]any{
					"success": false,
					"method":  "none",
					"message": "Password not saved (--password-storage=none). Make sure to store it securely.",
				},
			},
		},
		{
			// The password is returned even when the connection string can't
			// be built, since it may be the only copy.
			name: "with password and no endpoint",
			tool: toolBranchCreate,
			args: map[string]any{"service_id": "e6ue9697jf", "with_password": true},
			opts: []runOption{experimental, withConfig(map[string]any{"password_storage": "none"})},
			mock: expectCreate(baseReq, sampleBranch(branchStatus(api.DeployStatusQUEUED), func(b *api.Branch) {
				b.InitialPassword = new("branch-pw")
				b.Endpoint = nil
			})),
			wantOutput: map[string]any{
				"branch": map[string]any{
					"id":         "br4nch1d9x",
					"name":       "test-branch",
					"service_id": "e6ue9697jf",
					"status":     "QUEUED",
					"created":    "2025-01-16T09:00:00Z",
					"resources":  map[string]any{"cpu": "0.25 cores", "memory": "2 GB"},
					"password":   "branch-pw",
				},
				"message": acceptedMsg,
				"password_storage": map[string]any{
					"success": false,
					"method":  "none",
					"message": "Password not saved (--password-storage=none). Make sure to store it securely.",
				},
			},
		},
		{
			name:     "wait polls until ready",
			synctest: true,
			tool:     toolBranchCreate,
			args:     map[string]any{"service_id": "e6ue9697jf", "wait": true},
			opts:     []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectCreate(baseReq, created)(m)
				expectGetBranch(m, sampleBranch())
			},
			wantOutput: map[string]any{
				"branch":           sampleBranchDetail(api.DeployStatusREADY),
				"message":          "Branch is ready.",
				"password_storage": storedPassword,
			},
		},
	})
}
