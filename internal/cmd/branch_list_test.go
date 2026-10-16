package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchListCmd(t *testing.T) {
	// The API lists every branch in the project, including other services'.
	other := sampleBranch(func(b *api.Branch) {
		b.ID = "br-99999"
		b.Name = "other-branch"
		b.ServiceID = "svc-99999"
	})
	paused := sampleBranch(func(b *api.Branch) {
		b.ID = "br-67890"
		b.Name = "feature/user-auth"
		b.Status = api.DeployStatusPAUSED
	})
	expectList := func(branches ...api.Branch) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, "svc-12345", sampleService())
			m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, nil).
				Return(&api.GetBranchesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &branches,
				}, nil)
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"branch", "list", "svc-12345"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing service id",
			args:    []string{"branch", "list"},
			opts:    []runOption{withExperimental()},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "service not found",
			args: []string{"branch", "list", "svc-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefNotFound(m, "svc-12345")
			},
			wantErr: "service 'svc-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "network error",
			args: []string{"branch", "list", "svc-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, nil).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list branches: connection refused",
		},
		{
			name: "API error",
			args: []string{"branch", "list", "svc-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, nil).
					Return(&api.GetBranchesResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSON4XX:      &api.Error{Message: new("forbidden")},
					}, nil)
			},
			wantErr: "forbidden",
			checks:  []checkFunc{checkExitCode(common.ExitPermissionDenied)},
		},
		{
			name: "nil response body",
			args: []string{"branch", "list", "svc-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, nil).
					Return(&api.GetBranchesResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "no branches",
			args:       []string{"branch", "list", "svc-12345"},
			opts:       []runOption{withExperimental()},
			mock:       expectList(other),
			wantStderr: "No branches found for service 'test-service' (svc-12345).\n",
		},
		{
			name: "table shows only the service's branches",
			args: []string{"branch", "list", "svc-12345"},
			opts: []runOption{withExperimental()},
			mock: expectList(sampleBranch(), other, paused),
			wantStdout: `┌───────────┬───────────────────┬────────┬──────────────────┐
│ BRANCH ID │       NAME        │ STATUS │     CREATED      │
├───────────┼───────────────────┼────────┼──────────────────┤
│ br-12345  │ test-branch       │ READY  │ 2025-01-16 09:00 │
│ br-67890  │ feature/user-auth │ PAUSED │ 2025-01-16 09:00 │
└───────────┴───────────────────┴────────┴──────────────────┘
`,
		},
		{
			name: "json",
			args: []string{"branch", "list", "svc-12345", "-o", "json"},
			opts: []runOption{withExperimental()},
			mock: expectList(sampleBranch(), other),
			wantStdout: `[
  {
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
    "connection_string": "postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require",
    "console_url": "https://console.cloud.tigerdata.com/dashboard/services/svc-12345/branches/br-12345"
  }
]
`,
		},
		{
			name: "default service from config",
			args: []string{"branch", "list"},
			opts: []runOption{withExperimental(), withConfig(map[string]any{"service_id": "svc-12345"})},
			mock: expectList(sampleBranch()),
			wantStdout: `┌───────────┬─────────────┬────────┬──────────────────┐
│ BRANCH ID │    NAME     │ STATUS │     CREATED      │
├───────────┼─────────────┼────────┼──────────────────┤
│ br-12345  │ test-branch │ READY  │ 2025-01-16 09:00 │
└───────────┴─────────────┴────────┴──────────────────┘
`,
		},
	})
}
