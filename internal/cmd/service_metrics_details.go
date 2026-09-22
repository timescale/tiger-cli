package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceMetricsDetailsCmd fetches descriptive metadata for a metric
func buildServiceMetricsDetailsCmd(app *common.App) *cobra.Command {
	var metric string

	cmd := &cobra.Command{
		Use:   "details [service-id]",
		Short: "Get metric details",
		Long: `Get descriptive metadata for a metric: what it measures, its type, default
aggregation function, and available labels.

Use 'tiger service metrics available-series' to discover valid metric names,
then 'tiger service metrics series' to fetch its data.

These metrics have no richer metadata — expect just the name back, with type,
default aggregation, description, and labels all empty: timescale_cloud_system_cpu_total_millicores,
timescale_cloud_system_cpu_usage_millicores, timescale_cloud_system_disk_io_read_bytes,
timescale_cloud_system_disk_io_read_ops, timescale_cloud_system_disk_io_total_bytes,
timescale_cloud_system_disk_io_total_ops, timescale_cloud_system_disk_io_write_bytes,
timescale_cloud_system_disk_io_write_ops, timescale_cloud_system_disk_usage_bytes,
timescale_cloud_system_memory_total_bytes, timescale_cloud_system_memory_usage_bytes,
timescale_cloud_database_qps, timescale_cloud_database_num_connections,
timescale_cloud_database_job_duration_usecs, timescale_cloud_database_job_success.`,
		Example: `  # Describe a metric
  tiger service metrics details --metric pg_stat_activity_count

  # Get metric details as JSON
  tiger service metrics details --metric pg_stat_activity_count --output json`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			resp, err := client.GetServiceMetricDetailsWithResponse(cmd.Context(), projectID, serviceID, metric)
			if err != nil {
				return fmt.Errorf("failed to get metric details: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			return outputMetricDetails(cmd.OutOrStdout(), cfg.Output, *resp.JSON200)
		},
	}

	cmd.Flags().StringVar(&metric, "metric", "", "Metric name")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	markFlagRequired(cmd, "metric")

	return cmd
}

// outputMetricDetails formats and outputs metric metadata based on the specified format
func outputMetricDetails(output io.Writer, format string, details api.MetricDetails) error {
	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(output, details)
	case "yaml":
		return util.SerializeToYAML(output, details)
	default:
		return outputMetricDetailsTable(details, output)
	}
}

// outputMetricDetailsTable renders metric metadata as a PROPERTY/VALUE table,
// plus a LABEL/DESCRIPTION table when the metric has labels.
func outputMetricDetailsTable(details api.MetricDetails, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("PROPERTY", "VALUE")

	table.Append("Name", details.Name)

	metricType := "undocumented"
	if details.Type != nil {
		metricType = string(*details.Type)
	}
	table.Append("Type", metricType)

	defaultAgg := "undocumented"
	if details.DefaultAgg != nil {
		defaultAgg = string(*details.DefaultAgg)
	}
	table.Append("Default Aggregation", defaultAgg)

	description := details.Description
	if description == "" {
		description = "undocumented"
	}
	table.Append("Description", description)

	if err := table.Render(); err != nil {
		return err
	}

	if len(details.Labels) == 0 {
		return nil
	}

	fmt.Fprintln(output)

	labelTable := tablewriter.NewWriter(output)
	labelTable.Header("LABEL", "DESCRIPTION")
	for _, l := range details.Labels {
		desc := l.Description
		if desc == "" {
			desc = "undocumented"
		}
		labelTable.Append(l.Name, desc)
	}
	return labelTable.Render()
}
