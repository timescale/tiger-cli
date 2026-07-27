package cmd

import (
	"github.com/spf13/cobra"
)

func buildAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication and credentials",
		Long:  `Manage authentication and credentials for Tiger Cloud platform.`,
	}

	cmd.AddCommand(buildLoginCmd())
	cmd.AddCommand(buildLogoutCmd())
	cmd.AddCommand(buildStatusCmd())

	return cmd
}
