package cmd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

// TestServiceCmd covers the group command itself, which shares groupRun and
// groupArgs with every other group command.
func TestServiceCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "no subcommand prints help without loading",
			args:       []string{"service"},
			wantStdout: matchHelp("Manage database services within Tiger Cloud platform."),
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:    "unknown subcommand suggests close matches",
			args:    []string{"service", "lsit"},
			wantErr: "unknown command \"lsit\" for \"tiger service\"\n\nDid you mean this?\n\tlist",
		},
		{
			name:    "unknown subcommand with no close match",
			args:    []string{"service", "frobnicate"},
			wantErr: `unknown command "frobnicate" for "tiger service"`,
		},
	})
}

func TestServiceCommandAliases(t *testing.T) {
	emptyList := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().GetServicesWithResponse(validCtx, testProjectID, nil).
			Return(&api.GetServicesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &[]api.Service{},
			}, nil)
	}

	runCmdTests(t, []cmdTest{
		{
			name:       "service",
			args:       []string{"service", "list"},
			mock:       emptyList,
			wantStderr: noServicesStderr,
		},
		{
			name:       "services alias",
			args:       []string{"services", "list"},
			mock:       emptyList,
			wantStderr: noServicesStderr,
		},
		{
			name:       "svc alias",
			args:       []string{"svc", "list"},
			mock:       emptyList,
			wantStderr: noServicesStderr,
		},
	})
}

// TestServiceExperimentalGate covers the registration gate for the remaining
// preview subtree (`service backup`): buildServiceCmd only adds it when the
// experimental env var is truthy, so by default the command doesn't exist in
// the tree at all. `service metrics` graduated off this gate and is asserted
// present regardless of the flag. Help output is asserted loosely
// (containment) rather than exactly, so the gate test doesn't break every time
// an unrelated subcommand's help text changes.
func TestServiceExperimentalGate(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:    "backup unregistered by default",
			args:    []string{"service", "backup"},
			wantErr: `unknown command "backup" for "tiger service"`,
		},
		{
			name:       "metrics registered by default",
			args:       []string{"service", "metrics", "--help"},
			wantStdout: matchHelp("Commands for querying time-series metrics"),
		},
		{
			name:       "backup registered when experimental",
			args:       []string{"service", "backup", "--help"},
			opts:       []runOption{withEnv("TIGER_EXPERIMENTAL", "true")},
			wantStdout: matchHelp("Manage the backups taken for a database service"),
		},
	})
}

// matchHelp asserts the output is a help text that mentions want and none of
// the gated names in absent. Help is asserted loosely (containment) so a gate
// test doesn't break every time unrelated help text changes.
func matchHelp(want string, absent ...string) matcher {
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

// envTag returns a sampleService override setting the service's environment
// tag, which is what the prod read-only gate reads off the resolved service.
func envTag(tag string) func(*api.Service) {
	return func(s *api.Service) {
		s.Metadata = &api.ServiceMetadata{Environment: &tag}
	}
}

// fluidPlatform is a sampleService override putting the service on the fluid
// platform, whose CPU/Memory combinations differ from standard's.
func fluidPlatform(s *api.Service) {
	s.Platform = new(api.ServicePlatformFLUID)
}

// withIOPS is a sampleService override setting the provisioned IOPS of the
// service's resource.
func withIOPS(iops int) func(*api.Service) {
	return func(s *api.Service) {
		s.Resources[0].Spec.Iops = new(iops)
	}
}

// expectTaggedService registers the resolution of svc-12345 onto a service
// carrying the given environment tag — the lookup whose tag the prod gate
// reads, with nothing registered for the write it then refuses.
func expectTaggedService(tag string) func(*mocks.MockClientWithResponsesInterface) {
	return func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveRef(m, "svc-12345", sampleService(envTag(tag)))
	}
}
