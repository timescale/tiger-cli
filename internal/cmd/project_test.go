package cmd

import "testing"

// TestProjectCmd covers the group command itself (see groupRun and groupArgs).
func TestProjectCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "no subcommand prints help without loading",
			args:       []string{"project"},
			wantStdout: matchHelp("Manage Tiger Cloud projects."),
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:    "unknown subcommand suggests close matches",
			args:    []string{"project", "lsit"},
			wantErr: "unknown command \"lsit\" for \"tiger project\"\n\nDid you mean this?\n\tlist",
		},
		{
			name:    "unknown subcommand with no close match",
			args:    []string{"project", "frobnicate"},
			wantErr: `unknown command "frobnicate" for "tiger project"`,
		},
	})
}
