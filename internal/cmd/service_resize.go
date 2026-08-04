package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceResizeCmd creates the resize subcommand
func buildServiceResizeCmd() *cobra.Command {
	var resizeCPU string
	var resizeMemory string
	var resizeNoWait bool
	var resizeWaitTimeout time.Duration

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

Examples:
  # Resize default service to 2 CPU cores and 8GB memory
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
  tiger service resize --cpu 2000 --memory 8 --wait-timeout 45m

Allowed CPU/Memory Configurations:
  0.5 CPU (500m) / 2GB  |  1 CPU (1000m) / 4GB     |  2 CPU (2000m) / 8GB     |  4 CPU (4000m) / 16GB
  8 CPU (8000m) / 32GB  |  16 CPU (16000m) / 64GB  |  32 CPU (32000m) / 128GB

Note: You can specify both CPU and memory together, or specify only one (the other will be automatically configured).`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config and API client
			cfg, err := common.LoadConfig(cmd.Context(), cmd.Flags())
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}

			if err := common.CheckReadOnly(cfg.Config); err != nil {
				cmd.SilenceUsage = true
				return err
			}

			// Determine service ID
			serviceID, err := getServiceID(cfg.Config, args)
			if err != nil {
				return err
			}

			// Validate and normalize CPU/Memory configuration
			cpuMemoryCfg, err := common.ValidateAndNormalizeCPUMemory(resizeCPU, resizeMemory)
			if err != nil {
				return err
			}

			// At least one of CPU or memory must be specified
			if cpuMemoryCfg == nil {
				return fmt.Errorf("must specify at least one of --cpu or --memory")
			}

			cmd.SilenceUsage = true

			// Display resize information
			statusOutput := cmd.ErrOrStderr()
			fmt.Fprintf(statusOutput, "📐 Resizing service '%s' to %s...\n", serviceID, cpuMemoryCfg)

			// Prepare resize request
			resizeReq := api.ResizeInput{
				CpuMillis: *cpuMemoryCfg.CPUMillisString(),
				MemoryGbs: *cpuMemoryCfg.MemoryGBsString(),
			}

			// Make API call to resize service
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			resp, err := cfg.Client.ResizeServiceWithResponse(ctx, cfg.ProjectID, serviceID, resizeReq)
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

			fmt.Fprintf(statusOutput, "✅ Resize request accepted for service '%s'!\n", serviceID)

			// If not waiting, return early
			if resizeNoWait {
				fmt.Fprintln(statusOutput, "💡 Use 'tiger service get' to check service status.")
				return nil
			}

			// Wait for resize to complete
			fmt.Fprintf(statusOutput, "⏳ Waiting for resize to complete (timeout: %v)...\n", resizeWaitTimeout)
			if err := common.WaitForService(cmd.Context(), common.WaitForServiceArgs{
				Client:    cfg.Client,
				ProjectID: cfg.ProjectID,
				ServiceID: serviceID,
				Handler: &common.StatusWaitHandler{
					TargetStatus: "READY",
					Service:      &service,
				},
				Output:     statusOutput,
				Timeout:    resizeWaitTimeout,
				TimeoutMsg: "service may still be resizing",
			}); err != nil {
				// Return error for sake of exit code, but silence since we already output it
				fmt.Fprintf(statusOutput, "❌ Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			fmt.Fprintf(statusOutput, "🎉 Service '%s' has been successfully resized to %s!\n", serviceID, cpuMemoryCfg)
			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&resizeCPU, "cpu", "", "CPU allocation in millicores")
	cmd.Flags().StringVar(&resizeMemory, "memory", "", "Memory allocation in gigabytes")
	cmd.Flags().BoolVar(&resizeNoWait, "no-wait", false, "Don't wait for resize operation to complete")
	cmd.Flags().DurationVar(&resizeWaitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")

	return cmd
}
