package cmd

import "testing"

// TestServiceMetricsCmd covers the group command itself (see groupRun and groupArgs).
func TestServiceMetricsCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "no subcommand prints help without loading",
			args:       []string{"service", "metrics"},
			wantStdout: matchHelp("Commands for querying time-series metrics for a Tiger Cloud service."),
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:    "unknown subcommand suggests close matches",
			args:    []string{"service", "metrics", "seires"},
			wantErr: "unknown command \"seires\" for \"tiger service metrics\"\n\nDid you mean this?\n\tseries",
		},
		{
			name:    "unknown subcommand with no close match",
			args:    []string{"service", "metrics", "frobnicate"},
			wantErr: `unknown command "frobnicate" for "tiger service metrics"`,
		},
	})
}
