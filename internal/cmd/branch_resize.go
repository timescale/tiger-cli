package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildBranchResizeCmd creates the resize subcommand
func buildBranchResizeCmd(app *common.App) *cobra.Command {
	var cpu string
	var memory string
	var iops int

	cmd := &cobra.Command{
		Use:   "resize <name-or-id>",
		Short: "Resize a branch",
		Long: `Resize a branch by changing its CPU, memory, and provisioned IOPS allocation.

The branch can be given by ID or name. The command returns once the resize
request is accepted, without waiting for the resize to complete, which may take
several minutes. The branch may be temporarily unavailable during the resize.

` + branchCPUMemoryHelp + `

Note: You can specify both CPU and memory together, or specify only one (the
other will be automatically configured).

The allowed --iops values depend on your plan and the branch's CPU/memory
allocation. See the Tiger Data documentation for details. If only --iops is
given, the branch keeps its current CPU and memory. If --cpu or --memory is
given without --iops, the IOPS is set to the baseline for the new size.`,
		Example: `  # Resize a branch to 2 CPU cores and 16GB memory
  tiger branch resize br-12345 --cpu 2 --memory 16

  # Resize a branch using only CPU (memory will be auto-configured)
  tiger branch resize br-12345 --cpu 2

  # Change only the provisioned IOPS
  tiger branch resize br-12345 --iops 16000`,
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
			if iopsSet {
				resizeReq.Iops = &iops
			}

			cmd.PrintErrf("Resizing branch %s...\n", branchLabel(*resolved))

			resp, err := client.ResizeBranchWithResponse(cmd.Context(), projectID, resolved.ServiceID, resolved.ID, resizeReq)
			if err != nil {
				return fmt.Errorf("failed to resize branch: %w", err)
			}
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}

			// Report the size from the response rather than the request, since
			// the API fills in what the request left out (e.g. the baseline
			// IOPS for the new CPU/memory).
			branch := resp.JSON202
			size := formatSize(common.CPUMemoryConfig{
				CPUMillis: branch.CPUMillis,
				MemoryGBs: branch.MemoryGbs,
			}, branch.Iops)
			cmd.PrintErrf("Resize request accepted: %s.\n", size)
			cmd.PrintErrln("The resize is in progress and may take several minutes to complete.")
			return nil
		},
	}

	cmd.Flags().StringVar(&cpu, "cpu", "", "CPU allocation in cores")
	cmd.Flags().StringVar(&memory, "memory", "", "Memory allocation in gigabytes")
	cmd.Flags().IntVar(&iops, "iops", 0, "Provisioned IOPS")

	registerFlagCompletion(cmd, "cpu", branchCPUCompletion)
	registerFlagCompletion(cmd, "memory", branchMemoryCompletion)
	registerFlagCompletion(cmd, "iops", iopsCompletion)

	return cmd
}
