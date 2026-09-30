package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildAllowListDeleteCmd creates the allowlist delete subcommand.
func buildAllowListDeleteCmd(app *common.App) *cobra.Command {
	var deleteConfirm bool

	cmd := &cobra.Command{
		Use:     "delete <allow-list-id>",
		Aliases: []string{"rm"},
		Short:   "Delete an IP allow list",
		Long: `Delete an IP allow list.

This operation is irreversible. It is refused if the IP allow list is still
attached to any service — detach it from every service first. By default,
you will be prompted to type the IP allow list ID to confirm deletion,
unless you use the --confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.`,
		Example: `  # Delete an IP allow list (with confirmation prompt)
  tiger allowlist delete 1234567890

  # Delete an IP allow list without a confirmation prompt
  tiger allowlist delete 1234567890 --confirm`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			if cfg.ReadOnly.BlocksAll() {
				return common.ErrReadOnly
			}

			if !deleteConfirm {
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
					return fmt.Errorf("TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt")
				}
				cmd.PrintErrf("Are you sure you want to delete IP allow list '%s'? This operation cannot be undone.\n", args[0])
				cmd.PrintErrf("Type the IP allow list ID '%s' to confirm: ", args[0])
				confirmation, err := util.ReadLine(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				if confirmation != args[0] {
					cmd.PrintErrln("Delete operation cancelled.")
					return nil
				}
			}

			resp, err := client.DeleteAllowListWithResponse(cmd.Context(), projectID, args[0])
			if err != nil {
				return fmt.Errorf("failed to delete IP allow list: %w", err)
			}

			if resp.StatusCode() != http.StatusNoContent {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			cmd.PrintErrf("IP allow list '%s' deleted.\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Skip confirmation prompt (AI agents must confirm with user first)")

	return cmd
}
