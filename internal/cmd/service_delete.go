package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceDeleteCmd creates the delete subcommand
func buildServiceDeleteCmd(app *common.App) *cobra.Command {
	var confirm bool

	cmd := &cobra.Command{
		Use:     "delete [service-id]",
		Aliases: []string{"rm"},
		Short:   "Delete a database service",
		Long: `Delete a database service permanently.

This operation is irreversible. By default, you will be prompted to type the service ID
to confirm deletion, unless you use the --confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.`,
		Example: `  # Delete a service (with confirmation prompt)
  tiger service delete svc-12345

  # Delete service without confirmation prompt
  tiger service delete svc-12345 --confirm`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Require explicit service ID for safety
			if len(args) < 1 {
				return fmt.Errorf("service ID is required")
			}
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

			// Prompt for confirmation unless --confirm is used
			if !confirm {
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
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

			cmd.PrintErrf("🗑️  Service '%s' has been deleted.\n", serviceID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&confirm, "confirm", false, "Skip confirmation prompt (AI agents must confirm with user first)")

	return cmd
}
