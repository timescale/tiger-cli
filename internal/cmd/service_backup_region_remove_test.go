package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceBackupRegionRemoveCmd(t *testing.T) {
	// The command is experimental-gated (see the gate test in service_test.go),
	// so every case registers it explicitly.
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	setupRemove := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().DeleteBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", "eu-central-1").
			Return(&api.DeleteBackupRegionResponse{
				HTTPResponse: httpResponse(http.StatusNoContent),
			}, nil)
	}

	confirmPrompt := "Are you sure you want to stop copying service 'svc-12345' backups to 'eu-central-1'? " +
		"Existing copies there will be deleted and cannot be recovered.\n" +
		"Type the service ID 'svc-12345' to confirm: "

	runCmdTests(t, []cmdTest{
		{
			// No fallback to the configured default service ID for removals.
			name:    "missing service id",
			args:    []string{"service", "backup", "region", "remove", "--region", "eu-central-1"},
			opts:    []runOption{experimental, withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: `accepts 1 arg(s), received 0`,
		},
		{
			name:    "missing region flag",
			args:    []string{"service", "backup", "region", "remove", "svc-12345"},
			opts:    []runOption{experimental},
			wantErr: `required flag(s) "region" not set`,
		},
		{
			name:    "not logged in",
			args:    []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only all refuses",
			args:    []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1"},
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "read-only prod refuses PROD service",
			args:    []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1"},
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			setup:   expectTaggedService("PROD"),
			wantErr: `service svc-12345: this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name: "read-only prod allows DEV service",
			args: []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1", "--confirm"},
			opts: []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV")(m)
				setupRemove(m)
			},
			wantStderr: "✅ Backups for service 'svc-12345' will no longer be copied to 'eu-central-1'.\n",
		},
		{
			name:    "non-TTY without confirm",
			args:    []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1"},
			opts:    []runOption{experimental},
			wantErr: "TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt",
		},
		{
			name:       "confirmation mismatch",
			args:       []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1"},
			opts:       []runOption{experimental, withIsTerminal(true), withStdin("svc-other\n")},
			wantStderr: confirmPrompt + "❌ Remove operation cancelled.\n",
		},
		{
			name:       "confirmation match",
			args:       []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1"},
			opts:       []runOption{experimental, withIsTerminal(true), withStdin("svc-12345\n")},
			setup:      setupRemove,
			wantStderr: confirmPrompt + "✅ Backups for service 'svc-12345' will no longer be copied to 'eu-central-1'.\n",
		},
		{
			name:       "confirm flag skips prompt",
			args:       []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1", "--confirm"},
			opts:       []runOption{experimental},
			setup:      setupRemove,
			wantStderr: "✅ Backups for service 'svc-12345' will no longer be copied to 'eu-central-1'.\n",
		},
		{
			name: "network error",
			args: []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1", "--confirm"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().DeleteBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", "eu-central-1").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to remove backup region: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1", "--confirm"},
			opts: []runOption{experimental},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().DeleteBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", "eu-central-1").
					Return(&api.DeleteBackupRegionResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
	})
}
