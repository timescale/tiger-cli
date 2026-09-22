package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceDeleteCmd(t *testing.T) {
	// overrides apply to the resolved service, whose tag the prod gate reads.
	setupDelete := func(overrides ...func(*api.Service)) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, "svc-12345", sampleService(overrides...))
			m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
				Return(&api.DeleteServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
				}, nil)
		}
	}

	confirmPrompt := "Are you sure you want to delete service 'test-service' (svc-12345)? This operation cannot be undone.\n" +
		"Type the service ID 'svc-12345' to confirm: "

	runCmdTests(t, []cmdTest{
		{
			// No fallback to the configured default service ID for deletes.
			name:    "missing service id",
			args:    []string{"service", "delete"},
			opts:    []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: "service is required",
		},
		{
			// An empty argument is a missing service, not a ref to resolve.
			name:    "empty service",
			args:    []string{"service", "delete", ""},
			wantErr: "service is required",
		},
		{
			name:    "not logged in",
			args:    []string{"service", "delete", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only all refuses",
			args:    []string{"service", "delete", "svc-12345"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// prod judges the service by its environment tag, so the gate
			// fetches it. Only the tag lookup is registered: an attempted
			// mutation fails as an unexpected call.
			name:    "read-only prod refuses PROD service",
			args:    []string{"service", "delete", "svc-12345"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			setup:   expectTaggedService("PROD"),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			args:       []string{"service", "delete", "svc-12345", "--confirm"},
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			setup:      setupDelete(envTag("DEV")),
			wantStderr: "Service 'svc-12345' deleted.\n",
		},
		{
			name:    "non-TTY without confirm",
			args:    []string{"service", "delete", "svc-12345"},
			setup:   func(m *mocks.MockClientWithResponsesInterface) { expectResolveRef(m, "svc-12345", sampleService()) },
			wantErr: "TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt",
		},
		{
			name:       "confirmation mismatch",
			args:       []string{"service", "delete", "svc-12345"},
			opts:       []runOption{withIsTerminal(true), withStdin("svc-other\n")},
			setup:      func(m *mocks.MockClientWithResponsesInterface) { expectResolveRef(m, "svc-12345", sampleService()) },
			wantStderr: confirmPrompt + "Delete operation cancelled.\n",
		},
		{
			name:  "confirmation match",
			args:  []string{"service", "delete", "svc-12345"},
			opts:  []runOption{withIsTerminal(true), withStdin("svc-12345\n")},
			setup: setupDelete(),
			wantStderr: confirmPrompt +
				"Service 'svc-12345' deleted.\n",
		},
		{
			// The prompt takes the ID and nothing else: typing back the name
			// that was used to reach the service does not authorize the
			// delete, since a name can move to a different service through a
			// rename.
			name: "name typed back at the prompt does not confirm",
			args: []string{"service", "delete", "test-service"},
			opts: []runOption{withIsTerminal(true), withStdin("test-service\n")},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			wantStderr: confirmPrompt + "Delete operation cancelled.\n",
		},
		{
			// The same name ref goes through once the ID is typed.
			name: "name ref confirmed with the resolved ID",
			args: []string{"service", "delete", "test-service"},
			opts: []runOption{withIsTerminal(true), withStdin("svc-12345\n")},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
				m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.DeleteServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantStderr: confirmPrompt + "Service 'svc-12345' deleted.\n",
		},
		{
			name:       "confirm flag skips prompt",
			args:       []string{"service", "delete", "svc-12345", "--confirm"},
			setup:      setupDelete(),
			wantStderr: "Service 'svc-12345' deleted.\n",
		},
		{
			name: "network error",
			args: []string{"service", "delete", "svc-12345", "--confirm"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345")
				m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to delete Service: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "delete", "svc-12345", "--confirm"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345")
				m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.DeleteServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name:       "rm alias",
			args:       []string{"service", "rm", "svc-12345", "--confirm"},
			setup:      setupDelete(),
			wantStderr: "Service 'svc-12345' deleted.\n",
		},
	})
}
