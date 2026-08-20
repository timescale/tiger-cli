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
func buildServiceStartCmd(app *common.App) *cobra.Command {
	var startNoWait bool
	var startWaitTimeout time.Duration

	cmd := &cobra.Command{
		Use:     "start [service-id]",
		Aliases: []string{"resume"},
		Short:   "Start a stopped database service",
		Long: `Start a stopped database service.

This operation starts a service that is currently in an inactive/stopped state. The service will transition to an active state and become available for connections.

Examples:
  # Start a service (waits for completion by default)
  tiger service start svc-12345

  # Start service without waiting for completion
  tiger service start svc-12345 --no-wait

  # Start service with custom wait timeout
  tiger service start svc-12345 --wait-timeout 10m`,
		ValidArgsFunction: serviceIDCompletion(app),
		Args:              cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}

			// Determine source service ID
			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			if err := common.CheckReadOnlyByServiceID(cmd.Context(), cfg, client, projectID, serviceID); err != nil {
				return err
			}

			// Make the start request
			resp, err := client.StartServiceWithResponse(
				context.Background(),
				api.ProjectID(projectID),
				api.ServiceID(serviceID),
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

			cmd.PrintErrf("▶️  Start request accepted for service '%s'.\n", serviceID)

			// If not waiting, return early
			if startNoWait {
				cmd.PrintErrln("💡 Use 'tiger service get' to check service status.")
				return nil
			}

			// Wait for service to become ready
			cmd.PrintErrf("⏳ Waiting for service to start (wait timeout: %v)...\n", startWaitTimeout)
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
				Timeout:    startWaitTimeout,
				TimeoutMsg: "service may still be starting",
			}); err != nil {
				// Return error for sake of exit code, but log ourselves for sake of icon
				cmd.PrintErrf("❌ Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			cmd.PrintErrf("✅ Service has been successfully started!\n")
			return nil
		},
	}

	// Add flags
	cmd.Flags().BoolVar(&startNoWait, "no-wait", false, "Don't wait for the operation to complete")
	cmd.Flags().DurationVar(&startWaitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for operation to complete")

	return cmd
}
