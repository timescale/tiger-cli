package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildConfigCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
		Long:  `Manage CLI configuration settings stored in ~/.config/tiger/config.yaml`,
	}

	cmd.AddCommand(buildConfigShowCmd(app))
	cmd.AddCommand(buildConfigSetCmd(app))
	cmd.AddCommand(buildConfigUnsetCmd(app))
	cmd.AddCommand(buildConfigResetCmd(app))

	return cmd
}
