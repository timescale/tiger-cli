package cmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceBackupRegionAddCmd creates the region add subcommand.
func buildServiceBackupRegionAddCmd(app *common.App) *cobra.Command {
	var region string

	cmd := &cobra.Command{
		Use:   "add [service-id]",
		Short: "Start copying a service's backups to another region",
		Long: `Start copying a service's backups to another region.

The region is added immediately; existing backups are copied to it in the
background.

The service ID can be provided as an argument or will use the default service
from your configuration.

Examples:
  # Copy the default service's backups to eu-central-1
  tiger service backup region add --region eu-central-1

  # Copy a specific service's backups to eu-central-1
  tiger service backup region add svc-12345 --region eu-central-1`,
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

			if err := common.CheckReadOnlyByServiceID(cmd.Context(), cfg, client, projectID, serviceID); err != nil {
				return err
			}

			resp, err := client.CreateBackupRegionWithResponse(cmd.Context(), projectID, serviceID, api.BackupRegionCreate{
				RegionCode: region,
			})
			if err != nil {
				return fmt.Errorf("failed to add backup region: %w", err)
			}

			if resp.StatusCode() != http.StatusCreated {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON201 == nil {
				return fmt.Errorf("empty response from API")
			}

			cmd.PrintErrf("✅ Backups for service '%s' will now be copied to '%s'.\n", serviceID, region)

			switch strings.ToLower(cfg.Output) {
			case "json", "yaml":
				return outputBackupRegions(cmd, []api.BackupRegion{*resp.JSON201}, cfg.Output)
			default:
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&region, "region", "", "Region to copy backups to")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	cmd.RegisterFlagCompletionFunc("output", outputCompletion())

	cmd.MarkFlagRequired("region")

	return cmd
}
