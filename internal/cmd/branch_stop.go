package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchStopCmd creates the stop subcommand
func buildBranchStopCmd(app *common.App) *cobra.Command {
	var noWait bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:     "stop <name-or-id>",
		Aliases: []string{"pause"},
		Short:   "Stop a running branch",
		Long: `Stop a running branch.

A stopped branch no longer accepts connections and stops charging for compute,
but keeps the changes made on it, whose storage is still billed.

The branch can be given by ID or name.`,
		Example: `  # Stop a branch (waits for completion by default)
  tiger branch stop br-12345

  # Stop a branch without waiting for completion
  tiger branch stop br-12345 --no-wait`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: branchRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			resp, err := client.StopBranchWithResponse(cmd.Context(), projectID, resolved.ServiceID, resolved.ID)
			if err != nil {
				return fmt.Errorf("failed to stop branch: %w", err)
			}
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}
			branch := *resp.JSON202

			cmd.PrintErrf("Stop request accepted for branch %s.\n", branchLabel(*resolved))

			if noWait {
				cmd.PrintErrln("Use 'tiger branch get' to check branch status.")
				return nil
			}

			cmd.PrintErrf("Waiting for branch to stop (timeout: %v)...\n", waitTimeout)
			if err := common.WaitForBranch(cmd.Context(), common.WaitForBranchArgs{
				Client:       client,
				ProjectID:    projectID,
				ServiceID:    resolved.ServiceID,
				BranchID:     resolved.ID,
				Branch:       &branch,
				TargetStatus: api.DeployStatusPAUSED,
				Input:        cmd.InOrStdin(),
				Output:       cmd.ErrOrStderr(),
				Timeout:      waitTimeout,
				TimeoutMsg:   "branch may still be stopping",
			}); err != nil {
				// Return error for sake of exit code, but silence since we already output it
				cmd.PrintErrf("Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			cmd.PrintErrln("Branch stopped.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for the operation to complete")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")

	return cmd
}
