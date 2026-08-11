package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildConfigResetCmd(app *common.App) *cobra.Command {
	return &cobra.Command{
		Use:               "reset",
		Short:             "Reset to defaults",
		Long:              `Reset all configuration settings to their default values`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg := app.GetConfig()

			if err := cfg.Reset(); err != nil {
				return fmt.Errorf("failed to reset config: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Configuration reset to defaults")
			return nil
		},
	}
}
