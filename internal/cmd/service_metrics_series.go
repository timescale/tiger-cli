package cmd

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceMetricsSeriesCmd fetches time-series data for a named metric
func buildServiceMetricsSeriesCmd(app *common.App) *cobra.Command {
	var metric string
	var from string
	var to string
	var role string
	var filters []string
	var bucketSeconds int
	var fn string

	cmd := &cobra.Command{
		Use:   "series [service-id]",
		Short: "Get metric series data",
		Long: `Get time-series data for a specific metric.

Use 'tiger service metrics available-series' to discover valid metric names.

Each labeled series (e.g. one per replica) is returned independently with its
full list of raw data points.`,
		Example: `  # Fetch CPU usage for the last hour
  tiger service metrics series --metric timescale_cloud_system_cpu_usage_millicores \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z

  # Get memory data points as JSON
  tiger service metrics series --metric timescale_cloud_system_memory_usage_bytes \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z --output json

  # Fetch data for the primary instance only
  tiger service metrics series --metric timescale_cloud_system_cpu_usage_millicores \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z --role PRIMARY

  # Filter by an arbitrary label
  tiger service metrics series --metric some_metric_name \
    --from 2026-05-13T00:00:00Z --to 2026-05-13T01:00:00Z \
    --filter ordinal=0`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fromTime, err := time.Parse(time.RFC3339, from)
			if err != nil {
				return fmt.Errorf("--from must be RFC3339 (e.g., 2026-05-13T00:00:00Z): %w", err)
			}
			toTime, err := time.Parse(time.RFC3339, to)
			if err != nil {
				return fmt.Errorf("--to must be RFC3339 (e.g., 2026-05-13T01:00:00Z): %w", err)
			}

			labelFilters, err := parseMetricFilters(role, filters)
			if err != nil {
				return err
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			body := api.MetricsSeriesRequest{
				Name: metric,
				From: fromTime,
				To:   toTime,
			}
			if bucketSeconds > 0 {
				bs := bucketSeconds
				body.BucketSeconds = &bs
			}
			if fn != "" {
				f := api.MetricsSeriesRequestFn(strings.ToUpper(fn))
				body.Fn = &f
			}
			if len(labelFilters) > 0 {
				body.Filters = &labelFilters
			}

			resp, err := client.GetServiceMetricsSeriesWithResponse(cmd.Context(), projectID, serviceID, body)
			if err != nil {
				return fmt.Errorf("failed to fetch metric series: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			return renderMetricSeries(cmd, cfg.Output, *resp.JSON200)
		},
	}

	cmd.Flags().StringVar(&metric, "metric", "", "Metric series name")
	cmd.Flags().StringVar(&from, "from", "", "Start of the time window (RFC3339)")
	cmd.Flags().StringVar(&to, "to", "", "End of the time window (RFC3339)")
	cmd.Flags().StringVar(&role, "role", "", "Filter to a specific instance role (PRIMARY or REPLICA)")
	cmd.Flags().StringSliceVar(&filters, "filter", nil, "Arbitrary label filter as name=value (repeatable)")
	cmd.Flags().IntVar(&bucketSeconds, "bucket-seconds", 0, "Aggregation bucket size in seconds (optional; server auto-selects based on the time window when omitted, minimum 60s)")
	cmd.Flags().StringVar(&fn, "fn", "", "Aggregation function applied per bucket. One of: RATE, INCREASE, SUM, AVG, MIN, MAX, COUNT, P50, P90, P99, LAST. Rejected on the timescale_cloud_* resource/qps/connections/jobs metrics; omit to let the server pick the default")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())
	registerFlagCompletion(cmd, "role", metricsSeriesRoleCompletion)
	registerFlagCompletion(cmd, "fn", metricsSeriesFnCompletion)

	cmd.MarkFlagRequired("metric")
	cmd.MarkFlagRequired("from")
	cmd.MarkFlagRequired("to")

	return cmd
}

// parseMetricFilters merges --role and --filter into the preview label filter
// list. Server-side label values are lowercased on response, so we lowercase
// role values here too for symmetry.
func parseMetricFilters(role string, filters []string) ([]api.MetricLabelFilter, error) {
	var out []api.MetricLabelFilter
	if role != "" {
		out = append(out, api.MetricLabelFilter{Key: "role", Value: strings.ToLower(role)})
	}
	for _, f := range filters {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("--filter must be name=value, got %q", f)
		}
		out = append(out, api.MetricLabelFilter{Key: k, Value: v})
	}
	return out, nil
}

// labelString renders a label map as Prometheus-style `{name="value",...}` for
// human-friendly output. Keys are sorted for stable display.
func labelString(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func renderMetricSeries(cmd *cobra.Command, output string, series []api.MetricSeries) error {
	out := cmd.OutOrStdout()

	switch strings.ToLower(output) {
	case "json":
		return util.SerializeToJSON(out, series)
	case "yaml":
		return util.SerializeToYAML(out, series)
	default:
		if len(series) == 0 {
			cmd.Println("No metric data returned for the requested window.")
			return nil
		}
		table := tablewriter.NewWriter(out)
		table.Header("SERIES", "TIME", "VALUE")
		for _, s := range series {
			label := labelString(s.Labels)
			if len(s.Data) == 0 {
				// Keep matched-but-empty series visible instead of dropping
				// them: emit one placeholder row for the label.
				table.Append(label, "", "(no data)")
				continue
			}
			for _, p := range s.Data {
				val := "null"
				if p.Value != nil {
					val = fmt.Sprintf("%.3f", *p.Value)
				}
				table.Append(label, p.Time.UTC().Format(time.RFC3339), val)
			}
		}
		return table.Render()
	}
}
