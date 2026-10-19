package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchGetCmd(t *testing.T) {
	getSample := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveBranch(m, "br-12345", sampleBranch())
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
console_url: https://console.cloud.tigerdata.com/dashboard/services/svc-12345/branches/br-12345
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
status: READY
`,
		},
		{
			name:       "env",
			args:       []string{"branch", "get", "br-12345", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       getSample,
			wantStdout: sampleBranchEnv,
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
  "role": "tsdbadmin",
  "host": "br-12345.test-project-123.branch.tsdb.cloud.timescale.com",
  "port": 36956,
  "database": "tsdb",
  "connection_string": "postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require\u0026options=-c%20tsdb_admin.read_only_connection%3Dtrue",
  "console_url": "https://console.cloud.tigerdata.com/dashboard/services/svc-12345/branches/br-12345"
}
`,
		},
		{
			name:       "by name",
			args:       []string{"branch", "get", "test-branch", "-o", "env"},
			opts:       []runOption{withExperimental()},
			mock:       func(m *mocks.MockClientWithResponsesInterface) { expectResolveBranch(m, "test-branch", sampleBranch()) },
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
