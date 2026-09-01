package cmd

import (
	"net/http"
	"strings"
	"testing"

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
			wantStdout: matchHelp("Manage the backups taken for a database service"),
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

// expectTaggedService registers a fetch of svc-12345 carrying the given
// environment tag: the lookup a ...ByServiceID gate makes, or — for commands
// that fetch the service anyway (update-password, the db commands) — the fetch
// whose tag the gate reads.
func expectTaggedService(tag string) func(*mocks.MockClientWithResponsesInterface) {
	return func(m *mocks.MockClientWithResponsesInterface) {
		expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
			s.Metadata = &api.ServiceMetadata{Environment: &tag}
		}))
	}
}
