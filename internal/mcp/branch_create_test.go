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
	baseReq := api.BranchCreate{ServiceID: "e6ue9697jf"}
	parent := sampleService()
	parentPassword := withStoredPassword(parent, "parent-pw")
	queued := sampleBranch(branchStatus(api.DeployStatusQUEUED))

	// setupCreate registers fetching the parent and creating a branch of it
	// with req, answered with branch.
	setupCreate := func(req api.BranchCreate, branch api.Branch) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "e6ue9697jf", parent)
			m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, req).
				Return(&api.CreateBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}

	sameUsers := ` The branch uses the same users and passwords as service "test-service" (e6ue9697jf).`
	acceptedMsg := "Branch creation request accepted. The branch may still be provisioning." + sameUsers
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
			// A branch always counts as DEV, so prod allows branching a PROD
			// service.
			name: "read-only prod allows branching a PROD service",
			tool: toolBranchCreate,
			args: map[string]any{"service_id": "e6ue9697jf", "set_default": false},
			opts: []runOption{experimental, withConfig(map[string]any{"read_only": "prod"}), parentPassword},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("PROD", 1)(m)
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateBranchResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &queued,
					}, nil)
			},
			wantOutput: map[string]any{
				"branch":           sampleBranchDetail(api.DeployStatusQUEUED),
				"message":          acceptedMsg,
				"password_storage": storedPassword,
			},
		},
		{
			name: "parent not found",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "network error",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", parent)
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, baseReq).
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
				expectGetService(m, "e6ue9697jf", parent)
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, baseReq).
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
				expectGetService(m, "e6ue9697jf", parent)
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateBranchResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "copies the password and sets the default",
			tool: toolBranchCreate,
			args: args,
			opts: []runOption{experimental, parentPassword},
			mock: setupCreate(baseReq, queued),
			wantOutput: map[string]any{
				"branch":           sampleBranchDetail(api.DeployStatusQUEUED),
				"message":          acceptedMsg,
				"password_storage": storedPassword,
			},
			checks: []checkFunc{checkStoredPassword("br4nch1d9x", "parent-pw"), checkDefaultService("br4nch1d9x")},
		},
		{
			// A missing parent password isn't fatal: the branch exists either
			// way, so the tool reports the failure rather than erroring.
			name: "reports a parent password that isn't saved",
			tool: toolBranchCreate,
			args: map[string]any{"service_id": "e6ue9697jf", "set_default": false},
			opts: []runOption{experimental},
			mock: setupCreate(baseReq, queued),
			wantOutput: map[string]any{
				"branch":  sampleBranchDetail(api.DeployStatusQUEUED),
				"message": acceptedMsg,
				"password_storage": map[string]any{
					"success": false,
					"method":  "keyring",
					"message": "Failed to copy the password of parent service e6ue9697jf: no password found in keyring for this service",
				},
			},
			checks: []checkFunc{checkDefaultService("")},
		},
		{
			name: "custom name and compute",
			tool: toolBranchCreate,
			args: map[string]any{"service_id": "e6ue9697jf", "name": "ci-4821", "cpu_memory": "2 CPU/16 GB", "set_default": false},
			opts: []runOption{experimental, parentPassword},
			mock: setupCreate(api.BranchCreate{
				ServiceID: "e6ue9697jf",
				Name:      new("ci-4821"),
				CPUMillis: new(2000),
				MemoryGbs: new(16),
			}, queued),
			wantOutput: map[string]any{
				"branch":           sampleBranchDetail(api.DeployStatusQUEUED),
				"message":          acceptedMsg,
				"password_storage": storedPassword,
			},
		},
		{
			name:     "wait polls until ready",
			synctest: true,
			tool:     toolBranchCreate,
			args:     map[string]any{"service_id": "e6ue9697jf", "wait": true, "set_default": false},
			opts:     []runOption{experimental, parentPassword},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupCreate(baseReq, queued)(m)
				expectGetBranch(m, sampleBranch())
			},
			wantOutput: map[string]any{
				"branch":           sampleBranchDetail(api.DeployStatusREADY),
				"message":          "Branch is ready." + sameUsers,
				"password_storage": storedPassword,
			},
		},
	})
}
