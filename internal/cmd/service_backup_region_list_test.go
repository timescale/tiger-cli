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

	created := time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)
	regions := []api.BackupRegion{
		{RegionCode: "eu-central-1", Created: &created},
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
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"service", "backup", "region", "list", "svc-12345"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBackupRegionsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list backup regions: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "backup", "region", "list", "svc-12345"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
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
			setup: func(m *mocks.MockClientWithResponsesInterface) {
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
			setup:      setupList([]api.BackupRegion{}),
			wantStderr: "No backup regions configured for this service.\n",
		},
		{
			name:       "table output",
			args:       []string{"service", "backup", "region", "list", "svc-12345"},
			opts:       []runOption{experimental},
			setup:      setupList(regions),
			wantStdout: regionsTable,
		},
		{
			name:       "default service id from config",
			args:       []string{"service", "backup", "region", "list"},
			opts:       []runOption{experimental, withConfig(map[string]any{"service_id": "svc-12345"})},
			setup:      setupList(regions),
			wantStdout: regionsTable,
		},
		{
			name:  "json output",
			args:  []string{"service", "backup", "region", "list", "svc-12345", "-o", "json"},
			opts:  []runOption{experimental},
			setup: setupList(regions),
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
			name:  "yaml output",
			args:  []string{"service", "backup", "region", "list", "svc-12345", "-o", "yaml"},
			opts:  []runOption{experimental},
			setup: setupList(regions),
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
			setup:   setupList(regions),
			wantErr: "environment variable output is not supported for backup regions",
		},
	})
}
