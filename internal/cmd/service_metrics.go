package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceMetricsCmd creates the metrics subcommand group. Registered
// unconditionally in buildServiceCmd.
func buildServiceMetricsCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "View service metrics",
		Long:  `Commands for querying time-series metrics for a Tiger Cloud service.`,
	}
	cmd.AddCommand(buildServiceMetricsAvailableSeriesCmd(app))
	cmd.AddCommand(buildServiceMetricsDetailsCmd(app))
	cmd.AddCommand(buildServiceMetricsSeriesCmd(app))
	return cmd
}
