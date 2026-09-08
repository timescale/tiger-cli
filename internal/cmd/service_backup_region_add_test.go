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

func TestServiceBackupRegionAddCmd(t *testing.T) {
	// The command is experimental-gated (see the gate test in service_test.go),
	// so every case registers it explicitly.
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	created := time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)
	region := api.BackupRegion{RegionCode: "eu-central-1", Created: &created}

	setupAdd := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
			Return(&api.CreateBackupRegionResponse{
				HTTPResponse: httpResponse(http.StatusCreated),
				JSON201:      &region,
			}, nil)
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing service id",
			args:    []string{"service", "backup", "region", "add", "--region", "eu-central-1"},
			opts:    []runOption{experimental},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "missing region flag",
			args:    []string{"service", "backup", "region", "add", "svc-12345"},
			opts:    []runOption{experimental},
			wantErr: `required flag(s) "region" not set`,
		},
		{
			name:    "read-only all refuses",
			args:    []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "read-only prod refuses PROD service",
			args:    []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			setup:   expectTaggedService("PROD"),
			wantErr: `service svc-12345: this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name: "read-only prod allows DEV service",
			args: []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts: []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV")(m)
				setupAdd(m)
			},
			wantStderr: "✅ Backups for service 'svc-12345' will now be copied to 'eu-central-1'.\n",
		},
		{
			name: "network error",
			args: []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to add backup region: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
					Return(&api.CreateBackupRegionResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
					Return(&api.CreateBackupRegionResponse{
						HTTPResponse: httpResponse(http.StatusCreated),
						JSON201:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// Table output is intentionally omitted: the stderr confirmation
			// already covers it.
			name:       "table output",
			args:       []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts:       []runOption{experimental},
			setup:      setupAdd,
			wantStderr: "✅ Backups for service 'svc-12345' will now be copied to 'eu-central-1'.\n",
		},
		{
			name:       "default service id from config",
			args:       []string{"service", "backup", "region", "add", "--region", "eu-central-1"},
			opts:       []runOption{experimental, withConfig(map[string]any{"service_id": "svc-12345"})},
			setup:      setupAdd,
			wantStderr: "✅ Backups for service 'svc-12345' will now be copied to 'eu-central-1'.\n",
		},
		{
			name:       "json output",
			args:       []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1", "-o", "json"},
			opts:       []runOption{experimental},
			setup:      setupAdd,
			wantStderr: "✅ Backups for service 'svc-12345' will now be copied to 'eu-central-1'.\n",
			wantStdout: `[
  {
    "created": "2026-01-15T09:30:00Z",
    "region_code": "eu-central-1"
  }
]
`,
		},
		{
			name:       "yaml output",
			args:       []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1", "-o", "yaml"},
			opts:       []runOption{experimental},
			setup:      setupAdd,
			wantStderr: "✅ Backups for service 'svc-12345' will now be copied to 'eu-central-1'.\n",
			wantStdout: `- created: "2026-01-15T09:30:00Z"
  region_code: eu-central-1
`,
		},
		{
			name:    "env output rejected by flag",
			args:    []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1", "-o", "env"},
			opts:    []runOption{experimental},
			wantErr: `invalid argument "env" for "-o, --output" flag: invalid output format: env (must be one of: json, yaml, table)`,
		},
	})
}
