package mcp

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceBackupRegionAddTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "region_code": "eu-central-1"}

	// The tool is experimental-gated (see the first case), so every other
	// case registers it explicitly.
	experimental := withExperimental()

	region := api.BackupRegion{
		RegionCode: "eu-central-1",
		Created:    new(time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)),
	}
	wantOutput := map[string]any{
		"region": map[string]any{
			"region_code": "eu-central-1",
			"created":     "2026-01-15T09:30:00Z",
		},
		"message": "Backups for service 'e6ue9697jf' will now be copied to 'eu-central-1'.",
	}
	expectAdd := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "e6ue9697jf", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
			Return(&api.CreateBackupRegionResponse{
				HTTPResponse: httpResponse(http.StatusCreated),
				JSON201:      &region,
			}, nil)
	}

	runToolTests(t, []toolTest{
		{
			// The tool is gated at registration, so without the experimental
			// gate the server never advertises it and the SDK refuses the call
			// itself — a transport error rather than a result.
			name:        "not registered without the experimental gate",
			tool:        toolServiceBackupRegionAdd,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "service_backup_region_add"`,
		},
		{
			name:    "not logged in",
			tool:    toolServiceBackupRegionAdd,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceBackupRegionAdd,
			args:    map[string]any{"service_id": "NOPE", "region_code": "eu-central-1"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "NOPE" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "missing region code",
			tool:    toolServiceBackupRegionAdd,
			args:    map[string]any{"service_id": "e6ue9697jf"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: required: missing properties: ["region_code"]`,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceBackupRegionAdd,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// Only the tag lookup is registered: an attempted add fails as an
			// unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceBackupRegionAdd,
			args:    args,
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name: "read-only prod allows DEV service",
			tool: toolServiceBackupRegionAdd,
			args: args,
			opts: []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				expectAdd(m)
			},
			wantOutput: wantOutput,
		},
		{
			name: "network error",
			tool: toolServiceBackupRegionAdd,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "e6ue9697jf", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to add backup region: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceBackupRegionAdd,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "e6ue9697jf", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
					Return(&api.CreateBackupRegionResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "nil response body",
			tool: toolServiceBackupRegionAdd,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateBackupRegionWithResponse(validCtx, testProjectID, "e6ue9697jf", api.BackupRegionCreate{RegionCode: "eu-central-1"}).
					Return(&api.CreateBackupRegionResponse{HTTPResponse: httpResponse(http.StatusCreated)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "region added",
			tool:       toolServiceBackupRegionAdd,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectAdd,
			wantOutput: wantOutput,
		},
	})
}
