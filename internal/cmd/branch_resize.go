package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchResizeCmd creates the resize subcommand
func buildBranchResizeCmd(app *common.App) *cobra.Command {
	var cpu string
	var memory string
	var noWait bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "resize <branch-id>",
		Short: "Resize a branch",
		Long: `Resize a branch by changing its CPU and memory allocation. A branch's storage is
shared with its parent service and isn't resized.

The branch may be temporarily unavailable during the resize.

` + branchCPUMemoryHelp + `

Note: You can specify both CPU and memory together, or specify only one (the other will be automatically configured).`,
		Example: `  # Resize a branch to 2 CPU cores and 16GB memory
  tiger branch resize br-12345 --cpu 2000 --memory 16

  # Resize a branch using only CPU (memory will be auto-configured)
  tiger branch resize br-12345 --cpu 2000

  # Resize without waiting for completion
  tiger branch resize br-12345 --cpu 2000 --no-wait`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: branchIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			branchID, err := getBranchID(args)
			if err != nil {
				return err
			}

			if cpu == "" && memory == "" {
				return errors.New("must specify at least one of --cpu or --memory")
			}
			cpuMemoryCfg, err := common.BranchCPUMemoryConfigs().ValidateAndNormalize(cpu, memory)
			if err != nil {
				return err
			}

			resolved, err := resolveBranchForWrite(cmd.Context(), cfg, client, projectID, branchID)
			if err != nil {
				return err
			}

			cmd.PrintErrf("Resizing branch %s to %s...\n", branchLabel(*resolved), cpuMemoryCfg)

			resp, err := client.ResizeBranchWithResponse(cmd.Context(), projectID, resolved.ID, api.ResizeInput{
				CPUMillis: *cpuMemoryCfg.CPUMillisString(),
				MemoryGbs: *cpuMemoryCfg.MemoryGBsString(),
			})
			if err != nil {
				return fmt.Errorf("failed to resize branch: %w", err)
			}
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}
			branch := *resp.JSON202

			cmd.PrintErrf("Resize request accepted for branch %s.\n", branchLabel(*resolved))

			if noWait {
				cmd.PrintErrln("Use 'tiger branch get' to check branch status.")
				return nil
			}

			cmd.PrintErrf("Waiting for resize to complete (timeout: %v)...\n", waitTimeout)
			if err := common.WaitForBranch(cmd.Context(), common.WaitForBranchArgs{
				Client:       client,
				ProjectID:    projectID,
				BranchID:     resolved.ID,
				Branch:       &branch,
				TargetStatus: api.DeployStatusREADY,
				Input:        cmd.InOrStdin(),
				Output:       cmd.ErrOrStderr(),
				Timeout:      waitTimeout,
				TimeoutMsg:   "branch may still be resizing",
			}); err != nil {
				// Return error for sake of exit code, but silence since we already output it
				cmd.PrintErrf("Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			cmd.PrintErrf("Branch resized to %s.\n", cpuMemoryCfg)
			return nil
		},
	}

	cmd.Flags().StringVar(&cpu, "cpu", "", "CPU allocation in millicores")
	cmd.Flags().StringVar(&memory, "memory", "", "Memory allocation in gigabytes")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for resize operation to complete")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")

	registerFlagCompletion(cmd, "cpu", branchCPUCompletion)
	registerFlagCompletion(cmd, "memory", branchMemoryCompletion)

	return cmd
}
