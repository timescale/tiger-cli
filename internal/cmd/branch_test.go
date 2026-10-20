package cmd

import (
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

// TestBranchExperimentalGate covers the registration gate for the branch
// commands: `tiger branch` and `tiger service branch` exist only behind the
// experimental env var.
func TestBranchExperimentalGate(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:    "branch unregistered by default",
			args:    []string{"branch", "list"},
			wantErr: `unknown command "branch" for "tiger"`,
			wantStderr: "Error: unknown command \"branch\" for \"tiger\"\n" +
				"Run 'tiger --help' for usage.\n",
		},
		{
			// Cobra treats an unknown name under a group as a stray argument
			// and prints the group's help (see TestServiceExperimentalGate).
			name:       "service branch unregistered by default",
			args:       []string{"service", "branch"},
			wantStdout: matchHelp("Available Commands:", "branch"),
		},
		{
			name:       "branch registered when experimental",
			args:       []string{"branch", "--help"},
			opts:       []runOption{withExperimental()},
			wantStdout: matchHelp("Manage branches of Fluid Services."),
		},
		{
			name:       "branches alias",
			args:       []string{"branches", "--help"},
			opts:       []runOption{withExperimental()},
			wantStdout: matchHelp("Manage branches of Fluid Services."),
		},
		{
			name:       "service branch registered when experimental",
			args:       []string{"service", "branch", "--help"},
			opts:       []runOption{withExperimental()},
			wantStdout: matchHelp("tiger service branch svc-12345 --name ci-4821"),
		},
	})
}

// withExperimental turns on the experimental gate, which every branch command
// sits behind.
func withExperimental() runOption {
	return withEnv("TIGER_EXPERIMENTAL", "true")
}

// sampleBranch returns a branch of sampleService with reasonable defaults.
// Use overrides to customize specific fields.
func sampleBranch(overrides ...func(*api.Branch)) api.Branch {
	branch := api.Branch{
		ID:        "br-12345",
		ProjectID: testProjectID,
		ServiceID: "svc-12345",
		Name:      "test-branch",
		Status:    api.DeployStatusREADY,
		CPUMillis: 250,
		MemoryGbs: 2,
		Created:   time.Date(2025, 1, 16, 9, 0, 0, 0, time.UTC),
		Endpoint: &api.Endpoint{
			Host: new("br-12345.test-project-123.branch.tsdb.cloud.timescale.com"),
			Port: new(36956),
		},
	}
	for _, o := range overrides {
		o(&branch)
	}
	return branch
}

// branchStatus returns a sampleBranch override setting the branch's status.
func branchStatus(status api.DeployStatus) func(*api.Branch) {
	return func(b *api.Branch) { b.Status = status }
}

// branchType is the resolve type every `tiger branch` command restricts its
// lookup to.
var branchType = new(api.ResolveTypeBRANCH)

// expectResolveBranch registers resolving ref as a branch, answered with
// branch.
func expectResolveBranch(m *mocks.MockClientWithResponsesInterface, ref string, branch api.Branch) {
	expectResolve(m, ref, branchType, branch)
}

// expectResolveBranchStatus registers resolving ref as a branch, answered with
// status. A nil apiErr stands in for a response with no body.
func expectResolveBranchStatus(m *mocks.MockClientWithResponsesInterface, ref string, status int, apiErr *api.ClientError) {
	expectResolveStatus(m, ref, branchType, status, apiErr)
}

// expectGetBranch registers one fetch of branch, through its parent, as a
// wait loop's poll makes it.
func expectGetBranch(m *mocks.MockClientWithResponsesInterface, branch api.Branch) {
	m.EXPECT().GetBranchWithResponse(validCtx, testProjectID, branch.ServiceID, branch.ID).
		Return(&api.GetBranchResponse{
			HTTPResponse: httpResponse(http.StatusOK),
			JSON200:      &branch,
		}, nil)
}

// sampleBranchEnv is the env output of sampleBranch.
const sampleBranchEnv = `PGHOST=br-12345.test-project-123.branch.tsdb.cloud.timescale.com
PGPORT=36956
PGDATABASE=tsdb
PGUSER=tsdbadmin
`

// sampleBranchTable and sampleBranchJSON are the table and JSON output of
// sampleBranch.
const (
	sampleBranchTable = `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                                    VALUE                                                    │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ Branch ID         │ br-12345                                                                                                    │
│ Name              │ test-branch                                                                                                 │
│ Parent Service    │ test-service (svc-12345)                                                                                    │
│ Status            │ READY                                                                                                       │
│ CPU               │ 0.25 cores (250m)                                                                                           │
│ Memory            │ 2 GB                                                                                                        │
│ Direct Endpoint   │ br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956                                             │
│ Created           │ 2025-01-16 09:00:00 UTC                                                                                     │
│ Connection String │ postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345/branches/br-12345                          │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
`
	sampleBranchJSON = `{
  "cpu_millis": 250,
  "created": "2025-01-16T09:00:00Z",
  "endpoint": {
    "host": "br-12345.test-project-123.branch.tsdb.cloud.timescale.com",
    "port": 36956
  },
  "id": "br-12345",
  "memory_gbs": 2,
  "name": "test-branch",
  "project_id": "test-project-123",
  "service_id": "svc-12345",
  "status": "READY",
  "service_name": "test-service",
  "role": "tsdbadmin",
  "host": "br-12345.test-project-123.branch.tsdb.cloud.timescale.com",
  "port": 36956,
  "database": "tsdb",
  "connection_string": "postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require",
  "console_url": "https://console.cloud.tigerdata.com/dashboard/services/svc-12345/branches/br-12345"
}
`
)
