package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceAllowListCmd creates the service allowlist group command.
func buildServiceAllowListCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allowlist",
		Short: "Manage a service's IP allow list",
		Long:  `Attach or detach the IP allow list that restricts which IP ranges can reach a service.`,
	}

	cmd.AddCommand(buildServiceAllowListAttachCmd(app))
	cmd.AddCommand(buildServiceAllowListDetachCmd(app))

	return cmd
}
