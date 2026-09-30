package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestAllowListGetCmd(t *testing.T) {
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")
	allowList := sampleAllowList()

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"allowlist", "get", "1234567890"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name: "network error",
			args: []string{"allowlist", "get", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get IP allow list: connection refused",
		},
		{
			name: "not found",
			args: []string{"allowlist", "get", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("IP allow list not found")},
					}, nil)
			},
			wantErr: "IP allow list not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"allowlist", "get", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{
						HTTPResponse: httpResponse(http.StatusOK),
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "table output",
			args: []string{"allowlist", "get", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowList,
					}, nil)
			},
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
			name: "json output",
			args: []string{"allowlist", "get", "1234567890", "-o", "json"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowList,
					}, nil)
			},
			wantStdout: "{\n" +
				"  \"allow_list_id\": \"1234567890\",\n" +
				"  \"cidr_blocks\": [\n" +
				"    \"203.0.113.0/24\"\n" +
				"  ],\n" +
				"  \"created_at\": \"2025-01-15T10:30:00Z\",\n" +
				"  \"description\": \"Office network\",\n" +
				"  \"project_id\": \"test-project-123\"\n" +
				"}\n",
		},
		{
			name: "yaml output",
			args: []string{"allowlist", "get", "1234567890", "-o", "yaml"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowList,
					}, nil)
			},
			wantStdout: "allow_list_id: \"1234567890\"\n" +
				"cidr_blocks:\n" +
				"  - 203.0.113.0/24\n" +
				"created_at: \"2025-01-15T10:30:00Z\"\n" +
				"description: Office network\n" +
				"project_id: test-project-123\n",
		},
		{
			name:    "env output flag rejected",
			args:    []string{"allowlist", "get", "1234567890", "-o", "env"},
			opts:    []runOption{experimental},
			wantErr: `invalid argument "env" for "-o, --output" flag: invalid output format: env (must be one of: json, yaml, table)`,
		},
		{
			name: "describe alias",
			args: []string{"allowlist", "describe", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowList,
					}, nil)
			},
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
			name: "show alias",
			args: []string{"allowlist", "show", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.GetAllowListResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &allowList,
					}, nil)
			},
			wantStdout: "┌─────────────┬──────────────────────┐\n" +
				"│  PROPERTY   │        VALUE         │\n" +
				"├─────────────┼──────────────────────┤\n" +
				"│ ID          │ 1234567890           │\n" +
				"│ Description │ Office network       │\n" +
				"│ CIDR Blocks │ 203.0.113.0/24       │\n" +
				"│ Created     │ 2025-01-15 10:30 UTC │\n" +
				"└─────────────┴──────────────────────┘\n",
		},
	})
}
