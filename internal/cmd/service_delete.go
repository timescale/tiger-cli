package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceDeleteCmd creates the delete subcommand
func buildServiceDeleteCmd(app *common.App) *cobra.Command {
	var deleteNoWait bool
	var deleteWaitTimeout time.Duration
	var deleteConfirm bool

	cmd := &cobra.Command{
		Use:   "delete [service-id]",
		Short: "Delete a database service",
		Long: `Delete a database service permanently.

This operation is irreversible. By default, you will be prompted to type the service ID
to confirm deletion, unless you use the --confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.

Examples:
  # Delete a service (with confirmation prompt)
  tiger service delete svc-12345

  # Delete service without confirmation prompt
  tiger service delete svc-12345 --confirm

  # Delete service without waiting for completion
  tiger service delete svc-12345 --no-wait

  # Delete service with custom wait timeout
  tiger service delete svc-12345 --wait-timeout 15m`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Require explicit service ID for safety
			if len(args) < 1 {
				return fmt.Errorf("service ID is required")
			}
			serviceID := args[0]

			cmd.SilenceUsage = true

			// Check read-only mode before the confirmation prompt, so it refuses
			// without asking the user to type the service ID.
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			if err := common.CheckReadOnly(cfg); err != nil {
				return err
			}

			// Prompt for confirmation unless --confirm is used
			if !deleteConfirm {
				if !util.IsTerminal(cmd.InOrStdin()) {
					return fmt.Errorf("TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt")
				}
				cmd.PrintErrf("Are you sure you want to delete service '%s'? This operation cannot be undone.\n", serviceID)
				cmd.PrintErrf("Type the service ID '%s' to confirm: ", serviceID)
				confirmation, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if confirmation != serviceID {
					cmd.PrintErrln("❌ Delete operation cancelled.")
					return nil
				}
			}

			// Make the delete request
			resp, err := client.DeleteServiceWithResponse(
				cmd.Context(),
				api.ProjectID(projectID),
				api.ServiceID(serviceID),
			)
			if err != nil {
				return fmt.Errorf("failed to delete Service: %w", err)
			}

			// Handle response
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			cmd.PrintErrf("🗑️  Delete request accepted for service '%s'.\n", serviceID)

			// If not waiting, return early
			if deleteNoWait {
				cmd.PrintErrln("💡 Use 'tiger service list' to check deletion status.")
				return nil
			}

			// Wait for deletion to complete
			if err := common.WaitForService(cmd.Context(), common.WaitForServiceArgs{
				Client:    client,
				ProjectID: projectID,
				ServiceID: serviceID,
				Handler: &common.DeletionWaitHandler{
					ServiceID: serviceID,
				},
				Output:     cmd.ErrOrStderr(),
				Timeout:    deleteWaitTimeout,
				TimeoutMsg: "service may still be deleting",
			}); err != nil {
				// Return error for sake of exit code, but log ourselves for sake of icon
				cmd.PrintErrf("❌ Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			cmd.PrintErrf("✅ Service '%s' has been successfully deleted.\n", serviceID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&deleteNoWait, "no-wait", false, "Don't wait for deletion to complete, return immediately")
	cmd.Flags().DurationVar(&deleteWaitTimeout, "wait-timeout", 30*time.Minute, "Wait timeout duration (e.g., 30m, 1h30m, 90s)")
	cmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Skip confirmation prompt (AI agents must confirm with user first)")

	return cmd
}
