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

// buildServiceStopCmd creates the stop subcommand
func buildServiceStopCmd() *cobra.Command {
	var stopNoWait bool
	var stopWaitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "stop [service-id]",
		Short: "Stop a running database service",
		Long: `Stop a running database service.

This operation stops a service that is currently active/running. The service will transition to an inactive state and will no longer accept connections.

Examples:
  # Stop a service (waits for completion by default)
  tiger service stop svc-12345

  # Stop service without waiting for completion
  tiger service stop svc-12345 --no-wait

  # Stop service with custom wait timeout
  tiger service stop svc-12345 --wait-timeout 10m`,
		ValidArgsFunction: serviceIDCompletion,
		Args:              cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config and API client
			cfg, err := common.LoadConfig(cmd.Context())
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}

			if err := common.CheckReadOnly(cfg.Config); err != nil {
				cmd.SilenceUsage = true
				return err
			}

			// Determine source service ID
			serviceID, err := getServiceID(cfg.Config, args)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			// Make the stop request
			resp, err := cfg.Client.StopServiceWithResponse(
				context.Background(),
				api.ProjectId(cfg.ProjectID),
				api.ServiceId(serviceID),
			)
			if err != nil {
				return fmt.Errorf("failed to stop Service: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}
			service := *resp.JSON202

			statusOutput := cmd.ErrOrStderr()
			fmt.Fprintf(statusOutput, "⏹️  Stop request accepted for service '%s'.\n", serviceID)

			// If not waiting, return early
			if stopNoWait {
				fmt.Fprintln(statusOutput, "💡 Use 'tiger service get' to check service status.")
				return nil
			}

			// Wait for service to become paused
			fmt.Fprintf(statusOutput, "⏳ Waiting for service to stop (timeout: %v)...\n", stopWaitTimeout)
			if err := common.WaitForService(cmd.Context(), common.WaitForServiceArgs{
				Client:    cfg.Client,
				ProjectID: cfg.ProjectID,
				ServiceID: serviceID,
				Handler: &common.StatusWaitHandler{
					TargetStatus: "PAUSED",
					Service:      &service,
				},
				Output:     statusOutput,
				Timeout:    stopWaitTimeout,
				TimeoutMsg: "service may still be stopping",
			}); err != nil {
				// Return error for sake of exit code, but log ourselves for sake of icon
				fmt.Fprintf(statusOutput, "❌ Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			fmt.Fprintf(statusOutput, "✅ Service has been successfully stopped!\n")
			return nil
		},
	}

	// Add flags
	cmd.Flags().BoolVar(&stopNoWait, "no-wait", false, "Don't wait for the operation to complete")
	cmd.Flags().DurationVar(&stopWaitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")

	return cmd
}
