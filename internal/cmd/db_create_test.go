package cmd

import "testing"

// TestDbCreateCmd covers the group command itself (see groupRun and groupArgs).
func TestDbCreateCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "no subcommand prints help without loading",
			args:       []string{"db", "create"},
			wantStdout: matchHelp("Create database resources such as roles, databases, and extensions."),
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:    "unknown subcommand suggests close matches",
			args:    []string{"db", "create", "rol"},
			wantErr: "unknown command \"rol\" for \"tiger db create\"\n\nDid you mean this?\n\trole",
		},
		{
			name:    "unknown subcommand with no close match",
			args:    []string{"db", "create", "frobnicate"},
			wantErr: `unknown command "frobnicate" for "tiger db create"`,
		},
	})
}
