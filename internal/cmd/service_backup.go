package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceBackupCmd creates the backup subcommand group. The endpoints
// are marked preview upstream, so registration is gated on TIGER_EXPERIMENTAL
// in buildServiceCmd.
func buildServiceBackupCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Manage a service's backups",
		Long:  `Manage the backups taken for a database service, including cross-region copies.`,
	}

	cmd.AddCommand(buildServiceBackupListCmd(app))
	cmd.AddCommand(buildServiceBackupRegionCmd(app))

	return cmd
}
