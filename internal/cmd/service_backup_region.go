package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceBackupRegionCmd creates the backup-region subcommand group. The
// endpoints are marked preview upstream, so registration is gated on
// TIGER_EXPERIMENTAL in buildServiceCmd.
func buildServiceBackupRegionCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup-region",
		Short: "Manage a service's cross-region backup copies",
		Long: `Manage the additional regions a service's backups are copied to.

The region the service runs in always has a copy; these commands manage
regions beyond that one.`,
	}

	cmd.AddCommand(buildServiceBackupRegionListCmd(app))
	cmd.AddCommand(buildServiceBackupRegionAddCmd(app))
	cmd.AddCommand(buildServiceBackupRegionRemoveCmd(app))

	return cmd
}
