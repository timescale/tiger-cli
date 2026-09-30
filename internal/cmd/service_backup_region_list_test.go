package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceBackupRegionListCmd(t *testing.T) {
	// The command is experimental-gated (see the gate test in service_test.go),
	// so every case registers it explicitly.
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	regions := []api.BackupRegion{
		{RegionCode: "eu-central-1", Created: new(time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC))},
		{RegionCode: "us-west-2"},
	}

	const regionsTable = `┌──────────────┬──────────────────────┐
│    REGION    │        ADDED         │
├──────────────┼──────────────────────┤
│ eu-central-1 │ 2026-01-15 09:30 UTC │
│ us-west-2    │                      │
└──────────────┴──────────────────────┘
`

	setupList := func(regions []api.BackupRegion) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRefID(m, "svc-12345")
			m.EXPECT().GetBackupRegionsWithResponse(validCtx, testProjectID, "svc-12345").
				Return(&api.GetBackupRegionsResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &regions,
				}, nil)
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "backup", "region", "list", "svc-12345"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing service id",
			args:    []string{"service", "backup", "region", "list"},
			opts:    []runOption{experimental},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "ambiguous name refused",
			args: []string{"service", "backup", "region", "list", "my-api-db"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "network error",
			args: []string{"service", "backup", "region", "list", "svc-12345"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetBackupRegionsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list backup regions: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "backup", "region", "list", "svc-12345"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetBackupRegionsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetBackupRegionsResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"service", "backup", "region", "list", "svc-12345"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().GetBackupRegionsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetBackupRegionsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "empty list",
			args:       []string{"service", "backup", "region", "list", "svc-12345"},
			opts:       []runOption{experimental},
			mock:       setupList([]api.BackupRegion{}),
			wantStdout: "No backup regions configured for this service.\n",
		},
		{
			// An empty list still serializes as a valid array in structured
			// formats, not the table's human-readable message.
			name:       "empty list json output",
			args:       []string{"service", "backup", "region", "list", "svc-12345", "-o", "json"},
			opts:       []runOption{experimental},
			mock:       setupList([]api.BackupRegion{}),
			wantStdout: "[]\n",
		},
		{
			name:       "empty list yaml output",
			args:       []string{"service", "backup", "region", "list", "svc-12345", "-o", "yaml"},
			opts:       []runOption{experimental},
			mock:       setupList([]api.BackupRegion{}),
			wantStdout: "[]\n",
		},
		{
			name:       "table output",
			args:       []string{"service", "backup", "region", "list", "svc-12345"},
			opts:       []runOption{experimental},
			mock:       setupList(regions),
			wantStdout: regionsTable,
		},
		{
			name:       "ls alias",
			args:       []string{"service", "backup", "region", "ls", "svc-12345"},
			opts:       []runOption{experimental},
			mock:       setupList(regions),
			wantStdout: regionsTable,
		},
		{
			name:       "default service id from config",
			args:       []string{"service", "backup", "region", "list"},
			opts:       []runOption{experimental, withConfig(map[string]any{"service_id": "svc-12345"})},
			mock:       setupList(regions),
			wantStdout: regionsTable,
		},
		{
			name: "json output",
			args: []string{"service", "backup", "region", "list", "svc-12345", "-o", "json"},
			opts: []runOption{experimental},
			mock: setupList(regions),
			wantStdout: `[
  {
    "created": "2026-01-15T09:30:00Z",
    "region_code": "eu-central-1"
  },
  {
    "region_code": "us-west-2"
  }
]
`,
		},
		{
			name: "yaml output",
			args: []string{"service", "backup", "region", "list", "svc-12345", "-o", "yaml"},
			opts: []runOption{experimental},
			mock: setupList(regions),
			wantStdout: `- created: "2026-01-15T09:30:00Z"
  region_code: eu-central-1
- region_code: us-west-2
`,
		},
		{
			name:    "env output rejected by flag",
			args:    []string{"service", "backup", "region", "list", "svc-12345", "-o", "env"},
			opts:    []runOption{experimental},
			wantErr: `invalid argument "env" for "-o, --output" flag: invalid output format: env (must be one of: json, yaml, table)`,
		},
		{
			// The flag rejects env at parse time, but a hand-edited config file
			// can still reach outputBackupRegions' env branch.
			name:    "env output from config file",
			args:    []string{"service", "backup", "region", "list", "svc-12345"},
			opts:    []runOption{experimental, withConfig(map[string]any{"output": "env"})},
			mock:    setupList(regions),
			wantErr: "environment variable output is not supported for backup regions",
		},
	})
}
