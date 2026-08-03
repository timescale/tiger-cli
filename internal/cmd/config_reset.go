package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/logging"
)

func buildConfigResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "reset",
		Short:             "Reset to defaults",
		Long:              `Reset all configuration settings to their default values`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := cfg.Reset(); err != nil {
				return fmt.Errorf("failed to reset config: %w", err)
			}

			logging.Info("Configuration reset to defaults")
			fmt.Fprintln(cmd.OutOrStdout(), "Configuration reset to defaults")
			return nil
		},
	}
}
