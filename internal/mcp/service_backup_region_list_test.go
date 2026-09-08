package mcp

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceBackupRegionListTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	// The tool is experimental-gated (see the first case), so every other
	// case registers it explicitly.
	experimental := withExperimental()

	expectRegions := func(regions *[]api.BackupRegion) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetBackupRegionsWithResponse(validCtx, testProjectID, "e6ue9697jf").
				Return(&api.GetBackupRegionsResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      regions,
				}, nil)
		}
	}

	regions := []api.BackupRegion{
		{
			RegionCode: "eu-central-1",
			Created:    new(time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)),
		},
		{RegionCode: "ap-southeast-1"},
	}
	wantRegions := map[string]any{"regions": []any{
		map[string]any{"region_code": "eu-central-1", "created": "2026-01-15T09:30:00Z"},
		map[string]any{"region_code": "ap-southeast-1"},
	}}
	noRegions := map[string]any{"regions": []any{}}

	runToolTests(t, []toolTest{
		{
			// The tool is gated at registration, so without the experimental
			// gate the server never advertises it and the SDK refuses the call
			// itself — a transport error rather than a result.
			name:        "not registered without the experimental gate",
			tool:        toolServiceBackupRegionList,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "service_backup_region_list"`,
		},
		{
			name:    "not logged in",
			tool:    toolServiceBackupRegionList,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceBackupRegionList,
			args:    map[string]any{"service_id": "NOPE"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "NOPE" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "missing service ID",
			tool:    toolServiceBackupRegionList,
			args:    map[string]any{},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: required: missing properties: ["service_id"]`,
		},
		{
			name: "network error",
			tool: toolServiceBackupRegionList,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBackupRegionsWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list backup regions: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceBackupRegionList,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetBackupRegionsWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetBackupRegionsResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name:    "nil response body",
			tool:    toolServiceBackupRegionList,
			args:    args,
			opts:    []runOption{experimental},
			mock:    expectRegions(nil),
			wantErr: "empty response from API",
		},
		{
			name:       "no backup regions configured",
			tool:       toolServiceBackupRegionList,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectRegions(&[]api.BackupRegion{}),
			wantOutput: noRegions,
		},
		{
			name:       "backup regions listed",
			tool:       toolServiceBackupRegionList,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectRegions(&regions),
			wantOutput: wantRegions,
		},
	})
}
