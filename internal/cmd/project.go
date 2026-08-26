package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildProjectCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage Tiger Cloud projects",
		Long:  `Manage Tiger Cloud projects.`,
	}

	cmd.AddCommand(buildProjectUseCmd(app))

	return cmd
}
