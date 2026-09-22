package mcp

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceBackups(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	// The tool is experimental-gated (see the first case), so every other
	// case registers it explicitly.
	experimental := withExperimental()

	expectBackups := func(backups *[]api.Backup) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetBackupsWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.GetBackupsResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      backups,
				}, nil)
		}
	}

	// A finished full backup and an incremental one still running, so the
	// output covers both the populated and the omitted optional fields.
	backups := []api.Backup{
		{
			Label:           "20260115-093000F",
			Type:            api.BackupTypeFULL,
			StartedAt:       time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC),
			FinishedAt:      new(time.Date(2026, 1, 15, 9, 41, 12, 0, time.UTC)),
			DurationSeconds: new(int64(672)),
			SizeBytes:       new(int64(4831838208)),
			Regions: []api.BackupRegionState{
				{RegionCode: "us-east-1", Status: new(api.BackupCopyStatusFINISHED)},
				{RegionCode: "eu-central-1", Status: new(api.BackupCopyStatusRUNNING)},
			},
		},
		{
			Label:     "20260115-093000F_20260116-093000I",
			Type:      api.BackupTypeINCREMENTAL,
			StartedAt: time.Date(2026, 1, 16, 9, 30, 0, 0, time.UTC),
			Regions:   []api.BackupRegionState{{RegionCode: "us-east-1"}},
		},
	}
	wantBackups := map[string]any{"backups": []any{
		map[string]any{
			"label":            "20260115-093000F",
			"type":             "FULL",
			"started_at":       "2026-01-15T09:30:00Z",
			"finished_at":      "2026-01-15T09:41:12Z",
			"duration_seconds": 672.0,
			"size_bytes":       4.831838208e+09,
			"regions": []any{
				map[string]any{"region_code": "us-east-1", "status": "FINISHED"},
				map[string]any{"region_code": "eu-central-1", "status": "RUNNING"},
			},
		},
		map[string]any{
			"label":      "20260115-093000F_20260116-093000I",
			"type":       "INCREMENTAL",
			"started_at": "2026-01-16T09:30:00Z",
			"regions":    []any{map[string]any{"region_code": "us-east-1"}},
		},
	}}
	noBackups := map[string]any{"backups": []any{}}

	runToolTests(t, []toolTest{
		{
			// The tool is gated at registration, so without the experimental
			// gate the server never advertises it and the SDK refuses the call
			// itself — a transport error rather than a result.
			name:        "not registered without the experimental gate",
			tool:        toolServiceBackups,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "service_backups"`,
		},
		{
			name:    "not logged in",
			tool:    toolServiceBackups,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceBackups,
			args:    map[string]any{"service_id": "NOPE"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "NOPE" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "missing service ID",
			tool:    toolServiceBackups,
			args:    map[string]any{},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: required: missing properties: ["service_id"]`,
		},
		{
			name: "network error",
			tool: toolServiceBackups,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBackupsWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list backups: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceBackups,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBackupsWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetBackupsResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "API error without a message body",
			tool: toolServiceBackups,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBackupsWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetBackupsResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
		},
		{
			// A 200 with no parsed body is reported as no backups, not an error.
			name:       "nil response body",
			tool:       toolServiceBackups,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectBackups(nil),
			wantOutput: noBackups,
		},
		{
			// A JSON `null` array is normalized to an empty one, so the output
			// stays a valid array.
			name:       "null backups array",
			tool:       toolServiceBackups,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectBackups(new([]api.Backup)),
			wantOutput: noBackups,
		},
		{
			name:       "no backups taken yet",
			tool:       toolServiceBackups,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectBackups(&[]api.Backup{}),
			wantOutput: noBackups,
		},
		{
			name:       "backups listed",
			tool:       toolServiceBackups,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectBackups(&backups),
			wantOutput: wantBackups,
		},
	})
}
