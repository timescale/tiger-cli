package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildDbCreateCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create database resources",
		Long:  `Create database resources such as roles, databases, and extensions.`,
	}

	cmd.AddCommand(buildDbCreateRoleCmd(app))

	return cmd
}
