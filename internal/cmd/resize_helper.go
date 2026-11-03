package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

// resizeInProgressMsg follows an accepted resize. The resize commands don't
// wait for it to finish, because the API doesn't reliably report when it has:
// the status can read READY before the resize has even started.
const resizeInProgressMsg = "The resize is in progress and may take several minutes to complete."

// resizeWaitFlags are the resize commands' former wait flags. They're kept as
// hidden no-ops so scripts that pass them don't break.
var resizeWaitFlags = []string{"no-wait", "wait-timeout"}

// addResizeWaitFlags registers resizeWaitFlags on a resize command.
func addResizeWaitFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("no-wait", false, "Don't wait for resize operation to complete")
	cmd.Flags().Duration("wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")
	for _, flag := range resizeWaitFlags {
		markFlagHidden(cmd, flag)
	}
}

// warnResizeWaitFlags warns about any of resizeWaitFlags that were given. It
// prints to stderr itself because cobra's flag deprecation notice goes to
// stdout.
func warnResizeWaitFlags(cmd *cobra.Command) {
	for _, flag := range resizeWaitFlags {
		if cmd.Flags().Changed(flag) {
			cmd.PrintErrf("Warning: --%s has no effect; resize no longer waits for completion\n", flag)
		}
	}
}
