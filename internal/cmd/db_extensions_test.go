package cmd

import "testing"

// TestDbExtensionsCmd covers the group command itself (see groupRun and groupArgs).
func TestDbExtensionsCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "no subcommand prints help without loading",
			args:       []string{"db", "extensions"},
			wantStdout: matchHelp("Manage the PostgreSQL extensions installed in a database service."),
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:    "unknown subcommand suggests close matches",
			args:    []string{"db", "extensions", "lsit"},
			wantErr: "unknown command \"lsit\" for \"tiger db extensions\"\n\nDid you mean this?\n\tlist",
		},
		{
			name:    "unknown subcommand with no close match",
			args:    []string{"db", "extensions", "frobnicate"},
			wantErr: `unknown command "frobnicate" for "tiger db extensions"`,
		},
	})
}
