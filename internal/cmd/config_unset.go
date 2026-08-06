package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildConfigUnsetCmd(app *common.App) *cobra.Command {
	return &cobra.Command{
		Use:               "unset <key>",
		Short:             "Remove configuration value",
		Long:              `Remove a configuration value and save changes to ~/.config/tiger/config.yaml`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: configOptionCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg := app.GetConfig()

			key := args[0]
			if err := cfg.Unset(key); err != nil {
				return fmt.Errorf("failed to unset config: %w", err)
			}

			cmd.Printf("Unset %s\n", key)
			return nil
		},
	}
}
