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

// buildServiceStartCmd creates the start subcommand
func buildServiceStartCmd() *cobra.Command {
	var startNoWait bool
	var startWaitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "start [service-id]",
		Short: "Start a stopped database service",
		Long: `Start a stopped database service.

This operation starts a service that is currently in an inactive/stopped state. The service will transition to an active state and become available for connections.

Examples:
  # Start a service (waits for completion by default)
  tiger service start svc-12345

  # Start service without waiting for completion
  tiger service start svc-12345 --no-wait

  # Start service with custom wait timeout
  tiger service start svc-12345 --wait-timeout 10m`,
		ValidArgsFunction: serviceIDCompletion,
		Args:              cobra.MaximumNArgs(1),
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

			// Determine source service ID
			serviceID, err := getServiceID(cfg.Config, args)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			// Make the start request
			resp, err := cfg.Client.StartServiceWithResponse(
				context.Background(),
				api.ProjectId(cfg.ProjectID),
				api.ServiceId(serviceID),
			)
			if err != nil {
				return fmt.Errorf("failed to start Service: %w", err)
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
			fmt.Fprintf(statusOutput, "▶️  Start request accepted for service '%s'.\n", serviceID)

			// If not waiting, return early
			if startNoWait {
				fmt.Fprintln(statusOutput, "💡 Use 'tiger service get' to check service status.")
				return nil
			}

			// Wait for service to become ready
			fmt.Fprintf(statusOutput, "⏳ Waiting for service to start (wait timeout: %v)...\n", startWaitTimeout)
			if err := common.WaitForService(cmd.Context(), common.WaitForServiceArgs{
				Client:    cfg.Client,
				ProjectID: cfg.ProjectID,
				ServiceID: serviceID,
				Handler: &common.StatusWaitHandler{
					TargetStatus: "READY",
					Service:      &service,
				},
				Output:     statusOutput,
				Timeout:    startWaitTimeout,
				TimeoutMsg: "service may still be starting",
			}); err != nil {
				// Return error for sake of exit code, but log ourselves for sake of icon
				fmt.Fprintf(statusOutput, "❌ Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			fmt.Fprintf(statusOutput, "✅ Service has been successfully started!\n")
			return nil
		},
	}

	// Add flags
	cmd.Flags().BoolVar(&startNoWait, "no-wait", false, "Don't wait for the operation to complete")
	cmd.Flags().DurationVar(&startWaitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")

	return cmd
}
