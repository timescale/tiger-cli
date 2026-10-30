package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildBranchDeleteCmd creates the delete subcommand
func buildBranchDeleteCmd(app *common.App) *cobra.Command {
	var confirm bool

	cmd := &cobra.Command{
		Use:     "delete <name-or-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a branch",
		Long: `Delete a branch permanently, along with any changes made on it. The parent
service is not affected.

The branch can be given by ID or name.

This operation is irreversible. By default, you will be prompted to type the
branch ID to confirm deletion — the ID, not the name — unless you use the
--confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.`,
		Example: `  # Delete a branch (with confirmation prompt)
  tiger branch delete br-12345

  # Delete a branch without confirmation prompt
  tiger branch delete br-12345 --confirm`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: branchRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			branchRef, err := getBranchRef(args)
			if err != nil {
				return err
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Gated here, ahead of the confirmation prompt, so read-only mode
			// refuses without first asking the user to type the branch ID.
			branch, err := resolveBranchForWrite(cmd.Context(), cfg, client, projectID, branchRef)
			if err != nil {
				return err
			}

			if !confirm {
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
					return errors.New("TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt")
				}
				cmd.PrintErrf("Are you sure you want to delete branch %s? This operation cannot be undone.\n", branchLabel(*branch))
				cmd.PrintErrf("Type the branch ID '%s' to confirm: ", branch.ID)
				confirmation, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if confirmation != branch.ID {
					cmd.PrintErrln("Delete operation cancelled.")
					return nil
				}
			}

			if err := common.Status202(client.DeleteBranchWithResponse(cmd.Context(), projectID, branch.ServiceID, branch.ID)); err != nil {
				return err
			}

			cmd.PrintErrf("Branch %s deleted.\n", branchLabel(*branch))
			return nil
		},
	}

	cmd.Flags().BoolVar(&confirm, "confirm", false, "Skip confirmation prompt (AI agents must confirm with user first)")

	return cmd
}
