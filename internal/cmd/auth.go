package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildAuthCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication and credentials",
		Long:  `Manage authentication and credentials for Tiger Cloud platform.`,
	}

	cmd.AddCommand(buildLoginCmd(app))
	cmd.AddCommand(buildLogoutCmd(app))
	cmd.AddCommand(buildStatusCmd(app))

	return cmd
}
