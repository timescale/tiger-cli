package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestAllowListUpdateCmd(t *testing.T) {
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"allowlist", "update", "1234567890", "--description", "New name"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing allow list id",
			args:    []string{"allowlist", "update"},
			opts:    []runOption{experimental},
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "read-only all refuses",
			args:    []string{"allowlist", "update", "1234567890", "--description", "New name"},
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "no fields to change",
			args:    []string{"allowlist", "update", "1234567890"},
			opts:    []runOption{experimental},
			wantErr: "at least one of --description or --cidr is required",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "network error",
			args: []string{"allowlist", "update", "1234567890", "--description", "New name"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					Description: new("New name"),
				}).Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to update IP allow list: connection refused",
		},
		{
			name: "API error",
			args: []string{"allowlist", "update", "1234567890", "--description", "New name"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					Description: new("New name"),
				}).Return(&api.UpdateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusNotFound),
					JSON4XX:      &api.ClientError{Message: new("IP allow list not found")},
				}, nil)
			},
			wantErr: "IP allow list not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"allowlist", "update", "1234567890", "--description", "New name"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					Description: new("New name"),
				}).Return(&api.UpdateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusOK),
				}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "rename only",
			args: []string{"allowlist", "update", "1234567890", "--description", "New name"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				allowList := sampleAllowList(func(a *api.AllowList) { a.Description = "New name" })
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					Description: new("New name"),
				}).Return(&api.UpdateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &allowList,
				}, nil)
			},
			wantStderr: "IP allow list '1234567890' updated.\n",
			wantStdout: "┌─────────────┬──────────────────────┐\n" +
				"│  PROPERTY   │        VALUE         │\n" +
				"├─────────────┼──────────────────────┤\n" +
				"│ ID          │ 1234567890           │\n" +
				"│ Description │ New name             │\n" +
				"│ CIDR Blocks │ 203.0.113.0/24       │\n" +
				"│ Created     │ 2025-01-15 10:30 UTC │\n" +
				"└─────────────┴──────────────────────┘\n",
		},
		{
			name: "cidr only",
			args: []string{"allowlist", "update", "1234567890", "--cidr", "198.51.100.0/24"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				allowList := sampleAllowList(func(a *api.AllowList) { a.CidrBlocks = []string{"198.51.100.0/24"} })
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					CidrBlocks: &[]string{"198.51.100.0/24"},
				}).Return(&api.UpdateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &allowList,
				}, nil)
			},
			wantStderr: "IP allow list '1234567890' updated.\n",
			wantStdout: "┌─────────────┬──────────────────────┐\n" +
				"│  PROPERTY   │        VALUE         │\n" +
				"├─────────────┼──────────────────────┤\n" +
				"│ ID          │ 1234567890           │\n" +
				"│ Description │ Office network       │\n" +
				"│ CIDR Blocks │ 198.51.100.0/24      │\n" +
				"│ Created     │ 2025-01-15 10:30 UTC │\n" +
				"└─────────────┴──────────────────────┘\n",
		},
		{
			name: "description and cidr, json output",
			args: []string{"allowlist", "update", "1234567890", "--description", "New name", "--cidr", "198.51.100.0/24", "--cidr", "203.0.113.0/24", "-o", "json"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				allowList := sampleAllowList(func(a *api.AllowList) {
					a.Description = "New name"
					a.CidrBlocks = []string{"198.51.100.0/24", "203.0.113.0/24"}
				})
				m.EXPECT().UpdateAllowListWithResponse(validCtx, testProjectID, "1234567890", api.AllowListUpdate{
					Description: new("New name"),
					CidrBlocks:  &[]string{"198.51.100.0/24", "203.0.113.0/24"},
				}).Return(&api.UpdateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &allowList,
				}, nil)
			},
			wantStderr: "IP allow list '1234567890' updated.\n",
			wantStdout: "{\n" +
				"  \"allow_list_id\": \"1234567890\",\n" +
				"  \"cidr_blocks\": [\n" +
				"    \"198.51.100.0/24\",\n" +
				"    \"203.0.113.0/24\"\n" +
				"  ],\n" +
				"  \"created_at\": \"2025-01-15T10:30:00Z\",\n" +
				"  \"description\": \"New name\",\n" +
				"  \"project_id\": \"test-project-123\"\n" +
				"}\n",
		},
	})
}
