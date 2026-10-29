package cmd

import "testing"

// TestServiceBackupRegionCmd covers the group command itself (see groupRun and groupArgs).
func TestServiceBackupRegionCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "no subcommand prints help without loading",
			args:       []string{"service", "backup", "region"},
			opts:       []runOption{withExperimental()},
			wantStdout: matchHelp("Manage the additional regions a service's backups are copied to."),
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:    "unknown subcommand suggests close matches",
			args:    []string{"service", "backup", "region", "remvoe"},
			opts:    []runOption{withExperimental()},
			wantErr: "unknown command \"remvoe\" for \"tiger service backup region\"\n\nDid you mean this?\n\tremove",
		},
		{
			name:    "unknown subcommand with no close match",
			args:    []string{"service", "backup", "region", "frobnicate"},
			opts:    []runOption{withExperimental()},
			wantErr: `unknown command "frobnicate" for "tiger service backup region"`,
		},
	})
}
