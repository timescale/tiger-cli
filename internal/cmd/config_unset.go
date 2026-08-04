package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/logging"
)

func buildConfigUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "unset <key>",
		Short:             "Remove configuration value",
		Long:              `Remove a configuration value and save changes to ~/.config/tiger/config.yaml`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: configOptionCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, err := config.Load(cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			key := args[0]
			if err := cfg.Unset(key); err != nil {
				return fmt.Errorf("failed to unset config: %w", err)
			}

			logging.Info("Configuration updated", zap.String("key", key))
			fmt.Fprintf(cmd.OutOrStdout(), "Unset %s\n", key)
			return nil
		},
	}
}
