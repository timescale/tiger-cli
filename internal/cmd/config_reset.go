package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildConfigResetCmd(app *common.App) *cobra.Command {
	return &cobra.Command{
		Use:          "reset",
		Aliases:      []string{"clear"},
		Short:        "Reset to defaults",
		Long:         `Reset all configuration settings to their default values`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.GetConfig()

			if err := cfg.Reset(); err != nil {
				return fmt.Errorf("failed to reset config: %w", err)
			}

			cmd.Println("Configuration reset to defaults")
			return nil
		},
	}
}
