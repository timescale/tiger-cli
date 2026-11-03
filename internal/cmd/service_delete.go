package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceDeleteCmd creates the delete subcommand
func buildServiceDeleteCmd(app *common.App) *cobra.Command {
	var confirm bool

	long := `Delete a database service permanently.

The service can be given by ID or name, but must be given explicitly: there is
no fallback to the default service.

This operation is irreversible. By default, you will be prompted to type the
service ID to confirm deletion — the ID, not the name — unless you use the
--confirm flag.
`
	// Branches are a preview, so only the experimental help mentions them. The
	// command lists them regardless: they're deleted either way.
	if app.Experimental {
		long += `
Deleting a service also deletes all of its branches. They are listed in the
confirmation prompt and again once the service is deleted.
`
	}
	long += `
Note for AI agents: Always confirm with the user before performing this destructive operation.`

	cmd := &cobra.Command{
		Use:     "delete <name-or-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a database service",
		Long:    long,
		Example: `  # Delete a service (with confirmation prompt)
  tiger service delete svc-12345

  # Delete service without confirmation prompt
  tiger service delete svc-12345 --confirm`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Require an explicit service for safety: no default fallback.
			if len(args) < 1 || args[0] == "" {
				return errors.New("service name or ID is required")
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

			if !confirm && (!util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr())) {
				return errors.New("TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt")
			}

			branches := listBranchesToDelete(cmd, client, projectID, *service)

			// Prompt for confirmation unless --confirm is used
			if !confirm {
				if len(branches) > 0 {
					cmd.PrintErrf("This service has %s, which will also be deleted:\n", common.FormatBranchCount(len(branches)))
					printBranchLabels(cmd, branches)
				}
				// Show both forms, but take only the ID: a name can move to a
				// different service through a rename, so it is the wrong
				// thing to authorize a delete with.
				cmd.PrintErrf("Are you sure you want to delete service %s? This operation cannot be undone.\n", serviceLabel(*service))
				cmd.PrintErrf("Type the service ID '%s' to confirm: ", service.ServiceID)
				confirmation, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if confirmation != service.ServiceID {
					cmd.PrintErrln("Delete operation cancelled.")
					return nil
				}
			}

			// Make the delete request
			if err := common.Status202(client.DeleteServiceWithResponse(
				cmd.Context(),
				api.ProjectID(projectID),
				api.ServiceID(service.ServiceID),
			)); err != nil {
				return err
			}

			if len(branches) == 0 {
				cmd.PrintErrf("Service %s deleted.\n", serviceLabel(*service))
				return nil
			}
			cmd.PrintErrf("Service %s deleted, along with its %s:\n", serviceLabel(*service), common.FormatBranchCount(len(branches)))
			printBranchLabels(cmd, branches)
			return nil
		},
	}

	cmd.Flags().BoolVar(&confirm, "confirm", false, "Skip confirmation prompt (AI agents must confirm with user first)")

	return cmd
}

// listBranchesToDelete returns the branches that deleting service takes with
// it, so the command can show them before and after the delete. The API
// deletes them without asking, so this isn't gated on the experimental env
// var: whoever has branches gets told. A failed listing is a warning rather
// than an error, so the delete can still go ahead.
func listBranchesToDelete(cmd *cobra.Command, client api.ClientWithResponsesInterface, projectID string, service api.Service) []api.Branch {
	if !common.CanHaveBranches(service) {
		return nil
	}
	branches, err := common.JSON200(client.GetBranchesWithResponse(cmd.Context(), projectID, service.ServiceID, nil))
	if err != nil {
		cmd.PrintErrf("Warning: failed to list branches: %v\n", err)
		cmd.PrintErrln("Any branches of this service will also be deleted.")
		return nil
	}
	return branches
}

// printBranchLabels prints one indented line per branch.
func printBranchLabels(cmd *cobra.Command, branches []api.Branch) {
	for _, branch := range branches {
		cmd.PrintErrf("  %s\n", branchLabel(branch))
	}
}
