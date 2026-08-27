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

func TestServiceBackupsCmd(t *testing.T) {
	// The command is experimental-gated (see the gate test in service_test.go),
	// so every case registers it explicitly.
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	// Four backups covering the formatting matrix: full/incremental, finished/
	// still running (absent finished_at/duration/size), second/minute/hour
	// durations, byte/mebibyte/gibibyte sizes, and regions with and without a
	// reported copy status.
	started := time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)
	finished := time.Date(2026, 1, 15, 9, 41, 12, 0, time.UTC)
	backups := []api.Backup{
		{
			Label:           "20260115-093000F",
			Type:            api.BackupTypeFULL,
			StartedAt:       started,
			FinishedAt:      &finished,
			DurationSeconds: new(int64(672)),
			SizeBytes:       new(int64(4831838208)),
			Regions: []api.BackupRegionState{
				{RegionCode: "us-east-1", Status: new(api.BackupCopyStatusFINISHED)},
				{RegionCode: "eu-central-1", Status: new(api.BackupCopyStatusFAILED)},
			},
		},
		{
			// Still running: no finish time, duration, or size. No per-region
			// status: the backend does not always report one.
			Label:     "20260115-093000F_20260116-100000I",
			Type:      api.BackupTypeINCREMENTAL,
			StartedAt: started.Add(24 * time.Hour),
			Regions:   []api.BackupRegionState{{RegionCode: "us-east-1"}},
		},
		{
			Label:           "20260117-093000F",
			Type:            api.BackupTypeFULL,
			StartedAt:       started.Add(48 * time.Hour),
			DurationSeconds: new(int64(2)),
			SizeBytes:       new(int64(512)),
			Regions: []api.BackupRegionState{
				{RegionCode: "us-east-1", Status: new(api.BackupCopyStatusFINISHED)},
			},
		},
		{
			Label:           "20260117-093000F_20260118-100000I",
			Type:            api.BackupTypeINCREMENTAL,
			StartedAt:       started.Add(72 * time.Hour),
			DurationSeconds: new(int64(7325)),
			SizeBytes:       new(int64(53_300_000)),
			Regions: []api.BackupRegionState{
				{RegionCode: "us-east-1", Status: new(api.BackupCopyStatusRUNNING)},
			},
		},
	}

	// Expected table for the four sample backups (rendered with withUTC).
	const backupsTable = `┌──────────────────────┬─────────────┬──────────┬──────────┬─────────────────────────────────────────────┐
│       STARTED        │    TYPE     │ DURATION │   SIZE   │                   REGIONS                   │
├──────────────────────┼─────────────┼──────────┼──────────┼─────────────────────────────────────────────┤
│ 2026-01-15 09:30 UTC │ FULL        │ 11m12s   │ 4.5GiB   │ us-east-1 (FINISHED), eu-central-1 (FAILED) │
│ 2026-01-16 09:30 UTC │ INCREMENTAL │          │          │ us-east-1                                   │
│ 2026-01-17 09:30 UTC │ FULL        │ 2s       │ 512B     │ us-east-1 (FINISHED)                        │
│ 2026-01-18 09:30 UTC │ INCREMENTAL │ 2h2m5s   │ 50.83MiB │ us-east-1 (RUNNING)                         │
└──────────────────────┴─────────────┴──────────┴──────────┴─────────────────────────────────────────────┘
`

	setupList := func(backups []api.Backup) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetBackupsWithResponse(validCtx, testProjectID, "svc-12345").
				Return(&api.GetBackupsResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &backups,
				}, nil)
		}
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "backup", "svc-12345"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing service id",
			args:    []string{"service", "backup"},
			opts:    []runOption{experimental},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"service", "backup", "svc-12345"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBackupsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list backups: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "backup", "svc-12345"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBackupsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetBackupsResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"service", "backup", "svc-12345"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBackupsWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetBackupsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "empty list",
			args:       []string{"service", "backup", "svc-12345"},
			opts:       []runOption{experimental},
			setup:      setupList([]api.Backup{}),
			wantStderr: "No backups found for this service yet.\n",
		},
		{
			// The label is omitted from the table: it repeats STARTED and TYPE,
			// and no command takes it as input.
			name:       "table output",
			args:       []string{"service", "backup", "svc-12345"},
			opts:       []runOption{experimental, withUTC()},
			setup:      setupList(backups),
			wantStdout: backupsTable,
		},
		{
			name:       "default service id from config",
			args:       []string{"service", "backup"},
			opts:       []runOption{experimental, withUTC(), withConfig(map[string]any{"service_id": "svc-12345"})},
			setup:      setupList(backups),
			wantStdout: backupsTable,
		},
		{
			// The label stays in the structured formats.
			name:  "json output",
			args:  []string{"service", "backup", "svc-12345", "-o", "json"},
			opts:  []runOption{experimental},
			setup: setupList(backups[:2]),
			wantStdout: `[
  {
    "duration_seconds": 672,
    "finished_at": "2026-01-15T09:41:12Z",
    "label": "20260115-093000F",
    "regions": [
      {
        "region_code": "us-east-1",
        "status": "FINISHED"
      },
      {
        "region_code": "eu-central-1",
        "status": "FAILED"
      }
    ],
    "size_bytes": 4831838208,
    "started_at": "2026-01-15T09:30:00Z",
    "type": "FULL"
  },
  {
    "label": "20260115-093000F_20260116-100000I",
    "regions": [
      {
        "region_code": "us-east-1"
      }
    ],
    "started_at": "2026-01-16T09:30:00Z",
    "type": "INCREMENTAL"
  }
]
`,
		},
		{
			// size_bytes renders in scientific notation: SerializeToYAML
			// round-trips through JSON, so large integers become float64s.
			name:  "yaml output",
			args:  []string{"service", "backup", "svc-12345", "-o", "yaml"},
			opts:  []runOption{experimental},
			setup: setupList(backups[:2]),
			wantStdout: `- duration_seconds: 672
  finished_at: "2026-01-15T09:41:12Z"
  label: 20260115-093000F
  regions:
    - region_code: us-east-1
      status: FINISHED
    - region_code: eu-central-1
      status: FAILED
  size_bytes: 4.831838208e+09
  started_at: "2026-01-15T09:30:00Z"
  type: FULL
- label: 20260115-093000F_20260116-100000I
  regions:
    - region_code: us-east-1
  started_at: "2026-01-16T09:30:00Z"
  type: INCREMENTAL
`,
		},
		{
			name:    "env output rejected by flag",
			args:    []string{"service", "backup", "svc-12345", "-o", "env"},
			opts:    []runOption{experimental},
			wantErr: `invalid argument "env" for "-o, --output" flag: invalid output format: env (must be one of: json, yaml, table)`,
		},
		{
			// The flag rejects env at parse time, but a hand-edited config file
			// can still reach outputBackups' env branch.
			name:    "env output from config file",
			args:    []string{"service", "backup", "svc-12345"},
			opts:    []runOption{experimental, withConfig(map[string]any{"output": "env"})},
			setup:   setupList(backups[:2]),
			wantErr: "environment variable output is not supported for backups",
		},
	}

	runCmdTests(t, tests)
}
