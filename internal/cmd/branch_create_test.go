package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchCreateCmd(t *testing.T) {
	parent := sampleService(fluidPlatform)
	baseReq := api.BranchCreate{ServiceID: "svc-12345"}

	// setupCreate registers resolving svc-12345 onto parent and creating a
	// branch of it with req, answered with branch.
	setupCreate := func(req api.BranchCreate, branch api.Branch) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, "svc-12345", parent)
			m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, req).
				Return(&api.CreateBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}
	provisioning := sampleBranch(branchStatus(api.DeployStatusQUEUED))

	// The parent's saved password, which create copies to the branch.
	parentPassword := withStoredPassword(parent, "parent-pw")

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"branch", "create", "svc-12345"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only all refuses",
			args:    []string{"branch", "create", "svc-12345"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// A branch always counts as DEV, so prod allows branching a PROD
			// service, as it allows forking one into a DEV fork.
			name: "read-only prod allows branching a PROD service",
			args: []string{"branch", "create", "svc-12345", "--no-wait", "--no-set-default", "-o", "env"},
			opts: []runOption{withExperimental(), withConfig(map[string]any{"read_only": "prod"}), parentPassword},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService(fluidPlatform, envTag("PROD")))
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateBranchResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &provisioning,
					}, nil)
			},
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password saved to system keyring
Branch is being created. Use 'tiger branch get' to check branch status.
`,
		},
		{
			name:    "invalid cpu/memory combination",
			args:    []string{"branch", "create", "svc-12345", "--cpu", "1000", "--memory", "4"},
			opts:    []runOption{withExperimental()},
			wantErr: "invalid CPU/Memory combination. Allowed combinations: 0.25 CPU/2 GB, 0.5 CPU/4 GB, 1 CPU/8 GB, 2 CPU/16 GB, 4 CPU/32 GB, 8 CPU/64 GB, 16 CPU/128 GB, 32 CPU/256 GB",
		},
		{
			name:    "missing service id",
			args:    []string{"branch", "create"},
			opts:    []runOption{withExperimental()},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "service not found",
			args: []string{"branch", "create", "svc-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefNotFound(m, "svc-12345")
			},
			wantErr: "service 'svc-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "network error",
			args: []string{"branch", "create", "svc-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", parent)
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, baseReq).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create branch: connection refused",
			wantStderr: "Creating branch from service 'test-service' (svc-12345)...\n" +
				"Error: failed to create branch: connection refused\n",
		},
		{
			name: "API error",
			args: []string{"branch", "create", "svc-12345", "--name", "ci-4821"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", parent)
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, api.BranchCreate{ServiceID: "svc-12345", Name: new("ci-4821")}).
					Return(&api.CreateBranchResponse{
						HTTPResponse: httpResponse(http.StatusConflict),
						JSON4XX:      &api.Error{Message: new("a branch named ci-4821 already exists")},
					}, nil)
			},
			wantErr: "a branch named ci-4821 already exists",
			wantStderr: "Creating branch 'ci-4821' from service 'test-service' (svc-12345)...\n" +
				"Error: a branch named ci-4821 already exists\n",
		},
		{
			name: "nil response body",
			args: []string{"branch", "create", "svc-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", parent)
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateBranchResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantErr: "empty response from API",
			wantStderr: "Creating branch from service 'test-service' (svc-12345)...\n" +
				"Error: empty response from API\n",
		},
		{
			name:       "no-wait copies the password and sets the default",
			args:       []string{"branch", "create", "svc-12345", "--no-wait", "-o", "env"},
			opts:       []runOption{withExperimental(), parentPassword},
			mock:       setupCreate(baseReq, provisioning),
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password saved to system keyring
Default set to branch br-12345.
Branch is being created. Use 'tiger branch get' to check branch status.
`,
			checks: []checkFunc{checkStoredPassword("br-12345", "parent-pw"), checkDefaultService("br-12345")},
		},
		{
			name:       "parent password not saved",
			args:       []string{"branch", "create", "svc-12345", "--no-wait", "--no-set-default", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       setupCreate(baseReq, provisioning),
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Warning: Failed to copy the password of parent service svc-12345: no password found in keyring for this service
Branch is being created. Use 'tiger branch get' to check branch status.
`,
		},
		{
			name:       "password storage none",
			args:       []string{"branch", "create", "svc-12345", "--no-wait", "--no-set-default", "--password-storage", "none", "-o", "env"},
			opts:       []runOption{withExperimental(), parentPassword},
			mock:       setupCreate(baseReq, provisioning),
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password not saved (--password-storage=none)
Branch is being created. Use 'tiger branch get' to check branch status.
`,
		},
		{
			name: "custom name and compute",
			args: []string{"branch", "create", "svc-12345", "--name", "ci-4821", "--cpu", "2000", "--memory", "16", "--no-wait", "--no-set-default", "-o", "env"},
			opts: []runOption{withExperimental(), parentPassword},
			mock: setupCreate(api.BranchCreate{
				ServiceID: "svc-12345",
				Name:      new("ci-4821"),
				CPUMillis: new(2000),
				MemoryGbs: new(16),
			}, provisioning),
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch 'ci-4821' from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password saved to system keyring
Branch is being created. Use 'tiger branch get' to check branch status.
`,
		},
		{
			name: "memory alone sets the matching cpu",
			args: []string{"branch", "create", "svc-12345", "--memory", "4", "--no-wait", "--no-set-default", "-o", "env"},
			opts: []runOption{withExperimental(), parentPassword},
			mock: setupCreate(api.BranchCreate{
				ServiceID: "svc-12345",
				CPUMillis: new(500),
				MemoryGbs: new(4),
			}, provisioning),
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password saved to system keyring
Branch is being created. Use 'tiger branch get' to check branch status.
`,
		},
		{
			name:     "wait polls until ready",
			synctest: true,
			args:     []string{"branch", "create", "svc-12345", "--no-set-default"},
			opts:     []runOption{withExperimental(), parentPassword},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupCreate(baseReq, provisioning)(m)
				expectGetBranch(m, sampleBranch())
			},
			wantStdout: sampleBranchTable,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password saved to system keyring
Waiting for branch to be ready (timeout: 10m0s)...
⢎  Branch status: QUEUED
Branch is ready.
`,
		},
		{
			name:     "wait timeout",
			synctest: true,
			args:     []string{"branch", "create", "svc-12345", "--no-set-default", "-o", "env"},
			opts:     []runOption{withExperimental(), parentPassword},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupCreate(baseReq, provisioning)(m)
				m.EXPECT().GetBranchWithResponse(validCtx, testProjectID, "br-12345").
					Return(&api.GetBranchResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &provisioning,
					}, nil).AnyTimes()
			},
			wantErr:    "wait timeout reached after 10m0s - branch may still be provisioning",
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password saved to system keyring
Waiting for branch to be ready (timeout: 10m0s)...
⢎  Branch status: QUEUED
Error: wait timeout reached after 10m0s - branch may still be provisioning
`,
			checks: []checkFunc{checkExitCode(common.ExitTimeout)},
		},
		{
			name:       "json",
			args:       []string{"branch", "create", "svc-12345", "--no-set-default", "-o", "json"},
			opts:       []runOption{withExperimental(), parentPassword},
			mock:       setupCreate(baseReq, sampleBranch()),
			wantStdout: sampleBranchJSON,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password saved to system keyring
Waiting for branch to be ready (timeout: 10m0s)...
Branch is ready.
`,
		},
		{
			name:       "default service from config",
			args:       []string{"branch", "create", "--no-wait", "--no-set-default", "-o", "env"},
			opts:       []runOption{withExperimental(), parentPassword, withConfig(map[string]any{"service_id": "svc-12345"})},
			mock:       setupCreate(baseReq, provisioning),
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password saved to system keyring
Branch is being created. Use 'tiger branch get' to check branch status.
`,
		},
		{
			name:       "service branch is the same command",
			args:       []string{"service", "branch", "svc-12345", "--no-wait", "-o", "env"},
			opts:       []runOption{withExperimental(), parentPassword},
			mock:       setupCreate(baseReq, provisioning),
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Users and passwords are the same as service 'test-service' (svc-12345).
Password saved to system keyring
Default set to branch br-12345.
Branch is being created. Use 'tiger branch get' to check branch status.
`,
			checks: []checkFunc{checkStoredPassword("br-12345", "parent-pw"), checkDefaultService("br-12345")},
		},
	})
}
