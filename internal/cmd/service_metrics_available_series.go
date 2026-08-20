package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceMetricsAvailableSeriesCmd lists the metric series available for a service
func buildServiceMetricsAvailableSeriesCmd(app *common.App) *cobra.Command {

	cmd := &cobra.Command{
		Use:          "available-series [service-id]",
		Short:        "List available metric series",
		Long:         `List the names of all metric series available for a service.`,
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

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			resp, err := client.GetServiceMetricsAvailableSeriesWithResponse(ctx, projectID, serviceID)
			if err != nil {
				return fmt.Errorf("failed to list metric series: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			series := *resp.JSON200

			out := cmd.OutOrStdout()
			switch strings.ToLower(cfg.Output) {
			case "json":
				return util.SerializeToJSON(out, series)
			case "yaml":
				return util.SerializeToYAML(out, series)
			default:
				for _, s := range series {
					cmd.Println(s)
				}
			}
			return nil
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	return cmd
}
