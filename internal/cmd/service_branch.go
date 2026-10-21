package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceBranchCmd creates the branch subcommand of `tiger service`, the
// same command as `tiger branch create` (see newBranchCreateCmd).
func buildServiceBranchCmd(app *common.App) *cobra.Command {
	return newBranchCreateCmd(app, "branch [name-or-id]", "tiger service branch")
}
