package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceResizeCmd creates the resize subcommand
func buildServiceResizeCmd(app *common.App) *cobra.Command {
	var cpu string
	var memory string
	var noWait bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "resize [service-id]",
		Short: "Resize a database service",
		Long: `Resize a database service by changing its CPU and memory allocation.

The service ID can be provided as an argument or will use the default service
from your configuration. This command changes the compute and memory resources
allocated to your database service.

The service may be temporarily unavailable during the resize operation. Note
that changing resources will affect your billing - increasing resources will
increase costs.

Allowed CPU/Memory Configurations:
  0.5 CPU (500m) / 2GB  |  1 CPU (1000m) / 4GB     |  2 CPU (2000m) / 8GB     |  4 CPU (4000m) / 16GB
  8 CPU (8000m) / 32GB  |  16 CPU (16000m) / 64GB  |  32 CPU (32000m) / 128GB

Note: You can specify both CPU and memory together, or specify only one (the other will be automatically configured).`,
		Example: `  # Resize default service to 2 CPU cores and 8GB memory
  tiger service resize --cpu 2000 --memory 8

  # Resize specific service to 4 CPU cores and 16GB memory
  tiger service resize svc-12345 --cpu 4000 --memory 16

  # Resize service using only CPU (memory will be auto-configured to 8GB)
  tiger service resize --cpu 2000

  # Resize service using only memory (CPU will be auto-configured to 4000m)
  tiger service resize --memory 16

  # Resize without waiting for completion (waits by default)
  tiger service resize --cpu 2000 --memory 8 --no-wait

  # Resize with custom wait timeout
  tiger service resize --cpu 2000 --memory 8 --wait-timeout 45m`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Determine service ID
			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			// Validate and normalize CPU/Memory configuration
			cpuMemoryCfg, err := common.ValidateAndNormalizeCPUMemory(cpu, memory)
			if err != nil {
				return err
			}

			// At least one of CPU or memory must be specified
			if cpuMemoryCfg == nil {
				return fmt.Errorf("must specify at least one of --cpu or --memory")
			}

			if err := common.CheckReadOnlyByServiceID(cmd.Context(), cfg, client, projectID, serviceID); err != nil {
				return err
			}

			// Display resize information
			cmd.PrintErrf("📐 Resizing service '%s' to %s...\n", serviceID, cpuMemoryCfg)

			// Prepare resize request
			resizeReq := api.ResizeInput{
				CPUMillis: *cpuMemoryCfg.CPUMillisString(),
				MemoryGbs: *cpuMemoryCfg.MemoryGBsString(),
			}

			// Make API call to resize service
			resp, err := client.ResizeServiceWithResponse(cmd.Context(), projectID, serviceID, resizeReq)
			if err != nil {
				return fmt.Errorf("failed to resize service: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}
			service := *resp.JSON202

			cmd.PrintErrf("✅ Resize request accepted for service '%s'!\n", serviceID)

			// If not waiting, return early
			if noWait {
				cmd.PrintErrln("💡 Use 'tiger service get' to check service status.")
				return nil
			}

			// Wait for resize to complete
			cmd.PrintErrf("⏳ Waiting for resize to complete (timeout: %v)...\n", waitTimeout)
			if err := common.WaitForService(cmd.Context(), common.WaitForServiceArgs{
				Client:    client,
				ProjectID: projectID,
				ServiceID: serviceID,
				Handler: &common.StatusWaitHandler{
					TargetStatus: "READY",
					Service:      &service,
				},
				Input:      cmd.InOrStdin(),
				Output:     cmd.ErrOrStderr(),
				Timeout:    waitTimeout,
				TimeoutMsg: "service may still be resizing",
			}); err != nil {
				// Return error for sake of exit code, but silence since we already output it
				cmd.PrintErrf("❌ Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			cmd.PrintErrf("🎉 Service '%s' has been successfully resized to %s!\n", serviceID, cpuMemoryCfg)
			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&cpu, "cpu", "", "CPU allocation in millicores")
	cmd.Flags().StringVar(&memory, "memory", "", "Memory allocation in gigabytes")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for resize operation to complete")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")

	cmd.RegisterFlagCompletionFunc("cpu", cpuCompletion(common.GetAllowedResizeCPUMemoryConfigs()))
	cmd.RegisterFlagCompletionFunc("memory", memoryCompletion(common.GetAllowedResizeCPUMemoryConfigs()))

	return cmd
}
