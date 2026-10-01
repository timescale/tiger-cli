package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestAllowListCreateCmd(t *testing.T) {
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	baseArgs := []string{"allowlist", "create", "--description", "Office network", "--cidr", "203.0.113.0/24"}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    baseArgs,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing required flags",
			args:    []string{"allowlist", "create"},
			opts:    []runOption{experimental},
			wantErr: `required flag(s) "cidr", "description" not set`,
		},
		{
			name:    "missing cidr",
			args:    []string{"allowlist", "create", "--description", "Office network"},
			opts:    []runOption{experimental},
			wantErr: `required flag(s) "cidr" not set`,
		},
		{
			name:    "read-only all refuses",
			args:    baseArgs,
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name: "network error",
			args: baseArgs,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateAllowListWithResponse(validCtx, testProjectID, api.AllowListCreate{
					Description: "Office network",
					CidrBlocks:  []string{"203.0.113.0/24"},
				}).Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create IP allow list: connection refused",
		},
		{
			name: "API error",
			args: baseArgs,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateAllowListWithResponse(validCtx, testProjectID, api.AllowListCreate{
					Description: "Office network",
					CidrBlocks:  []string{"203.0.113.0/24"},
				}).Return(&api.CreateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusBadRequest),
					JSON4XX:      &api.ClientError{Message: new("cidr_blocks: block too small, minimum /17")},
				}, nil)
			},
			wantErr: "cidr_blocks: block too small, minimum /17",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "nil response body",
			args: baseArgs,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateAllowListWithResponse(validCtx, testProjectID, api.AllowListCreate{
					Description: "Office network",
					CidrBlocks:  []string{"203.0.113.0/24"},
				}).Return(&api.CreateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusCreated),
				}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "table output",
			args: baseArgs,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				allowList := sampleAllowList()
				m.EXPECT().CreateAllowListWithResponse(validCtx, testProjectID, api.AllowListCreate{
					Description: "Office network",
					CidrBlocks:  []string{"203.0.113.0/24"},
				}).Return(&api.CreateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusCreated),
					JSON201:      &allowList,
				}, nil)
			},
			wantStderr: "IP allow list 'Office network' created.\n",
			wantStdout: "┌─────────────┬──────────────────────┐\n" +
				"│  PROPERTY   │        VALUE         │\n" +
				"├─────────────┼──────────────────────┤\n" +
				"│ ID          │ 1234567890           │\n" +
				"│ Description │ Office network       │\n" +
				"│ CIDR Blocks │ 203.0.113.0/24       │\n" +
				"│ Created     │ 2025-01-15 10:30 UTC │\n" +
				"└─────────────┴──────────────────────┘\n",
		},
		{
			name: "multiple cidr blocks",
			args: []string{"allowlist", "create", "--description", "VPN ranges", "--cidr", "203.0.113.0/24", "--cidr", "198.51.100.0/24", "-o", "json"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				allowList := sampleAllowList(func(a *api.AllowList) {
					a.Description = "VPN ranges"
					a.CidrBlocks = []string{"203.0.113.0/24", "198.51.100.0/24"}
				})
				m.EXPECT().CreateAllowListWithResponse(validCtx, testProjectID, api.AllowListCreate{
					Description: "VPN ranges",
					CidrBlocks:  []string{"203.0.113.0/24", "198.51.100.0/24"},
				}).Return(&api.CreateAllowListResponse{
					HTTPResponse: httpResponse(http.StatusCreated),
					JSON201:      &allowList,
				}, nil)
			},
			wantStderr: "IP allow list 'VPN ranges' created.\n",
			wantStdout: "{\n" +
				"  \"allow_list_id\": \"1234567890\",\n" +
				"  \"cidr_blocks\": [\n" +
				"    \"203.0.113.0/24\",\n" +
				"    \"198.51.100.0/24\"\n" +
				"  ],\n" +
				"  \"created_at\": \"2025-01-15T10:30:00Z\",\n" +
				"  \"description\": \"VPN ranges\",\n" +
				"  \"project_id\": \"test-project-123\"\n" +
				"}\n",
		},
	})
}
