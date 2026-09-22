package cmd

import (
	"errors"
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
		Use:     "delete [name-or-id]",
		Aliases: []string{"rm"},
		Short:   "Delete a database service",
		Long: `Delete a database service permanently.

The service can be given by ID or name, but must be given explicitly: there is
no fallback to the default service.

This operation is irreversible. By default, you will be prompted to type the
service ID to confirm deletion — the ID, not the name — unless you use the
--confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.`,
		Example: `  # Delete a service (with confirmation prompt)
  tiger service delete svc-12345

  # Delete service without confirmation prompt
  tiger service delete svc-12345 --confirm`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Require an explicit service for safety: no default fallback.
			if len(args) < 1 || args[0] == "" {
				return errors.New("service is required")
			}
			serviceArg := args[0]

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Gated here, ahead of the confirmation prompt, so read-only mode
			// refuses without first asking the user to type the service ID.
			service, err := resolveServiceForWrite(cmd.Context(), cfg, client, projectID, argServiceRef(serviceArg))
			if err != nil {
				return err
			}
			serviceID := service.ServiceID

			// Prompt for confirmation unless --confirm is used
			if !confirm {
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
					return fmt.Errorf("TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt")
				}
				// Show both forms, but take only the ID: a name can move to a
				// different service through a rename, so it is the wrong
				// thing to authorize a delete with.
				cmd.PrintErrf("Are you sure you want to delete service '%s' (%s)? This operation cannot be undone.\n", service.Name, serviceID)
				cmd.PrintErrf("Type the service ID '%s' to confirm: ", serviceID)
				confirmation, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if confirmation != serviceID {
					cmd.PrintErrln("Delete operation cancelled.")
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

			cmd.PrintErrf("Service '%s' deleted.\n", serviceID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&confirm, "confirm", false, "Skip confirmation prompt (AI agents must confirm with user first)")

	return cmd
}
