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

	region := api.BackupRegion{RegionCode: "eu-central-1", Created: new(time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC))}

	// overrides apply to the resolved service, whose tag the prod gate reads.
	setupAdd := func(overrides ...func(*api.Service)) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, "svc-12345", sampleService(overrides...))
			m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
				Return(&api.CreateBackupRegionResponse{
					HTTPResponse: httpResponse(http.StatusCreated),
					JSON201:      &region,
				}, nil)
		}
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
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
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
			mock:    expectTaggedService("PROD"),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			args:       []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupAdd(envTag("DEV")),
			wantStderr: "Backups for service 'test-service' (svc-12345) will now be copied to 'eu-central-1'.\n",
			wantStdout: `┌──────────────┬──────────────────────┐
│    REGION    │        ADDED         │
├──────────────┼──────────────────────┤
│ eu-central-1 │ 2026-01-15 09:30 UTC │
└──────────────┴──────────────────────┘
`,
		},
		{
			name: "ambiguous name refused",
			args: []string{"service", "backup", "region", "add", "my-api-db", "--region", "eu-central-1"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "network error",
			args: []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to add backup region: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
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
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
					Return(&api.CreateBackupRegionResponse{
						HTTPResponse: httpResponse(http.StatusCreated),
						JSON201:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "table output",
			args:       []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts:       []runOption{experimental},
			mock:       setupAdd(),
			wantStderr: "Backups for service 'test-service' (svc-12345) will now be copied to 'eu-central-1'.\n",
			wantStdout: `┌──────────────┬──────────────────────┐
│    REGION    │        ADDED         │
├──────────────┼──────────────────────┤
│ eu-central-1 │ 2026-01-15 09:30 UTC │
└──────────────┴──────────────────────┘
`,
		},
		{
			name:       "default service id from config",
			args:       []string{"service", "backup", "region", "add", "--region", "eu-central-1"},
			opts:       []runOption{experimental, withConfig(map[string]any{"service_id": "svc-12345"})},
			mock:       setupAdd(),
			wantStderr: "Backups for service 'test-service' (svc-12345) will now be copied to 'eu-central-1'.\n",
			wantStdout: `┌──────────────┬──────────────────────┐
│    REGION    │        ADDED         │
├──────────────┼──────────────────────┤
│ eu-central-1 │ 2026-01-15 09:30 UTC │
└──────────────┴──────────────────────┘
`,
		},
		{
			name:       "json output",
			args:       []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1", "-o", "json"},
			opts:       []runOption{experimental},
			mock:       setupAdd(),
			wantStderr: "Backups for service 'test-service' (svc-12345) will now be copied to 'eu-central-1'.\n",
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
			mock:       setupAdd(),
			wantStderr: "Backups for service 'test-service' (svc-12345) will now be copied to 'eu-central-1'.\n",
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
		{
			// The flag rejects env at parse time, but a hand-edited config file
			// can still reach the RunE's env branch.
			name:    "env output from config file",
			args:    []string{"service", "backup", "region", "add", "svc-12345", "--region", "eu-central-1"},
			opts:    []runOption{experimental, withConfig(map[string]any{"output": "env"})},
			mock:    setupAdd(),
			wantErr: "environment variable output is not supported for backup regions",
			wantStderr: "Backups for service 'test-service' (svc-12345) will now be copied to 'eu-central-1'.\n" +
				"Error: environment variable output is not supported for backup regions\n",
		},
	})
}
