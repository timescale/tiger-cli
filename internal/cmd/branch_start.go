package cmd

import (
	"fmt"
	"net/http"
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
		Use:     "start [branch-id]",
		Aliases: []string{"resume"},
		Short:   "Start a stopped branch",
		Long: `Start a stopped branch.

Starting a branch resumes charging for its compute.

The branch is given by ID as an argument, or will use the default from your
configuration.`,
		Example: `  # Start a branch (waits for completion by default)
  tiger branch start br-12345

  # Start a branch without waiting for completion
  tiger branch start br-12345 --no-wait`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: branchIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			branchID, err := getBranchID(cfg, args)
			if err != nil {
				return err
			}

			resolved, err := resolveBranchForWrite(cmd.Context(), cfg, client, projectID, branchID)
			if err != nil {
				return err
			}

			resp, err := client.StartBranchWithResponse(cmd.Context(), projectID, resolved.ID)
			if err != nil {
				return fmt.Errorf("failed to start branch: %w", err)
			}
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}
			branch := *resp.JSON202

			cmd.PrintErrf("Start request accepted for branch %s.\n", branchLabel(*resolved))

			if noWait {
				cmd.PrintErrln("Use 'tiger branch get' to check branch status.")
				return nil
			}

			cmd.PrintErrf("Waiting for branch to start (timeout: %v)...\n", waitTimeout)
			if err := common.WaitForBranch(cmd.Context(), common.WaitForBranchArgs{
				Client:       client,
				ProjectID:    projectID,
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
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")

	return cmd
}
