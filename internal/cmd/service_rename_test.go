package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceRenameCmd(t *testing.T) {
	renamed := sampleService(func(s *api.Service) { s.Name = "analytics-prod" })

	// overrides apply to the resolved service, whose tag the prod gate reads.
	expectRename := func(overrides ...func(*api.Service)) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, "svc-12345", sampleService(overrides...))
			m.EXPECT().RenameServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ServiceRename{Name: "analytics-prod"}).
				Return(&api.RenameServiceResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &renamed,
				}, nil)
		}
	}

	const renamedMsg = "Renamed service 'test-service' (svc-12345) to 'analytics-prod'.\n"

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "rename", "svc-12345", "analytics-prod"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			// Both arguments are required: a lone argument can't say whether it
			// names the service or the new name.
			name:    "missing new name",
			args:    []string{"service", "rename", "svc-12345"},
			wantErr: "accepts 2 arg(s), received 1",
		},
		{
			// An empty argument is a missing service, not a ref to resolve.
			name:    "empty service",
			args:    []string{"service", "rename", "", "analytics-prod"},
			wantErr: "service name or ID is required",
		},
		{
			name:    "empty new name",
			args:    []string{"service", "rename", "svc-12345", "   "},
			wantErr: "new name cannot be empty",
		},
		{
			name:    "read-only all refuses",
			args:    []string{"service", "rename", "svc-12345", "analytics-prod"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// prod judges the service by its environment tag, so the gate
			// fetches it. Only the tag lookup is registered: an attempted
			// rename fails as an unexpected call.
			name:    "read-only prod refuses PROD service",
			args:    []string{"service", "rename", "svc-12345", "analytics-prod"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD"),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			args:       []string{"service", "rename", "svc-12345", "analytics-prod"},
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectRename(envTag("DEV")),
			wantStdout: renamedMsg,
		},
		{
			name: "ambiguous name refused",
			args: []string{"service", "rename", "my-api-db", "new-name"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "network error",
			args: []string{"service", "rename", "svc-12345", "analytics-prod"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().RenameServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ServiceRename{Name: "analytics-prod"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to rename service: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "rename", "svc-12345", "analytics-prod"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().RenameServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ServiceRename{Name: "analytics-prod"}).
					Return(&api.RenameServiceResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.Error{Message: new("invalid request")},
					}, nil)
			},
			wantErr: "invalid request",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "nil response body",
			args: []string{"service", "rename", "svc-12345", "analytics-prod"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().RenameServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ServiceRename{Name: "analytics-prod"}).
					Return(&api.RenameServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "renames the service",
			args:       []string{"service", "rename", "svc-12345", "analytics-prod"},
			mock:       expectRename(),
			wantStdout: renamedMsg,
		},
		{
			// The new name is trimmed before it is sent, so the mock's exact
			// request match is what proves the trimming happened.
			name:       "trims the new name",
			args:       []string{"service", "rename", "svc-12345", "  analytics-prod  "},
			mock:       expectRename(),
			wantStdout: renamedMsg,
		},
	})
}
