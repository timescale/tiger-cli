package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildConfigSetCmd(app *common.App) *cobra.Command {
	return &cobra.Command{
		Use:               "set <key> <value>",
		Short:             "Set configuration value",
		Long:              `Set a configuration value and save it to ~/.config/tiger/config.yaml`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: configOptionCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg := app.GetConfig()

			key, value := args[0], args[1]
			if err := cfg.Set(key, value); err != nil {
				return fmt.Errorf("failed to set config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, value)
			return nil
		},
	}
}
