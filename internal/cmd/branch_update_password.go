package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildBranchUpdatePasswordCmd creates the update-password subcommand
func buildBranchUpdatePasswordCmd(app *common.App) *cobra.Command {
	var newPassword string
	var autoGenerate bool

	cmd := &cobra.Command{
		Use:   "update-password [branch-id]",
		Short: "Update the master password for a branch",
		Long: `Update the master password for the 'tsdbadmin' user of a branch.

A branch starts with its parent service's password. Changing it here affects
only the branch, and changing the parent's password doesn't change the
branch's.

The branch is given by ID as an argument, or will use the default from your
configuration.`,
		Example: `  # Update the default branch's password, interactively prompts
  tiger branch update-password

  # Update a specific branch's password
  tiger branch update-password br-12345 --new-password new-secure-password

  # Update password using environment variable (TIGER_NEW_PASSWORD)
  export TIGER_NEW_PASSWORD="new-secure-password"
  tiger branch update-password br-12345

  # Auto-generate a secure password
  tiger branch update-password br-12345 --auto-generate`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: branchIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// The password comes from the flag, falling back to the env var
			password := newPassword
			if password == "" {
				password = os.Getenv("TIGER_NEW_PASSWORD")
			}
			if autoGenerate && password != "" {
				return errors.New("cannot use --auto-generate and --new-password together")
			}

			branchID, err := getBranchID(cfg, args)
			if err != nil {
				return err
			}

			ctx := cmd.Context()

			// Gated here, ahead of any prompt or write. The branch is needed
			// to save the new password under it.
			branch, err := resolveBranchForWrite(ctx, cfg, client, projectID, branchID)
			if err != nil {
				return err
			}

			if !autoGenerate && password == "" {
				if !util.IsTerminal(cmd.InOrStdin()) || !util.IsTerminal(cmd.ErrOrStderr()) {
					return errors.New("TTY not detected - use --new-password flag, --auto-generate flag, or TIGER_NEW_PASSWORD environment variable")
				}
				cmd.PrintErr("Enter new password (leave empty to generate): ")
				password, err = util.ReadPassword(ctx, cmd.InOrStdin())
				cmd.PrintErrln() // newline after password entry
				if err != nil {
					return fmt.Errorf("error reading password: %w", err)
				}
			}
			if password == "" {
				if password, err = common.GenerateSecurePassword(32); err != nil {
					return fmt.Errorf("failed to generate new password: %w", err)
				}
			}

			resp, err := client.UpdateBranchPasswordWithResponse(ctx, projectID, branch.ID, api.UpdatePasswordInput{Password: password})
			if err != nil {
				return fmt.Errorf("failed to update password: %w", err)
			}
			if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if result, err := common.SavePasswordWithResult(cfg, common.BranchAsService(*branch), password, "tsdbadmin"); err != nil {
				cmd.PrintErrf("Warning: could not save password: %v\n", err)
			} else if result.Success {
				cmd.PrintErrf("%s\n", result.Message)
			}

			cmd.PrintErrf("Password updated for tsdbadmin\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&newPassword, "new-password", "", "New password for the tsdbadmin user (can also be set via TIGER_NEW_PASSWORD env var)")
	cmd.Flags().BoolVar(&autoGenerate, "auto-generate", false, "Auto-generate a secure password")
	cmd.MarkFlagsMutuallyExclusive("new-password", "auto-generate")
	return cmd
}
