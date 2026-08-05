package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceMetricsCmd creates the metrics subcommand group. The metrics
// surface targets gateway endpoints marked `x-preview: true` in the OpenAPI
// spec — their request/response contract is still in flux. Registration is
// gated on TIGER_EXPERIMENTAL in buildServiceCmd, so this builder is only
// called when the env var is set; the tree doesn't include `metrics` at all
// otherwise.
func buildServiceMetricsCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "View service metrics",
		Long:  `Commands for querying time-series metrics for a Tiger Cloud service.`,
	}
	cmd.AddCommand(buildServiceMetricsAvailableSeriesCmd(app))
	cmd.AddCommand(buildServiceMetricsSeriesCmd(app))
	return cmd
}
