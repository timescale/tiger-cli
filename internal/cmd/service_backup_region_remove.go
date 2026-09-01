package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceBackupRegionRemoveCmd creates the region remove subcommand.
func buildServiceBackupRegionRemoveCmd(app *common.App) *cobra.Command {
	var region string

	cmd := &cobra.Command{
		Use:   "remove [service-id]",
		Short: "Stop copying a service's backups to a region",
		Long: `Stop copying a service's backups to a region.

Copies already stored there are deleted in the background and cannot be
recovered.

The service ID can be provided as an argument or will use the default service
from your configuration.

Examples:
  # Stop copying the default service's backups to eu-central-1
  tiger service backup region remove --region eu-central-1

  # Stop copying a specific service's backups to eu-central-1
  tiger service backup region remove svc-12345 --region eu-central-1`,
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

			resp, err := client.DeleteBackupRegionWithResponse(cmd.Context(), projectID, serviceID, region)
			if err != nil {
				return fmt.Errorf("failed to remove backup region: %w", err)
			}

			if resp.StatusCode() != http.StatusNoContent {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			cmd.PrintErrf("✅ Backups for service '%s' will no longer be copied to '%s'.\n", serviceID, region)
			return nil
		},
	}

	cmd.Flags().StringVar(&region, "region", "", "Region to stop copying backups to")
	cmd.MarkFlagRequired("region")

	return cmd
}
