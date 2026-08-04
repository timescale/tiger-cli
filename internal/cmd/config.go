package cmd

import (
	"github.com/spf13/cobra"
)

func buildConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
		Long:  `Manage CLI configuration settings stored in ~/.config/tiger/config.yaml`,
	}

	cmd.AddCommand(buildConfigShowCmd())
	cmd.AddCommand(buildConfigSetCmd())
	cmd.AddCommand(buildConfigUnsetCmd())
	cmd.AddCommand(buildConfigResetCmd())

	return cmd
}
