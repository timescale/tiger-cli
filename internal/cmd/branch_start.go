package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchStartCmd creates the start subcommand
func buildBranchStartCmd(app *common.App) *cobra.Command {
	var noWait bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:     "start <name-or-id>",
		Aliases: []string{"resume"},
		Short:   "Start a stopped branch",
		Long: `Start a stopped branch.

Starting a branch resumes charging for its compute.

The branch can be given by ID or name.`,
		Example: `  # Start a branch (waits for completion by default)
  tiger branch start br-12345

  # Start a branch without waiting for completion
  tiger branch start br-12345 --no-wait`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: branchRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateWaitTimeout(waitTimeout); err != nil {
				return err
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			branchRef, err := getBranchRef(args)
			if err != nil {
				return err
			}

			resolved, err := resolveBranchForWrite(cmd.Context(), cfg, client, projectID, branchRef)
			if err != nil {
				return err
			}

			branch, err := common.JSON202(client.StartBranchWithResponse(cmd.Context(), projectID, resolved.ServiceID, resolved.ID))
			if err != nil {
				return err
			}

			cmd.PrintErrf("Start request accepted for branch %s.\n", branchLabel(*resolved))

			if noWait {
				cmd.PrintErrf("Use 'tiger branch get %s' to check branch status.\n", resolved.ID)
				return nil
			}

			cmd.PrintErrf("Waiting for branch to start (timeout: %v)...\n", waitTimeout)
			if err := common.WaitForBranch(cmd.Context(), common.WaitForBranchArgs{
				Client:       client,
				ProjectID:    projectID,
				ServiceID:    resolved.ServiceID,
				BranchID:     resolved.ID,
				Branch:       &branch,
				TargetStatus: api.DeployStatusREADY,
				Input:        cmd.InOrStdin(),
				Output:       cmd.ErrOrStderr(),
				Timeout:      waitTimeout,
				TimeoutMsg:   "branch may still be starting",
			}); err != nil {
				// Return error for sake of exit code, but silence since we already output it
				cmd.PrintErrf("Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			cmd.PrintErrln("Branch started.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for the operation to complete")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for the operation to complete (e.g., 30m, 1h30m, 90s)")

	return cmd
}
