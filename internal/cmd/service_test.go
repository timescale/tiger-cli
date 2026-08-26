package cmd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceCommandAliases(t *testing.T) {
	emptyList := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
			Return(&api.GetServicesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &[]api.Service{},
			}, nil)
	}

	tests := []cmdTest{
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
	}

	runCmdTests(t, tests)
}

// TestServiceMetricsExperimentalGate covers the registration gate for the
// preview `service metrics` subtree: buildServiceCmd only adds it when the
// experimental env var is truthy, so by default the command doesn't exist in
// the tree at all. Help output is asserted loosely (containment) rather than
// exactly, so the gate test doesn't break every time an unrelated subcommand's
// help text changes.
func TestServiceMetricsExperimentalGate(t *testing.T) {
	t.Run("unregistered by default", func(t *testing.T) {
		result := runCommand(t, []string{"service", "metrics"}, nil)
		// Cobra only reports "unknown command" at the root level; for a
		// non-root group it treats the unknown name as a stray argument and
		// prints the group's help. The gate is observable as the absence of
		// any metrics entry in that help.
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		if !strings.Contains(result.stdout, "Available Commands:") {
			t.Errorf("expected the service group help on stdout, got:\n%s", result.stdout)
		}
		if strings.Contains(result.stdout, "metrics") {
			t.Errorf("service help must not mention metrics when experimental is off, got:\n%s", result.stdout)
		}
	})

	t.Run("registered when experimental", func(t *testing.T) {
		result := runCommand(t, []string{"service", "metrics", "--help"}, nil,
			withEnv("TIGER_EXPERIMENTAL", "true"))
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		if !strings.Contains(result.stdout, "Commands for querying time-series metrics") {
			t.Errorf("expected the metrics group help on stdout, got:\n%s", result.stdout)
		}
	})
}
