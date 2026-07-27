package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/config"
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

func configOptionCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Config option is always first positional argument
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return filterCompletionsByPrefix(config.ValidConfigOptions(), toComplete), cobra.ShellCompDirectiveNoFileComp
}
