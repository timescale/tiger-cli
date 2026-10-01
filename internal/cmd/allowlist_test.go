package cmd

import (
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/common"
)

// TestAllowListExperimentalGate covers the registration gate for the preview
// `allowlist` command: buildRootCmd only adds it when the experimental env
// var is truthy, so by default the command doesn't exist in the tree at all.
func TestAllowListExperimentalGate(t *testing.T) {
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	runCmdTests(t, []cmdTest{
		{
			name:    "unregistered by default",
			args:    []string{"allowlist", "list"},
			wantErr: `unknown command "allowlist" for "tiger"`,
			wantStderr: "Error: unknown command \"allowlist\" for \"tiger\"\n" +
				"Run 'tiger --help' for usage.\n",
		},
		{
			name: "registered when experimental",
			args: []string{"allowlist", "--help"},
			opts: []runOption{experimental},
			wantStdout: matchFunc(func(t *testing.T, got string) {
				if !strings.Contains(got, "Manage the IP allow lists") {
					t.Errorf("expected help output mentioning IP allow lists, got:\n%s", got)
				}
			}),
		},
		{
			name:    "allowlists alias",
			args:    []string{"allowlists", "list"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
	})
}
