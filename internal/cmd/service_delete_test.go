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
	// ref is what the command was given, which resolves to svc-12345; overrides
	// apply to the resolved service, whose tag the prod gate reads.
	setupDelete := func(ref string, overrides ...func(*api.Service)) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, ref, sampleService(overrides...))
			m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
				Return(&api.DeleteServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
				}, nil)
		}
	}

	// setupFluidDelete is setupDelete for a fluid service, which can have
	// branches, so the prompt lists them first.
	setupFluidDelete := func(branches ...api.Branch) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			setupDelete("svc-12345", fluidPlatform)(m)
			m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "svc-12345", nil).
				Return(&api.GetBranchesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &branches,
				}, nil)
		}
	}
	otherBranch := sampleBranch(func(b *api.Branch) {
		b.ID = "br-67890"
		b.Name = "other-branch"
	})

	confirmPrompt := "Are you sure you want to delete service 'test-service' (svc-12345)? This operation cannot be undone.\n" +
		"Type the service ID 'svc-12345' to confirm: "

	runCmdTests(t, []cmdTest{
		{
			// No fallback to the configured default service ID for deletes.
			name:    "missing service id",
			args:    []string{"service", "delete"},
			opts:    []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: "service name or ID is required",
		},
		{
			// An empty argument is a missing service, not a ref to resolve.
			name:    "empty service",
			args:    []string{"service", "delete", ""},
			wantErr: "service name or ID is required",
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
			mock:    expectTaggedService("PROD"),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			args:       []string{"service", "delete", "svc-12345", "--confirm"},
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupDelete("svc-12345", envTag("DEV")),
			wantStderr: "Service 'test-service' (svc-12345) deleted.\n",
		},
		{
			name: "ambiguous name refused",
			args: []string{"service", "delete", "my-api-db", "--confirm"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:    "non-TTY without confirm",
			args:    []string{"service", "delete", "svc-12345"},
			mock:    func(m *mocks.MockClientWithResponsesInterface) { expectResolveRef(m, "svc-12345", sampleService()) },
			wantErr: "TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt",
		},
		{
			name:       "confirmation mismatch",
			args:       []string{"service", "delete", "svc-12345"},
			opts:       []runOption{withIsTerminal(true), withStdin("svc-other\n")},
			mock:       func(m *mocks.MockClientWithResponsesInterface) { expectResolveRef(m, "svc-12345", sampleService()) },
			wantStderr: confirmPrompt + "Delete operation cancelled.\n",
		},
		{
			name: "confirmation match",
			args: []string{"service", "delete", "svc-12345"},
			opts: []runOption{withIsTerminal(true), withStdin("svc-12345\n")},
			mock: setupDelete("svc-12345"),
			wantStderr: confirmPrompt +
				"Service 'test-service' (svc-12345) deleted.\n",
		},
		{
			// Typing back the name used to reach the service does not confirm
			// the delete: the prompt takes only the ID.
			name: "name typed back at the prompt does not confirm",
			args: []string{"service", "delete", "test-service"},
			opts: []runOption{withIsTerminal(true), withStdin("test-service\n")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			wantStderr: confirmPrompt + "Delete operation cancelled.\n",
		},
		{
			// The same name ref goes through once the ID is typed.
			name:       "name ref confirmed with the resolved ID",
			args:       []string{"service", "delete", "test-service"},
			opts:       []runOption{withIsTerminal(true), withStdin("svc-12345\n")},
			mock:       setupDelete("test-service"),
			wantStderr: confirmPrompt + "Service 'test-service' (svc-12345) deleted.\n",
		},
		{
			// The API deletes a service's branches along with it, so they're
			// listed before the prompt and after the delete, even without the
			// experimental gate.
			name: "confirmation lists the branches deleted with the service",
			args: []string{"service", "delete", "svc-12345"},
			opts: []runOption{withIsTerminal(true), withStdin("svc-12345\n")},
			mock: setupFluidDelete(sampleBranch(), otherBranch),
			wantStderr: "This service has 2 branches, which will also be deleted:\n" +
				"  'test-branch' (br-12345)\n" +
				"  'other-branch' (br-67890)\n" +
				confirmPrompt +
				"Service 'test-service' (svc-12345) deleted, along with its 2 branches:\n" +
				"  'test-branch' (br-12345)\n" +
				"  'other-branch' (br-67890)\n",
		},
		{
			name: "confirmation lists a single branch",
			args: []string{"service", "delete", "svc-12345"},
			opts: []runOption{withIsTerminal(true), withStdin("svc-12345\n")},
			mock: setupFluidDelete(sampleBranch()),
			wantStderr: "This service has 1 branch, which will also be deleted:\n" +
				"  'test-branch' (br-12345)\n" +
				confirmPrompt +
				"Service 'test-service' (svc-12345) deleted, along with its 1 branch:\n" +
				"  'test-branch' (br-12345)\n",
		},
		{
			name:       "fluid service without branches lists none",
			args:       []string{"service", "delete", "svc-12345"},
			opts:       []runOption{withIsTerminal(true), withStdin("svc-12345\n")},
			mock:       setupFluidDelete(),
			wantStderr: confirmPrompt + "Service 'test-service' (svc-12345) deleted.\n",
		},
		{
			// The listing only informs the decision, so a failure still lets
			// the user decide.
			name: "branch listing fails",
			args: []string{"service", "delete", "svc-12345"},
			opts: []runOption{withIsTerminal(true), withStdin("svc-12345\n")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				setupDelete("svc-12345", fluidPlatform)(m)
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "svc-12345", nil).
					Return(nil, errors.New("connection refused"))
			},
			wantStderr: "Warning: failed to list branches: failed to call API: connection refused\n" +
				"Any branches of this service will also be deleted.\n" +
				confirmPrompt +
				"Service 'test-service' (svc-12345) deleted.\n",
		},
		{
			// Without a prompt, the branches are still listed once the service
			// is deleted.
			name: "confirm flag still lists the deleted branches",
			args: []string{"service", "delete", "svc-12345", "--confirm"},
			mock: setupFluidDelete(sampleBranch(), otherBranch),
			wantStderr: "Service 'test-service' (svc-12345) deleted, along with its 2 branches:\n" +
				"  'test-branch' (br-12345)\n" +
				"  'other-branch' (br-67890)\n",
		},
		{
			name:       "confirm flag skips prompt",
			args:       []string{"service", "delete", "svc-12345", "--confirm"},
			mock:       setupDelete("svc-12345"),
			wantStderr: "Service 'test-service' (svc-12345) deleted.\n",
		},
		{
			name: "network error",
			args: []string{"service", "delete", "svc-12345", "--confirm"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to call API: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "delete", "svc-12345", "--confirm"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
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
			mock:       setupDelete("svc-12345"),
			wantStderr: "Service 'test-service' (svc-12345) deleted.\n",
		},
		{
			// Branches are a preview, so the help mentions them only behind
			// the gate.
			name:       "help mentions branches when experimental",
			args:       []string{"service", "delete", "--help"},
			opts:       []runOption{withExperimental()},
			wantStdout: matchHelp("Deleting a service also deletes all of its branches."),
		},
		{
			name:       "help omits branches by default",
			args:       []string{"service", "delete", "--help"},
			wantStdout: matchHelp("Delete a database service permanently.", "branch"),
		},
	})
}
