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

// sampleAllowList returns an api.AllowList with reasonable defaults. Use
// overrides to customize specific fields.
func sampleAllowList(overrides ...func(*api.AllowList)) api.AllowList {
	al := api.AllowList{
		AllowListID: "1234567890",
		ProjectID:   testProjectID,
		Description: "Office network",
		CidrBlocks:  []string{"203.0.113.0/24"},
		CreatedAt:   time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	for _, o := range overrides {
		o(&al)
	}
	return al
}

func TestAllowListListCmd(t *testing.T) {
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	allowLists := []api.AllowList{
		sampleAllowList(),
		sampleAllowList(func(a *api.AllowList) {
			a.AllowListID = "9876543210"
			a.Description = "VPN ranges"
			a.CidrBlocks = []string{"198.51.100.0/24", "203.0.113.0/24"}
			a.CreatedAt = time.Date(2025, 2, 20, 14, 0, 0, 0, time.UTC)
		}),
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"allowlist", "list"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name: "network error",
			args: []string{"allowlist", "list"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list IP allow lists: connection refused",
		},
		{
			name: "API error",
			args: []string{"allowlist", "list"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSON4XX:      &api.ClientError{Message: new("access denied")},
					}, nil)
			},
			wantErr: "access denied",
			checks:  []checkFunc{checkExitCode(common.ExitPermissionDenied)},
		},
		{
			name: "nil response body",
			args: []string{"allowlist", "list"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "empty list",
			args: []string{"allowlist", "list"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				empty := []api.AllowList{}
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &empty,
					}, nil)
			},
			wantStderr: "No IP allow lists found for this project.\n",
		},
		{
			name: "table output",
			args: []string{"allowlist", "list"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowLists,
					}, nil)
			},
			wantStdout: "┌────────────┬────────────────┬─────────────────────────────────┬──────────────────────┐\n" +
				"│     ID     │  DESCRIPTION   │           CIDR BLOCKS           │       CREATED        │\n" +
				"├────────────┼────────────────┼─────────────────────────────────┼──────────────────────┤\n" +
				"│ 1234567890 │ Office network │ 203.0.113.0/24                  │ 2025-01-15 10:30 UTC │\n" +
				"│ 9876543210 │ VPN ranges     │ 198.51.100.0/24, 203.0.113.0/24 │ 2025-02-20 14:00 UTC │\n" +
				"└────────────┴────────────────┴─────────────────────────────────┴──────────────────────┘\n",
		},
		{
			name: "ls alias",
			args: []string{"allowlist", "ls"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowLists,
					}, nil)
			},
			wantStdout: "┌────────────┬────────────────┬─────────────────────────────────┬──────────────────────┐\n" +
				"│     ID     │  DESCRIPTION   │           CIDR BLOCKS           │       CREATED        │\n" +
				"├────────────┼────────────────┼─────────────────────────────────┼──────────────────────┤\n" +
				"│ 1234567890 │ Office network │ 203.0.113.0/24                  │ 2025-01-15 10:30 UTC │\n" +
				"│ 9876543210 │ VPN ranges     │ 198.51.100.0/24, 203.0.113.0/24 │ 2025-02-20 14:00 UTC │\n" +
				"└────────────┴────────────────┴─────────────────────────────────┴──────────────────────┘\n",
		},
		{
			name: "json output",
			args: []string{"allowlist", "list", "-o", "json"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowLists,
					}, nil)
			},
			wantStdout: "[\n" +
				"  {\n" +
				"    \"allow_list_id\": \"1234567890\",\n" +
				"    \"cidr_blocks\": [\n" +
				"      \"203.0.113.0/24\"\n" +
				"    ],\n" +
				"    \"created_at\": \"2025-01-15T10:30:00Z\",\n" +
				"    \"description\": \"Office network\",\n" +
				"    \"project_id\": \"test-project-123\"\n" +
				"  },\n" +
				"  {\n" +
				"    \"allow_list_id\": \"9876543210\",\n" +
				"    \"cidr_blocks\": [\n" +
				"      \"198.51.100.0/24\",\n" +
				"      \"203.0.113.0/24\"\n" +
				"    ],\n" +
				"    \"created_at\": \"2025-02-20T14:00:00Z\",\n" +
				"    \"description\": \"VPN ranges\",\n" +
				"    \"project_id\": \"test-project-123\"\n" +
				"  }\n" +
				"]\n",
		},
		{
			name: "yaml output",
			args: []string{"allowlist", "list", "-o", "yaml"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowLists,
					}, nil)
			},
			wantStdout: "- allow_list_id: \"1234567890\"\n" +
				"  cidr_blocks:\n" +
				"    - 203.0.113.0/24\n" +
				"  created_at: \"2025-01-15T10:30:00Z\"\n" +
				"  description: Office network\n" +
				"  project_id: test-project-123\n" +
				"- allow_list_id: \"9876543210\"\n" +
				"  cidr_blocks:\n" +
				"    - 198.51.100.0/24\n" +
				"    - 203.0.113.0/24\n" +
				"  created_at: \"2025-02-20T14:00:00Z\"\n" +
				"  description: VPN ranges\n" +
				"  project_id: test-project-123\n",
		},
		{
			// "env" is rejected at flag-parse time for list, but can still
			// reach the output switch via a hand-edited config file.
			name: "env output format from config",
			args: []string{"allowlist", "list"},
			opts: []runOption{experimental, withConfig(map[string]any{"output": "env"})},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListsWithResponse(validCtx, testProjectID).
					Return(&api.GetAllowListsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowLists,
					}, nil)
			},
			wantErr: "environment variable output is not supported for IP allow lists",
		},
		{
			name:    "env output flag rejected",
			args:    []string{"allowlist", "list", "-o", "env"},
			opts:    []runOption{experimental},
			wantErr: `invalid argument "env" for "-o, --output" flag: invalid output format: env (must be one of: json, yaml, table)`,
		},
	})
}
