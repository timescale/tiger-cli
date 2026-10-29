package cmd

import "testing"

// TestSkillsCmd covers the group command itself (see groupRun and groupArgs).
func TestSkillsCmd(t *testing.T) {
	runCmdTests(t, []cmdTest{
		{
			name:       "no subcommand prints help without loading",
			args:       []string{"skills"},
			wantStdout: matchHelp("Manage agent skills for AI coding agents."),
			checks:     []checkFunc{checkNotLoaded},
		},
		{
			name:    "unknown subcommand suggests close matches",
			args:    []string{"skills", "instal"},
			wantErr: "unknown command \"instal\" for \"tiger skills\"\n\nDid you mean this?\n\tinstall",
		},
		{
			name:    "unknown subcommand with no close match",
			args:    []string{"skills", "frobnicate"},
			wantErr: `unknown command "frobnicate" for "tiger skills"`,
		},
	})
}
