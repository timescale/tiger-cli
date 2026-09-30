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

	// ref is what the command was given, which resolves to svc-12345; overrides
	// apply to the resolved service, whose tag the prod gate reads.
	setupRemove := func(ref string, overrides ...func(*api.Service)) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, ref, sampleService(overrides...))
			m.EXPECT().DeleteBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", "eu-central-1").
				Return(&api.DeleteBackupRegionResponse{
					HTTPResponse: httpResponse(http.StatusNoContent),
				}, nil)
		}
	}

	confirmPrompt := "Are you sure you want to stop copying backups of service 'test-service' (svc-12345) to 'eu-central-1'? " +
		"Backup copies in that region will be deleted.\n" +
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
			// An empty argument is a missing service, not a ref to resolve.
			name:    "empty service",
			args:    []string{"service", "backup", "region", "remove", "", "--region", "eu-central-1"},
			opts:    []runOption{experimental},
			wantErr: "service name or ID is required",
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
			mock:    expectTaggedService("PROD"),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			args:       []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1", "--confirm"},
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupRemove("svc-12345", envTag("DEV")),
			wantStderr: "Backups for service 'test-service' (svc-12345) will no longer be copied to 'eu-central-1'.\n",
		},
		{
			name: "ambiguous name refused",
			args: []string{"service", "backup", "region", "remove", "my-api-db", "--region", "eu-central-1", "--confirm"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "non-TTY without confirm",
			args:    []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1"},
			opts:    []runOption{experimental},
			mock:    func(m *mocks.MockClientWithResponsesInterface) { expectResolveRef(m, "svc-12345", sampleService()) },
			wantErr: "TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt",
		},
		{
			name:       "confirmation mismatch",
			args:       []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1"},
			opts:       []runOption{experimental, withIsTerminal(true), withStdin("svc-other\n")},
			mock:       func(m *mocks.MockClientWithResponsesInterface) { expectResolveRef(m, "svc-12345", sampleService()) },
			wantStderr: confirmPrompt + "Remove operation cancelled.\n",
		},
		{
			name:       "confirmation match",
			args:       []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1"},
			opts:       []runOption{experimental, withIsTerminal(true), withStdin("svc-12345\n")},
			mock:       setupRemove("svc-12345"),
			wantStderr: confirmPrompt + "Backups for service 'test-service' (svc-12345) will no longer be copied to 'eu-central-1'.\n",
		},
		{
			// Typing back the name used to reach the service does not confirm
			// the removal: the prompt takes only the ID.
			name: "name typed back at the prompt does not confirm",
			args: []string{"service", "backup", "region", "remove", "test-service", "--region", "eu-central-1"},
			opts: []runOption{experimental, withIsTerminal(true), withStdin("test-service\n")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			wantStderr: confirmPrompt + "Remove operation cancelled.\n",
		},
		{
			name:       "name ref confirmed with the resolved ID",
			args:       []string{"service", "backup", "region", "remove", "test-service", "--region", "eu-central-1"},
			opts:       []runOption{experimental, withIsTerminal(true), withStdin("svc-12345\n")},
			mock:       setupRemove("test-service"),
			wantStderr: confirmPrompt + "Backups for service 'test-service' (svc-12345) will no longer be copied to 'eu-central-1'.\n",
		},
		{
			name:       "confirm flag skips prompt",
			args:       []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1", "--confirm"},
			opts:       []runOption{experimental},
			mock:       setupRemove("svc-12345"),
			wantStderr: "Backups for service 'test-service' (svc-12345) will no longer be copied to 'eu-central-1'.\n",
		},
		{
			name:       "rm alias",
			args:       []string{"service", "backup", "region", "rm", "svc-12345", "--region", "eu-central-1", "--confirm"},
			opts:       []runOption{experimental},
			mock:       setupRemove("svc-12345"),
			wantStderr: "Backups for service 'test-service' (svc-12345) will no longer be copied to 'eu-central-1'.\n",
		},
		{
			name: "network error",
			args: []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1", "--confirm"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().DeleteBackupRegionWithResponse(validCtx, testProjectID, "svc-12345", "eu-central-1").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to remove backup region: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "backup", "region", "remove", "svc-12345", "--region", "eu-central-1", "--confirm"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
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
