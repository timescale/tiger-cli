package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchRenameCmd creates the rename subcommand
func buildBranchRenameCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <branch-id> <new-name>",
		Short: "Rename a branch",
		Long: `Rename a branch.

Only the branch's name changes: its ID, endpoint, and data are untouched, so
existing connections and connection strings keep working.

Both the branch ID and the new name are required. There is no default
fallback, since a single argument would be ambiguous between the branch to
rename and the name to give it.`,
		Example: `  # Rename a branch
  tiger branch rename br-12345 feature/rbac`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: branchIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			branchID, newName := args[0], strings.TrimSpace(args[1])

			if branchID == "" {
				return errors.New("branch ID is required")
			}
			if newName == "" {
				return errors.New("new name cannot be empty")
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			branch, err := resolveBranchForWrite(cmd.Context(), cfg, client, projectID, branchID)
			if err != nil {
				return err
			}

			resp, err := client.RenameBranchWithResponse(cmd.Context(), projectID, branch.ID, api.BranchRename{Name: newName})
			if err != nil {
				return fmt.Errorf("failed to rename branch: %w", err)
			}
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON200 == nil {
				return errors.New("empty response from API")
			}
			renamed := *resp.JSON200

			cmd.Printf("Renamed branch %s to '%s'.\n", branchLabel(*branch), renamed.Name)
			return nil
		},
	}

	return cmd
}
