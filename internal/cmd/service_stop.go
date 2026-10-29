package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceStopCmd creates the stop subcommand
func buildServiceStopCmd(app *common.App) *cobra.Command {
	var noWait bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:     "stop [name-or-id]",
		Aliases: []string{"pause"},
		Short:   "Stop a running database service",
		Long: `Stop a running database service.

This operation stops a service that is currently active/running. The service will transition to an inactive state and will no longer accept connections.

The service can be given by ID or name as an argument, or will use the default
service from your configuration.`,
		Example: `  # Stop a service (waits for completion by default)
  tiger service stop svc-12345

  # Stop service without waiting for completion
  tiger service stop svc-12345 --no-wait

  # Stop service with custom wait timeout
  tiger service stop svc-12345 --wait-timeout 10m`,
		ValidArgsFunction: serviceRefCompletion(app),
		Args:              cobra.MaximumNArgs(1),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateWaitTimeout(waitTimeout); err != nil {
				return err
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Determine the service ref
			serviceRef, err := getServiceRef(cmd, cfg, args)
			if err != nil {
				return err
			}

			resolved, err := resolveServiceForWrite(cmd.Context(), cfg, client, projectID, serviceRef)
			if err != nil {
				return err
			}

			// Make the stop request
			service, err := common.JSON202(client.StopServiceWithResponse(
				cmd.Context(),
				api.ProjectID(projectID),
				api.ServiceID(resolved.ServiceID),
			))
			if err != nil {
				return err
			}

			cmd.PrintErrf("Stop request accepted for service %s.\n", serviceLabel(*resolved))

			// If not waiting, return early
			if noWait {
				cmd.PrintErrf("Use 'tiger service get %s' to check service status.\n", resolved.ServiceID)
				return nil
			}

			// Wait for service to become paused
			cmd.PrintErrf("Waiting for service to stop (timeout: %v)...\n", waitTimeout)
			if err := common.WaitForService(cmd.Context(), common.WaitForServiceArgs{
				Client:       client,
				ProjectID:    projectID,
				ServiceID:    resolved.ServiceID,
				Service:      &service,
				TargetStatus: api.DeployStatusPAUSED,
				Input:        cmd.InOrStdin(),
				Output:       cmd.ErrOrStderr(),
				Timeout:      waitTimeout,
				TimeoutMsg:   "service may still be stopping",
			}); err != nil {
				// Return error for sake of exit code, but log ourselves for sake of icon
				cmd.PrintErrf("Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}

			cmd.PrintErrf("Service stopped.\n")
			return nil
		},
	}

	// Add flags
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for the operation to complete")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for the operation to complete (e.g., 30m, 1h30m, 90s)")

	return cmd
}
