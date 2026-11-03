package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildDbExtensionsCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "extensions",
		Aliases:      []string{"ext"},
		Short:        "Manage PostgreSQL extensions",
		Long:         `Manage the PostgreSQL extensions installed in a database service.`,
		Args:         groupArgs,
		SilenceUsage: true,
		Run:          groupRun,
	}

	cmd.AddCommand(buildDbExtensionsListCmd(app))

	return cmd
}
