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

// buildServiceBackupRegionListCmd creates the backup-region list subcommand.
func buildServiceBackupRegionListCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [service-id]",
		Short: "List a service's backup regions",
		Long: `List the additional regions a service's backups are copied to.

The region the service runs in is not listed here; it always has a copy.

The service ID can be provided as an argument or will use the default service
from your configuration.

Examples:
  # List backup regions for the default service
  tiger service backup-region list

  # List backup regions for a specific service
  tiger service backup-region list svc-12345

  # Output as JSON
  tiger service backup-region list -o json`,
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

			resp, err := client.GetBackupRegionsWithResponse(cmd.Context(), projectID, serviceID)
			if err != nil {
				return fmt.Errorf("failed to list backup regions: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}
			regions := *resp.JSON200

			if len(regions) == 0 {
				cmd.PrintErrln("No backup regions configured for this service.")
				return nil
			}

			return outputBackupRegions(cmd, regions, cfg.Output)
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	cmd.RegisterFlagCompletionFunc("output", outputCompletion())

	return cmd
}

func outputBackupRegions(cmd *cobra.Command, regions []api.BackupRegion, format string) error {
	out := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(out, regions)
	case "yaml":
		return util.SerializeToYAML(out, regions)
	case "env":
		return fmt.Errorf("environment variable output is not supported for backup regions")
	default:
		return outputBackupRegionsTable(regions, out)
	}
}

func outputBackupRegionsTable(regions []api.BackupRegion, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("REGION", "ADDED")

	for _, region := range regions {
		var added string
		if region.Created != nil {
			added = region.Created.Local().Format("2006-01-02 15:04 MST")
		}
		table.Append(region.RegionCode, added)
	}

	return table.Render()
}
