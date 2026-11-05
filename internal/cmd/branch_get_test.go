package cmd

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchGetCmd(t *testing.T) {
	getSample := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveBranch(m, "br-12345", sampleBranch())
		expectGetService(m, "svc-12345", sampleService(fluidPlatform))
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"branch", "get", "br-12345"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			// No default fallback: service_id only ever holds a service ID.
			name:    "missing branch id",
			args:    []string{"branch", "get"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: "branch name or ID is required",
		},
		{
			name: "network error",
			args: []string{"branch", "get", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveError(m, "br-12345", branchType, errors.New("connection refused"))
			},
			wantErr: "failed to resolve branch 'br-12345': connection refused",
		},
		{
			name: "branch not found",
			args: []string{"branch", "get", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranchStatus(m, "br-12345", http.StatusNotFound, &api.ClientError{Message: new("branch not found")})
			},
			wantErr: "branch 'br-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "ambiguous name refused",
			args: []string{"branch", "get", "ci-4821"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranchStatus(m, "ci-4821", http.StatusBadRequest, &api.ClientError{Message: new("ref matches multiple branches")})
			},
			wantErr: "ref matches multiple branches\nRun 'tiger branch list' on the branch's service to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "API error",
			args: []string{"branch", "get", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranchStatus(m, "br-12345", http.StatusForbidden, &api.ClientError{Message: new("forbidden")})
			},
			wantErr: "forbidden",
			checks:  []checkFunc{checkExitCode(common.ExitPermissionDenied)},
		},
		{
			name: "nil response body",
			args: []string{"branch", "get", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranchStatus(m, "br-12345", http.StatusOK, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "table",
			args:       []string{"branch", "get", "br-12345"},
			opts:       []runOption{withExperimental()},
			mock:       getSample,
			wantStdout: sampleBranchTable,
		},
		{
			name:       "json",
			args:       []string{"branch", "get", "br-12345", "-o", "json"},
			opts:       []runOption{withExperimental()},
			mock:       getSample,
			wantStdout: sampleBranchJSON,
		},
		{
			name: "yaml",
			args: []string{"branch", "get", "br-12345", "-o", "yaml"},
			opts: []runOption{withExperimental()},
			mock: getSample,
			wantStdout: `connection_string: postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require
console_url: https://console.cloud.tigerdata.com/projects/test-project-123/services/svc-12345/branches/br-12345/overview
cpu_millis: 250
created: "2025-01-16T09:00:00Z"
database: tsdb
endpoint:
  host: br-12345.test-project-123.branch.tsdb.cloud.timescale.com
  port: 36956
host: br-12345.test-project-123.branch.tsdb.cloud.timescale.com
id: br-12345
memory_gbs: 2
name: test-branch
port: 36956
project_id: test-project-123
role: tsdbadmin
service_id: svc-12345
service_name: test-service
status: READY
`,
		},
		{
			// The parent's name is the only thing the second fetch adds, so
			// without it the branch is still shown, by parent ID.
			name: "parent fetch fails",
			args: []string{"branch", "get", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantStdout: strings.Replace(sampleBranchTable,
				"│ Parent Service    │ test-service (svc-12345)  ",
				"│ Parent Service    │ svc-12345                 ", 1),
			wantStderr: "Warning: failed to get parent service: failed to fetch service details: connection refused\n",
		},
		{
			name:       "env",
			args:       []string{"branch", "get", "br-12345", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       getSample,
			wantStdout: sampleBranchEnv,
		},
		{
			name:       "env with password",
			args:       []string{"branch", "get", "br-12345", "-o", "env", "--with-password"},
			opts:       []runOption{withExperimental(), withStoredPassword(common.BranchPasswordTarget(sampleBranch()), "branch-pw")},
			mock:       getSample,
			wantStdout: sampleBranchEnv + "PGPASSWORD=branch-pw\n",
		},
		{
			name: "table with password",
			args: []string{"branch", "get", "br-12345", "--with-password"},
			opts: []runOption{withExperimental(), withStoredPassword(common.BranchPasswordTarget(sampleBranch()), "branch-pw")},
			mock: getSample,
			wantStdout: `┌───────────────────┬───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                                         VALUE                                                         │
├───────────────────┼───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ Branch ID         │ br-12345                                                                                                              │
│ Name              │ test-branch                                                                                                           │
│ Parent Service    │ test-service (svc-12345)                                                                                              │
│ Status            │ READY                                                                                                                 │
│ CPU               │ 0.25 cores (250m)                                                                                                     │
│ Memory            │ 2 GB                                                                                                                  │
│ Direct Endpoint   │ br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956                                                       │
│ Created           │ 2025-01-16 09:00:00 UTC                                                                                               │
│ Password          │ branch-pw                                                                                                             │
│ Connection String │ postgresql://tsdbadmin:branch-pw@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/projects/test-project-123/services/svc-12345/branches/br-12345/overview           │
└───────────────────┴───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
`,
		},
		{
			// The branch has its own password, so its parent's isn't used.
			name: "json with password",
			args: []string{"branch", "get", "br-12345", "-o", "json", "--with-password"},
			opts: []runOption{withExperimental(), withStoredPassword(common.BranchPasswordTarget(sampleBranch()), "branch-pw"), withStoredPassword(common.ServicePasswordTarget(sampleService()), "parent-pw")},
			mock: getSample,
			wantStdout: `{
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
  "password": "branch-pw",
  "host": "br-12345.test-project-123.branch.tsdb.cloud.timescale.com",
  "port": 36956,
  "database": "tsdb",
  "connection_string": "postgresql://tsdbadmin:branch-pw@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require",
  "console_url": "https://console.cloud.tigerdata.com/projects/test-project-123/services/svc-12345/branches/br-12345/overview"
}
`,
		},
		{
			name:    "with password unavailable",
			args:    []string{"branch", "get", "br-12345", "--with-password"},
			opts:    []runOption{withExperimental()},
			mock:    getSample,
			wantErr: "password requested but not available for branch br-12345",
		},
		{
			// A branch counts as DEV, so only read_only=all makes its
			// connection string read-only.
			name: "read-only all makes the connection string read-only",
			args: []string{"branch", "get", "br-12345", "-o", "json"},
			opts: []runOption{withExperimental(), withConfig(map[string]any{"read_only": "all"})},
			mock: getSample,
			wantStdout: `{
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
  "connection_string": "postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require\u0026options=-c%20tsdb_admin.read_only_connection%3Dtrue",
  "console_url": "https://console.cloud.tigerdata.com/projects/test-project-123/services/svc-12345/branches/br-12345/overview"
}
`,
		},
		{
			// No port is guessed, so without one there's no endpoint to show
			// or connect to.
			name: "endpoint without a port",
			args: []string{"branch", "get", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch(func(b *api.Branch) { b.Endpoint.Port = nil }))
				expectGetService(m, "svc-12345", sampleService(fluidPlatform))
			},
			wantStdout: `┌────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│    PROPERTY    │                                                    VALUE                                                    │
├────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ Branch ID      │ br-12345                                                                                                    │
│ Name           │ test-branch                                                                                                 │
│ Parent Service │ test-service (svc-12345)                                                                                    │
│ Status         │ READY                                                                                                       │
│ CPU            │ 0.25 cores (250m)                                                                                           │
│ Memory         │ 2 GB                                                                                                        │
│ Created        │ 2025-01-16 09:00:00 UTC                                                                                     │
│ Console URL    │ https://console.cloud.tigerdata.com/projects/test-project-123/services/svc-12345/branches/br-12345/overview │
└────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
`,
			wantStderr: "Warning: failed to get connection details: endpoint port not available\n",
		},
		{
			// As for a service, a missing endpoint is warned about rather
			// than silently leaving out the connection details.
			name: "endpoint not available",
			args: []string{"branch", "get", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch(branchStatus(api.DeployStatusQUEUED), func(b *api.Branch) { b.Endpoint = nil }))
				expectGetService(m, "svc-12345", sampleService(fluidPlatform))
			},
			wantStdout: `┌────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│    PROPERTY    │                                                    VALUE                                                    │
├────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ Branch ID      │ br-12345                                                                                                    │
│ Name           │ test-branch                                                                                                 │
│ Parent Service │ test-service (svc-12345)                                                                                    │
│ Status         │ QUEUED                                                                                                      │
│ CPU            │ 0.25 cores (250m)                                                                                           │
│ Memory         │ 2 GB                                                                                                        │
│ Created        │ 2025-01-16 09:00:00 UTC                                                                                     │
│ Console URL    │ https://console.cloud.tigerdata.com/projects/test-project-123/services/svc-12345/branches/br-12345/overview │
└────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
`,
			wantStderr: "Warning: failed to get connection details: endpoint not available\n",
		},
		{
			name: "by name",
			args: []string{"branch", "get", "test-branch", "-o", "env"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "test-branch", sampleBranch())
				expectGetService(m, "svc-12345", sampleService(fluidPlatform))
			},
			wantStdout: sampleBranchEnv,
		},
		{
			name:       "describe alias",
			args:       []string{"branch", "describe", "br-12345", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       getSample,
			wantStdout: sampleBranchEnv,
		},
	})
}
