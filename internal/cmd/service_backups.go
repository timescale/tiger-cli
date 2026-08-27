package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	units "github.com/docker/go-units"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceBackupsCmd creates the backup command for listing a service's
// backups. The endpoint is marked preview upstream, so registration is gated on
// TIGER_EXPERIMENTAL in buildServiceCmd.
func buildServiceBackupsCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup [service-id]",
		Short: "List backups for a service",
		Long: `List the full and incremental backups taken for a database service.

Backups run automatically on a schedule; there is no command to create or delete
one. To restore data, create a recovery fork with tiger service fork.

The service ID can be provided as an argument or will use the default service
from your configuration.

Examples:
  # List backups for the default service
  tiger service backup

  # List backups for a specific service
  tiger service backup svc-12345

  # Output as JSON
  tiger service backup -o json`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			resp, err := client.GetBackupsWithResponse(cmd.Context(), projectID, serviceID)
			if err != nil {
				return fmt.Errorf("failed to list backups: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}
			backups := *resp.JSON200

			if len(backups) == 0 {
				cmd.PrintErrln("No backups found for this service yet.")
				return nil
			}

			return outputBackups(cmd, backups, cfg.Output)
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	cmd.RegisterFlagCompletionFunc("output", outputCompletion())

	return cmd
}

func outputBackups(cmd *cobra.Command, backups []api.Backup, format string) error {
	out := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(out, backups)
	case "yaml":
		return util.SerializeToYAML(out, backups)
	case "env":
		return fmt.Errorf("environment variable output is not supported for backups")
	default:
		return outputBackupsTable(backups, out)
	}
}

// outputBackupsTable omits the label: it repeats STARTED and TYPE, and no command
// takes it as input. It stays in the json and yaml output.
func outputBackupsTable(backups []api.Backup, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("STARTED", "TYPE", "DURATION", "SIZE", "REGIONS")

	for _, backup := range backups {
		table.Append(
			backup.StartedAt.Local().Format("2006-01-02 15:04 MST"),
			string(backup.Type),
			formatDurationSeconds(backup.DurationSeconds),
			formatSizeBytes(backup.SizeBytes),
			backupRegions(backup),
		)
	}

	return table.Render()
}

// backupRegions lists the regions holding a copy, with the state of any copy the
// backend reports one for.
func backupRegions(backup api.Backup) string {
	regions := make([]string, 0, len(backup.Regions))
	for _, region := range backup.Regions {
		if region.Status == nil {
			regions = append(regions, region.RegionCode)
			continue
		}
		regions = append(regions, fmt.Sprintf("%s (%s)", region.RegionCode, *region.Status))
	}
	return strings.Join(regions, ", ")
}

func formatDurationSeconds(seconds *int64) string {
	if seconds == nil {
		return ""
	}
	return (time.Duration(*seconds) * time.Second).String()
}

func formatSizeBytes(bytes *int64) string {
	if bytes == nil {
		return ""
	}
	return units.BytesSize(float64(*bytes))
}
