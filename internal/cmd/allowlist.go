package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildAllowListCmd creates the allowlist group command.
func buildAllowListCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "allowlist",
		Aliases: []string{"allowlists"},
		Short:   "Manage IP allow lists",
		Long:    `Manage the IP allow lists that restrict which IP ranges can reach your services.`,
	}

	cmd.AddCommand(buildAllowListListCmd(app))
	cmd.AddCommand(buildAllowListGetCmd(app))
	cmd.AddCommand(buildAllowListCreateCmd(app))
	cmd.AddCommand(buildAllowListUpdateCmd(app))
	cmd.AddCommand(buildAllowListDeleteCmd(app))

	return cmd
}
