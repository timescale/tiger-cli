package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchResizeCmd creates the resize subcommand
func buildBranchResizeCmd(app *common.App) *cobra.Command {
	var cpu string
	var memory string
	var iops int
	var noWait bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "resize <name-or-id>",
		Short: "Resize a branch",
		Long: `Resize a branch by changing its CPU, memory, and provisioned IOPS allocation.
A branch's storage is shared with its parent service and isn't resized.

The branch can be given by ID or name. It may be temporarily unavailable
during the resize.

` + branchCPUMemoryHelp + `

Note: You can specify both CPU and memory together, or specify only one (the
other will be automatically configured).

The allowed --iops values depend on your plan and the branch's CPU/memory
allocation. See the Tiger Data documentation for details. If only --iops is
given, the branch keeps its current CPU and memory.`,
		Example: `  # Resize a branch to 2 CPU cores and 16GB memory
  tiger branch resize br-12345 --cpu 2000 --memory 16

  # Resize a branch using only CPU (memory will be auto-configured)
  tiger branch resize br-12345 --cpu 2000

  # Change only the provisioned IOPS
  tiger branch resize br-12345 --iops 16000

  # Resize without waiting for completion
  tiger branch resize br-12345 --cpu 2000 --no-wait`,
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

			iopsSet := cmd.Flags().Changed("iops")
			if cpu == "" && memory == "" && !iopsSet {
				return errors.New("must specify at least one of --cpu, --memory, or --iops")
			}
			cpuMemoryCfg, err := common.FluidCPUMemoryConfigs().ValidateAndNormalize(cpu, memory)
			if err != nil {
				return err
			}

			resolved, err := resolveBranchForWrite(cmd.Context(), cfg, client, projectID, branchRef)
			if err != nil {
				return err
			}

			// An IOPS-only resize omits CPU and memory, which the API leaves
			// unchanged.
			resizeReq := api.BranchResizeInput{
				CPUMillis: cpuMemoryCfg.CPUMillisString(),
				MemoryGbs: cpuMemoryCfg.MemoryGBsString(),
			}
			var targets []string
			if cpuMemoryCfg != nil {
				targets = append(targets, cpuMemoryCfg.String())
			}
			if iopsSet {
				resizeReq.Iops = &iops
				targets = append(targets, fmt.Sprintf("%d IOPS", iops))
			}
			target := strings.Join(targets, ", ")

			cmd.PrintErrf("Resizing branch %s to %s...\n", branchLabel(*resolved), target)

			resp, err := client.ResizeBranchWithResponse(cmd.Context(), projectID, resolved.ServiceID, resolved.ID, resizeReq)
			if err != nil {
				return fmt.Errorf("failed to resize branch: %w", err)
			}
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON202 == nil {
				return errors.New("empty response from API")
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
				ServiceID:    resolved.ServiceID,
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

			cmd.PrintErrf("Branch resized to %s.\n", target)
			return nil
		},
	}

	cmd.Flags().StringVar(&cpu, "cpu", "", "CPU allocation in millicores")
	cmd.Flags().StringVar(&memory, "memory", "", "Memory allocation in gigabytes")
	cmd.Flags().IntVar(&iops, "iops", 0, "Provisioned IOPS")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for resize operation to complete")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")

	registerFlagCompletion(cmd, "cpu", branchCPUCompletion)
	registerFlagCompletion(cmd, "memory", branchMemoryCompletion)
	registerFlagCompletion(cmd, "iops", iopsCompletion)

	return cmd
}
