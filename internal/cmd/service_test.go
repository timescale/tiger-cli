package cmd

import (
	"net/http"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceCommandAliases(t *testing.T) {
	emptyList := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
			Return(&api.GetServicesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &[]api.Service{},
			}, nil)
	}

	runCmdTests(t, []cmdTest{
		{
			name:       "service",
			args:       []string{"service", "list"},
			setup:      emptyList,
			wantStderr: noServicesStderr,
		},
		{
			name:       "services alias",
			args:       []string{"services", "list"},
			setup:      emptyList,
			wantStderr: noServicesStderr,
		},
		{
			name:       "svc alias",
			args:       []string{"svc", "list"},
			setup:      emptyList,
			wantStderr: noServicesStderr,
		},
	})
}

// TestServiceExperimentalGate covers the registration gate for the preview
// subtrees (`service metrics`, `service backup`): buildServiceCmd only adds
// them when the experimental env var is truthy, so by default the commands
// don't exist in the tree at all. Help output is asserted loosely
// (containment) rather than exactly, so the gate test doesn't break every time
// an unrelated subcommand's help text changes.
func TestServiceExperimentalGate(t *testing.T) {
	// matchHelp asserts the output is a help text that mentions want and none
	// of the gated names in absent.
	matchHelp := func(want string, absent ...string) matcher {
		return matchFunc(func(t *testing.T, got string) {
			if !strings.Contains(got, want) {
				t.Errorf("expected help output containing %q, got:\n%s", want, got)
			}
			for _, name := range absent {
				if strings.Contains(got, name) {
					t.Errorf("help must not mention %s when experimental is off, got:\n%s", name, got)
				}
			}
		})
	}

	runCmdTests(t, []cmdTest{
		{
			// Cobra only reports "unknown command" at the root level; for a
			// non-root group it treats the unknown name as a stray argument
			// and prints the group's help. The gate is observable as the
			// absence of any gated entry in that help.
			name:       "unregistered by default",
			args:       []string{"service", "metrics"},
			wantStdout: matchHelp("Available Commands:", "metrics", "backup"),
		},
		{
			name:       "metrics registered when experimental",
			args:       []string{"service", "metrics", "--help"},
			opts:       []runOption{withEnv("TIGER_EXPERIMENTAL", "true")},
			wantStdout: matchHelp("Commands for querying time-series metrics"),
		},
		{
			name:       "backup registered when experimental",
			args:       []string{"service", "backup", "--help"},
			opts:       []runOption{withEnv("TIGER_EXPERIMENTAL", "true")},
			wantStdout: matchHelp("List the full and incremental backups"),
		},
	})
}

// checkStoredPassword returns a check asserting the password stored in the
// (mock) keyring for the given service.
func checkStoredPassword(serviceID, want string) checkFunc {
	return func(t *testing.T, result cmdResult) {
		t.Helper()
		svc := api.Service{ServiceID: serviceID, ProjectID: testProjectID}
		got, err := (&common.KeyringStorage{}).Get(svc, "tsdbadmin")
		if err != nil {
			t.Fatalf("failed to read stored password: %v", err)
		}
		if got != want {
			t.Errorf("stored password = %q, want %q", got, want)
		}
	}
}

// notRefusedForDEV asserts an error isn't one of the read-only refusals. The DEV
// half of the prod-mode cases only needs to prove the gate let the command reach
// the API; what the API then says is that command's own test.
var notRefusedForDEV = matchFunc(func(t *testing.T, got string) {
	t.Helper()
	for _, refusal := range []string{"read-only mode", "read_only"} {
		if strings.Contains(got, refusal) {
			t.Errorf("command was refused for a DEV service: %s", got)
		}
	}
})

// expectTaggedService registers the tag lookup a ...ByServiceID gate makes (and,
// for update-password, the fetch the command was making anyway).
func expectTaggedService(tag string) func(*mocks.MockClientWithResponsesInterface) {
	return func(m *mocks.MockClientWithResponsesInterface) {
		expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
			s.Metadata = &api.ServiceMetadata{Environment: &tag}
		}))
	}
}

// TestDestructiveCommands_ReadOnly checks that read_only=all refuses every
// destructive command. No expectations are registered, so any API call the gate
// failed to prevent fails the case as an unexpected call.
func TestDestructiveCommands_ReadOnly(t *testing.T) {
	readOnly := []runOption{withConfig(map[string]any{"read_only": "all"})}
	refused := common.ErrReadOnly.Error()

	runCmdTests(t, []cmdTest{
		{
			name:    "create",
			args:    []string{"service", "create", "--addons", "none", "--region", "us-east-1", "--cpu", "1000", "--memory", "4"},
			opts:    readOnly,
			wantErr: refused,
		},
		{name: "fork", args: []string{"service", "fork", "svc-12345", "--now"}, opts: readOnly, wantErr: refused},
		{name: "start", args: []string{"service", "start", "svc-12345"}, opts: readOnly, wantErr: refused},
		{name: "stop", args: []string{"service", "stop", "svc-12345"}, opts: readOnly, wantErr: refused},
		{
			name:    "resize",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "1000", "--memory", "4"},
			opts:    readOnly,
			wantErr: refused,
		},
		{
			name:    "update-password",
			args:    []string{"service", "update-password", "svc-12345", "--auto-generate"},
			opts:    readOnly,
			wantErr: refused,
		},
		{name: "delete", args: []string{"service", "delete", "svc-12345", "--confirm"}, opts: readOnly, wantErr: refused},
	})
}

// TestDestructiveCommands_ReadOnlyProd checks that read_only=prod refuses a
// destructive command against a PROD service and lets it through against a DEV
// one. The PROD cases register only the tag lookup, so an attempted mutation
// fails as an unexpected call; the DEV cases register the mutation, so gomock
// fails the case if the gate swallowed it.
func TestDestructiveCommands_ReadOnlyProd(t *testing.T) {
	prod := []runOption{withConfig(map[string]any{"read_only": "prod"})}
	// CheckReadOnlyByServiceID names the service it refused; update-password
	// gates on an already-fetched service and doesn't.
	refusedByID := `service svc-12345: ` + common.ErrReadOnlyProd.Error()

	serverError := httpResponse(http.StatusInternalServerError)

	runCmdTests(t, []cmdTest{
		{
			name:    "start refuses PROD",
			args:    []string{"service", "start", "svc-12345"},
			setup:   expectTaggedService("PROD"),
			opts:    prod,
			wantErr: refusedByID,
		},
		{
			name: "start allows DEV",
			args: []string{"service", "start", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV")(m)
				m.EXPECT().StartServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.StartServiceResponse{HTTPResponse: serverError}, nil)
			},
			opts:    prod,
			wantErr: notRefusedForDEV,
		},
		{
			name:    "stop refuses PROD",
			args:    []string{"service", "stop", "svc-12345"},
			setup:   expectTaggedService("PROD"),
			opts:    prod,
			wantErr: refusedByID,
		},
		{
			name: "stop allows DEV",
			args: []string{"service", "stop", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV")(m)
				m.EXPECT().StopServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.StopServiceResponse{HTTPResponse: serverError}, nil)
			},
			opts:    prod,
			wantErr: notRefusedForDEV,
		},
		{
			name:    "resize refuses PROD",
			args:    []string{"service", "resize", "svc-12345", "--cpu", "1000", "--memory", "4"},
			setup:   expectTaggedService("PROD"),
			opts:    prod,
			wantErr: refusedByID,
		},
		{
			name: "resize allows DEV",
			args: []string{"service", "resize", "svc-12345", "--cpu", "1000", "--memory", "4"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV")(m)
				m.EXPECT().ResizeServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ResizeInput{CPUMillis: "1000", MemoryGbs: "4"}).
					Return(&api.ResizeServiceResponse{HTTPResponse: serverError}, nil)
			},
			opts:    prod,
			wantErr: notRefusedForDEV,
		},
		{
			name:    "delete refuses PROD",
			args:    []string{"service", "delete", "svc-12345", "--confirm"},
			setup:   expectTaggedService("PROD"),
			opts:    prod,
			wantErr: refusedByID,
		},
		{
			name: "delete allows DEV",
			args: []string{"service", "delete", "svc-12345", "--confirm"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV")(m)
				m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.DeleteServiceResponse{HTTPResponse: serverError}, nil)
			},
			opts:    prod,
			wantErr: notRefusedForDEV,
		},
		{
			name:    "update-password refuses PROD",
			args:    []string{"service", "update-password", "svc-12345", "--auto-generate"},
			setup:   expectTaggedService("PROD"),
			opts:    prod,
			wantErr: common.ErrReadOnlyProd.Error(),
		},
		{
			name: "update-password allows DEV",
			args: []string{"service", "update-password", "svc-12345", "--auto-generate"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV")(m)
				m.EXPECT().UpdatePasswordWithResponse(validCtx, testProjectID, "svc-12345", gomock.Any()).
					Return(&api.UpdatePasswordResponse{HTTPResponse: serverError}, nil)
			},
			opts:    prod,
			wantErr: notRefusedForDEV,
		},
	})
}

// TestCreateFork_ReadOnlyProd checks that read_only=prod gates service create and
// fork on the tag they request rather than on any existing service, so creating
// DEV is allowed and creating PROD is not.
func TestCreateFork_ReadOnlyProd(t *testing.T) {
	prod := []runOption{withConfig(map[string]any{"read_only": "prod"})}
	serverError := httpResponse(http.StatusInternalServerError)

	runCmdTests(t, []cmdTest{
		{
			name:    "create PROD is refused",
			args:    []string{"service", "create", "--addons", "none", "--environment", "PROD"},
			opts:    prod,
			wantErr: common.ErrReadOnlyProd.Error(),
		},
		{
			name: "create DEV is allowed",
			args: []string{"service", "create", "--addons", "none", "--environment", "DEV"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, gomock.Any()).
					Return(&api.CreateServiceResponse{HTTPResponse: serverError}, nil)
			},
			opts:    prod,
			wantErr: notRefusedForDEV,
		},
		{
			// Forking a PROD source into a DEV fork reads production without
			// changing it, so only the fork's own tag is gated.
			name:    "fork to PROD is refused",
			args:    []string{"service", "fork", "svc-12345", "--now", "--environment", "PROD"},
			opts:    prod,
			wantErr: common.ErrReadOnlyProd.Error(),
		},
		{
			name: "fork to DEV is allowed",
			args: []string{"service", "fork", "svc-12345", "--now", "--environment", "DEV"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "svc-12345", gomock.Any()).
					Return(&api.ForkServiceResponse{HTTPResponse: serverError}, nil)
			},
			opts:    prod,
			wantErr: notRefusedForDEV,
		},
	})
}
