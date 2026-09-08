package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceBackupRegionRemoveCmd creates the region remove subcommand.
func buildServiceBackupRegionRemoveCmd(app *common.App) *cobra.Command {
	var region string
	var removeConfirm bool

	cmd := &cobra.Command{
		Use:   "remove [service-id]",
		Short: "Stop copying a service's backups to a region",
		Long: `Stop copying a service's backups to a region.

Copies already stored there are deleted in the background and cannot be
recovered. By default, you will be prompted to type the service ID to
confirm, unless you use the --confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.

Examples:
  # Stop copying a service's backups to eu-central-1 (with confirmation prompt)
  tiger service backup region remove svc-12345 --region eu-central-1

  # Stop copying without a confirmation prompt
  tiger service backup region remove svc-12345 --region eu-central-1 --confirm`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceID := args[0]

			// Check read-only mode before the confirmation prompt, so it refuses
			// without asking the user to type the service ID.
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			if err := common.CheckReadOnlyByServiceID(cmd.Context(), cfg, client, projectID, serviceID); err != nil {
				return err
			}

			if !removeConfirm {
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
					return fmt.Errorf("TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt")
				}
				cmd.PrintErrf("Are you sure you want to stop copying service '%s' backups to '%s'? Existing copies there will be deleted and cannot be recovered.\n", serviceID, region)
				cmd.PrintErrf("Type the service ID '%s' to confirm: ", serviceID)
				confirmation, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if confirmation != serviceID {
					cmd.PrintErrln("❌ Remove operation cancelled.")
					return nil
				}
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
	cmd.Flags().BoolVar(&removeConfirm, "confirm", false, "Skip confirmation prompt (AI agents must confirm with user first)")
	cmd.MarkFlagRequired("region")

	return cmd
}
