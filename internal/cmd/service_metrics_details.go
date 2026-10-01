package cmd

import (
	"errors"
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
		Use:   "details [name-or-id]",
		Short: "Get metric details",
		Long: fmt.Sprintf(`Get descriptive metadata for a metric: what it measures, its type, default
aggregation function, and available labels.

Use 'tiger service metrics available' to discover valid metric names,
then 'tiger service metrics series' to fetch its data.

These metrics have no richer metadata — expect just the name back, with type,
default aggregation, description, and labels all empty: %s.

The service can be given by ID or name as an argument, or will use the default
service from your configuration.`, strings.Join(common.LegacyMetrics, ", ")),
		Example: `  # Describe a metric
  tiger service metrics details --metric pg_stat_activity_count

  # Get metric details as JSON
  tiger service metrics details --metric pg_stat_activity_count --output json`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			serviceRef, err := getServiceRef(cmd, cfg, args)
			if err != nil {
				return err
			}

			service, err := resolveService(cmd.Context(), client, projectID, serviceRef)
			if err != nil {
				return err
			}

			resp, err := client.GetServiceMetricDetailsWithResponse(cmd.Context(), projectID, service.ServiceID, metric)
			if err != nil {
				return fmt.Errorf("failed to get metric details: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}

			return outputMetricDetails(cmd.OutOrStdout(), cfg.Output, *resp.JSON200)
		},
	}

	cmd.Flags().StringVar(&metric, "metric", "", "Metric name")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())
	registerFlagCompletion(cmd, "metric", metricNameCompletion(app))

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

	table.Append("Name", details.MetricName)

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
