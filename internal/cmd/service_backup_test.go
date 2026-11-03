package cmd

import "testing"

// TestServiceBackupCmd covers the group command itself (see groupRun and groupArgs).
func TestServiceBackupCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "no subcommand prints help without loading",
			args:       []string{"service", "backup"},
			opts:       []runOption{withExperimental()},
			wantStdout: matchHelp("Manage the backups taken for a database service, including cross-region copies."),
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:    "unknown subcommand suggests close matches",
			args:    []string{"service", "backup", "lsit"},
			opts:    []runOption{withExperimental()},
			wantErr: "unknown command \"lsit\" for \"tiger service backup\"\n\nDid you mean this?\n\tlist",
		},
		{
			name:    "unknown subcommand with no close match",
			args:    []string{"service", "backup", "frobnicate"},
			opts:    []runOption{withExperimental()},
			wantErr: `unknown command "frobnicate" for "tiger service backup"`,
		},
	})
}
