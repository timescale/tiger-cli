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
		ValidArgsFunction: configKeyValueCompletion,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.GetConfig()

			key, value := args[0], args[1]

			// The default service is stored as an ID, so a name given here is
			// resolved once at write time rather than on every command that
			// reads it.
			var resolvedName string
			if key == "service_id" && value != "" {
				_, client, projectID, err := app.GetAll()
				if err != nil {
					return err
				}
				service, err := resolveService(cmd.Context(), client, projectID, value)
				if err != nil {
					return err
				}
				value, resolvedName = service.ServiceID, service.Name
			}

			stored, err := cfg.Set(key, value)
			if err != nil {
				return fmt.Errorf("failed to set config: %w", err)
			}

			if resolvedName != "" {
				cmd.Printf("Set %s = %s (%s)\n", key, stored, resolvedName)
				return nil
			}
			cmd.Printf("Set %s = %s\n", key, stored)
			return nil
		},
	}
}
