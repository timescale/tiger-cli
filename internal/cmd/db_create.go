package cmd

import (
	"github.com/spf13/cobra"
)

func buildDbCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create database resources",
		Long:  `Create database resources such as roles, databases, and extensions.`,
	}

	cmd.AddCommand(buildDbCreateRoleCmd())

	return cmd
}
