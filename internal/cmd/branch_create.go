package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchCreateCmd creates the create subcommand of `tiger branch`, the
// same command as `tiger service branch` (see newBranchCreateCmd).
func buildBranchCreateCmd(app *common.App) *cobra.Command {
	return newBranchCreateCmd(app, "create [service-name-or-id]", "tiger branch create")
}
