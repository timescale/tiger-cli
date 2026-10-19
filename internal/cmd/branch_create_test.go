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
	baseReq := api.BranchCreate{}

	// The create response carries the branch's new password, once.
	created := sampleBranch(branchStatus(api.DeployStatusQUEUED), func(b *api.Branch) {
		b.InitialPassword = new("branch-pw")
	})

	// setupCreate registers resolving svc-12345 onto parent and creating a
	// branch of it with req, answered with branch.
	setupCreate := func(req api.BranchCreate, branch api.Branch) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, "svc-12345", parent)
			m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, "svc-12345", req).
				Return(&api.CreateBranchResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &branch,
				}, nil)
		}
	}

	noWaitStderr := func(firstLine string) string {
		return firstLine + `
Branch ID: br-12345
Password saved to system keyring
Branch is being created. Use 'tiger branch get br-12345' to check branch status.
`
	}
	defaultNoWaitStderr := noWaitStderr("Creating branch from service 'test-service' (svc-12345)...")

	runCmdTests(t, []cmdTest{
		{
			// A branch never becomes the default, so there's nothing to opt out of.
			name:    "no --no-set-default flag",
			args:    []string{"branch", "create", "svc-12345", "--no-set-default"},
			opts:    []runOption{withExperimental()},
			wantErr: "unknown flag: --no-set-default",
		},
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
			args: []string{"branch", "create", "svc-12345", "--no-wait", "-o", "env"},
			opts: []runOption{withExperimental(), withConfig(map[string]any{"read_only": "prod"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", sampleService(fluidPlatform, envTag("PROD")))
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, "svc-12345", baseReq).
					Return(&api.CreateBranchResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &created,
					}, nil)
			},
			wantStdout: sampleBranchEnv,
			wantStderr: defaultNoWaitStderr,
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
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, "svc-12345", baseReq).
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
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, "svc-12345", api.BranchCreate{Name: new("ci-4821")}).
					Return(&api.CreateBranchResponse{
						HTTPResponse: httpResponse(http.StatusConflict),
						JSON4XX:      &api.ClientError{Message: new("a branch named ci-4821 already exists")},
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
				m.EXPECT().CreateBranchWithResponse(validCtx, testProjectID, "svc-12345", baseReq).
					Return(&api.CreateBranchResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantErr: "empty response from API",
			wantStderr: "Creating branch from service 'test-service' (svc-12345)...\n" +
				"Error: empty response from API\n",
		},
		{
			// Only service IDs go in the default, so a branch never becomes it.
			name:       "no-wait saves the password and leaves the default alone",
			args:       []string{"branch", "create", "svc-12345", "--no-wait", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       setupCreate(baseReq, created),
			wantStdout: sampleBranchEnv,
			wantStderr: defaultNoWaitStderr,
			checks:     []checkFunc{checkStoredPassword("br-12345", "branch-pw"), checkDefaultService("")},
		},
		{
			// Nothing to save, so nothing is said about saving.
			name:       "no initial password",
			args:       []string{"branch", "create", "svc-12345", "--no-wait", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       setupCreate(baseReq, sampleBranch(branchStatus(api.DeployStatusQUEUED))),
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Branch is being created. Use 'tiger branch get br-12345' to check branch status.
`,
		},
		{
			name:       "password storage none",
			args:       []string{"branch", "create", "svc-12345", "--no-wait", "--password-storage", "none", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       setupCreate(baseReq, created),
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Password not saved (--password-storage=none). Make sure to store it securely.
Branch is being created. Use 'tiger branch get br-12345' to check branch status.
`,
		},
		{
			name: "custom name and compute",
			args: []string{"branch", "create", "svc-12345", "--name", "ci-4821", "--cpu", "2000", "--memory", "16", "--no-wait", "-o", "env"},
			opts: []runOption{withExperimental()},
			mock: setupCreate(api.BranchCreate{
				Name:      new("ci-4821"),
				CPUMillis: new(2000),
				MemoryGbs: new(16),
			}, created),
			wantStdout: sampleBranchEnv,
			wantStderr: noWaitStderr("Creating branch 'ci-4821' from service 'test-service' (svc-12345)..."),
		},
		{
			name:       "iops",
			args:       []string{"branch", "create", "svc-12345", "--iops", "16000", "--no-wait", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       setupCreate(api.BranchCreate{Iops: new(16000)}, created),
			wantStdout: sampleBranchEnv,
			wantStderr: defaultNoWaitStderr,
		},
		{
			name: "memory alone sets the matching cpu",
			args: []string{"branch", "create", "svc-12345", "--memory", "4", "--no-wait", "-o", "env"},
			opts: []runOption{withExperimental()},
			mock: setupCreate(api.BranchCreate{
				CPUMillis: new(500),
				MemoryGbs: new(4),
			}, created),
			wantStdout: sampleBranchEnv,
			wantStderr: defaultNoWaitStderr,
		},
		{
			name:     "wait polls until ready",
			synctest: true,
			args:     []string{"branch", "create", "svc-12345"},
			opts:     []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupCreate(baseReq, created)(m)
				expectGetBranch(m, sampleBranch())
			},
			wantStdout: sampleBranchTable,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Password saved to system keyring
Waiting for branch to be ready (timeout: 10m0s)...
⢎  Branch status: QUEUED
Branch is ready.
Connect with: tiger db psql br-12345
`,
		},
		{
			name:     "wait timeout",
			synctest: true,
			args:     []string{"branch", "create", "svc-12345", "-o", "env"},
			opts:     []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupCreate(baseReq, created)(m)
				queued := sampleBranch(branchStatus(api.DeployStatusQUEUED))
				m.EXPECT().GetBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345").
					Return(&api.GetBranchResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &queued,
					}, nil).AnyTimes()
			},
			wantErr:    "wait timeout reached after 10m0s - branch may still be provisioning",
			wantStdout: sampleBranchEnv,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Password saved to system keyring
Waiting for branch to be ready (timeout: 10m0s)...
⢎  Branch status: QUEUED
Error: wait timeout reached after 10m0s - branch may still be provisioning
`,
			checks: []checkFunc{checkExitCode(common.ExitTimeout)},
		},
		{
			// The initial password is saved but never output.
			name: "json",
			args: []string{"branch", "create", "svc-12345", "-o", "json"},
			opts: []runOption{withExperimental()},
			mock: setupCreate(baseReq, sampleBranch(func(b *api.Branch) {
				b.InitialPassword = new("branch-pw")
			})),
			wantStdout: sampleBranchJSON,
			wantStderr: `Creating branch from service 'test-service' (svc-12345)...
Branch ID: br-12345
Password saved to system keyring
Waiting for branch to be ready (timeout: 10m0s)...
Branch is ready.
Connect with: tiger db psql br-12345
`,
		},
		{
			name:       "default service from config",
			args:       []string{"branch", "create", "--no-wait", "-o", "env"},
			opts:       []runOption{withExperimental(), withConfig(map[string]any{"service_id": "svc-12345"})},
			mock:       setupCreate(baseReq, created),
			wantStdout: sampleBranchEnv,
			wantStderr: defaultNoWaitStderr,
			checks:     []checkFunc{checkDefaultService("svc-12345")},
		},
		{
			name:       "service branch is the same command",
			args:       []string{"service", "branch", "svc-12345", "--no-wait", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       setupCreate(baseReq, created),
			wantStdout: sampleBranchEnv,
			wantStderr: defaultNoWaitStderr,
			checks:     []checkFunc{checkStoredPassword("br-12345", "branch-pw"), checkDefaultService("")},
		},
	})
}
