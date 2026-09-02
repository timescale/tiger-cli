package cmd

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceForkCmd creates the fork subcommand
func buildServiceForkCmd(app *common.App) *cobra.Command {
	var forkServiceName string
	var forkNoWait bool
	var forkNoSetDefault bool
	var forkWaitTimeout time.Duration
	var forkNow bool
	var forkLastSnapshot bool
	var forkToTimestamp time.Time
	var forkCPU string
	var forkMemory string
	var forkWithPassword bool
	var forkEnvironment string

	cmd := &cobra.Command{
		Use:   "fork [service-id]",
		Short: "Fork an existing database service",
		Long: `Fork an existing database service to create a new independent copy.

You must specify exactly one timing option for the fork strategy:
- --now: Fork at the current database state (creates new snapshot or uses WAL replay)
- --last-snapshot: Fork at the last existing snapshot (faster fork)
- --to-timestamp: Fork at a specific point in time (point-in-time recovery)

By default:
- Name will be auto-generated as '{source-service-name}-fork'
- CPU and memory will be inherited from the source service
- The forked service will be set as your default service

You can override any of these defaults with the corresponding flags.`,
		Example: `  # Fork a service at the current state
  tiger service fork svc-12345 --now

  # Fork a service at the last snapshot
  tiger service fork svc-12345 --last-snapshot

  # Fork a service at a specific point in time
  tiger service fork svc-12345 --to-timestamp 2025-01-15T10:30:00Z

  # Fork with custom name
  tiger service fork svc-12345 --now --name my-forked-db

  # Fork with custom resources
  tiger service fork svc-12345 --now --cpu 2000 --memory 8

  # Fork without setting as default service
  tiger service fork svc-12345 --now --no-set-default

  # Fork without waiting for completion
  tiger service fork svc-12345 --now --no-wait

  # Fork with custom wait timeout
  tiger service fork svc-12345 --now --wait-timeout 45m`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate timing flags first - exactly one must be specified
			timingFlagsSet := 0
			if forkNow {
				timingFlagsSet++
			}
			if forkLastSnapshot {
				timingFlagsSet++
			}
			toTimestampSet := cmd.Flags().Changed("to-timestamp")
			if toTimestampSet {
				timingFlagsSet++
			}

			if timingFlagsSet == 0 {
				return fmt.Errorf("must specify --now, --last-snapshot or --to-timestamp")
			}
			if timingFlagsSet > 1 {
				return fmt.Errorf("can only specify one of --now, --last-snapshot or --to-timestamp")
			}

			// Validate and normalize environment tag (case-insensitive)
			forkEnvironment = strings.ToUpper(forkEnvironment)
			if !slices.Contains(validEnvironmentTags, forkEnvironment) {
				return fmt.Errorf("environment must be one of: %s (got '%s')", strings.Join(validEnvironmentTags, ", "), forkEnvironment)
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Gate on the fork's own tag: under prod mode, forking a PROD source
			// into a DEV fork is allowed — it reads production without changing it.
			environmentTag := api.EnvironmentTag(forkEnvironment)
			if err := common.CheckReadOnly(cfg, environmentTag); err != nil {
				return err
			}

			// Determine source service ID
			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			// Use provided custom values, validate against allowed combinations
			cpuMemoryCfg, err := common.ValidateAndNormalizeCPUMemory(forkCPU, forkMemory)
			if err != nil {
				return err
			}

			// Determine fork strategy and target time
			var forkStrategy api.ForkStrategy
			var targetTime *time.Time

			if forkNow {
				forkStrategy = api.ForkStrategyNOW
			} else if forkLastSnapshot {
				forkStrategy = api.ForkStrategyLASTSNAPSHOT
			} else if toTimestampSet {
				forkStrategy = api.ForkStrategyPITR
				targetTime = new(forkToTimestamp)
			}

			// Display what we're about to do
			strategyDesc := ""
			switch forkStrategy {
			case api.ForkStrategyNOW:
				strategyDesc = "current state"
			case api.ForkStrategyLASTSNAPSHOT:
				strategyDesc = "last snapshot"
			case api.ForkStrategyPITR:
				strategyDesc = fmt.Sprintf("point-in-time: %s", targetTime.Format(time.RFC3339))
			}
			// Prepare output message for name
			displayName := forkServiceName
			if !cmd.Flags().Changed("name") {
				displayName = "(auto-generated)"
			}
			cmd.PrintErrf("🍴 Forking service '%s' to create '%s' at %s...\n", serviceID, displayName, strategyDesc)

			// Create ForkServiceCreate request
			forkReq := api.ForkServiceCreate{
				ForkStrategy:   forkStrategy,
				TargetTime:     targetTime,
				CPUMillis:      cpuMemoryCfg.CPUMillisString(),
				MemoryGbs:      cpuMemoryCfg.MemoryGBsString(),
				EnvironmentTag: &environmentTag,
			}

			// Only set optional fields if flags were provided
			if forkServiceName != "" {
				forkReq.Name = &forkServiceName
			}

			// Make API call to fork service
			forkResp, err := client.ForkServiceWithResponse(cmd.Context(), projectID, serviceID, forkReq)
			if err != nil {
				return fmt.Errorf("failed to fork Service: %w", err)
			}

			// Handle API response
			if forkResp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(forkResp.StatusCode(), forkResp.JSON4XX)
			}

			if forkResp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}
			forkedService := *forkResp.JSON202
			forkedServiceID := forkedService.ServiceID

			cmd.PrintErrf("✅ Fork request accepted!\n")
			cmd.PrintErrf("📋 New Service ID: %s\n", forkedServiceID)

			// Save password immediately after service fork
			passwordSaved := handlePasswordSaving(cmd, cfg, forkedService, util.Deref(forkedService.InitialPassword))

			// Set as default service unless --no-set-default is used
			if !forkNoSetDefault {
				if err := setDefaultService(cmd, cfg, forkedServiceID); err != nil {
					// Log warning but don't fail the command
					cmd.PrintErrf("⚠️  Warning: Failed to set service as default: %v\n", err)
				}
			}

			// Handle wait behavior
			var waitErr error
			if forkNoWait {
				cmd.PrintErrf("⏳ Service is being forked. Use 'tiger service list' to check status.\n")
			} else {
				// Wait for service to be ready
				cmd.PrintErrf("⏳ Waiting for fork to complete (timeout: %v)...\n", forkWaitTimeout)
				if waitErr = common.WaitForService(cmd.Context(), common.WaitForServiceArgs{
					Client:    client,
					ProjectID: projectID,
					ServiceID: forkedServiceID,
					Handler: &common.StatusWaitHandler{
						TargetStatus: "READY",
						Service:      &forkedService,
					},
					Input:      cmd.InOrStdin(),
					Output:     cmd.ErrOrStderr(),
					Timeout:    forkWaitTimeout,
					TimeoutMsg: "service may still be provisioning",
				}); waitErr != nil {
					cmd.PrintErrf("❌ Error: %s\n", waitErr)
				} else {
					cmd.PrintErrf("🎉 Service fork completed successfully!\n")
					printConnectMessage(cmd, passwordSaved, forkNoSetDefault, forkedServiceID)
				}
			}

			if err := outputService(cmd, cfg, forkedService, cfg.Output, forkWithPassword, false); err != nil {
				cmd.PrintErrf("⚠️  Warning: Failed to output service details: %v\n", err)
			}

			// Return error for sake of exit code, but silence it since it was already output above
			cmd.SilenceErrors = true
			return waitErr
		},
	}

	// Add flags
	cmd.Flags().StringVar(&forkServiceName, "name", "", "Name for the forked service (defaults to '{source-name}-fork')")
	cmd.Flags().BoolVar(&forkNoWait, "no-wait", false, "Don't wait for fork operation to complete")
	cmd.Flags().BoolVar(&forkNoSetDefault, "no-set-default", false, "Don't set this service as the default service")
	cmd.Flags().DurationVar(&forkWaitTimeout, "wait-timeout", 30*time.Minute, "Wait timeout duration (e.g., 30m, 1h30m, 90s)")

	// Timing strategy flags
	cmd.Flags().BoolVar(&forkNow, "now", false, "Fork at the current database state (creates new snapshot or uses WAL replay)")
	cmd.Flags().BoolVar(&forkLastSnapshot, "last-snapshot", false, "Fork at the last existing snapshot (faster)")
	cmd.Flags().TimeVar(&forkToTimestamp, "to-timestamp", time.Time{}, []string{time.RFC3339}, "Fork at a specific point in time (RFC3339 format, e.g., 2025-01-15T10:30:00Z)")

	// Resource customization flags
	cmd.Flags().StringVar(&forkCPU, "cpu", "", "CPU allocation in millicores (inherits from source if not specified)")
	cmd.Flags().StringVar(&forkMemory, "memory", "", "Memory allocation in gigabytes (inherits from source if not specified)")
	cmd.Flags().StringVar(&forkEnvironment, "environment", "DEV", "Environment tag (DEV or PROD)")
	cmd.Flags().BoolVar(&forkWithPassword, "with-password", false, "Include password in output")
	cmd.Flags().VarP(new(outputWithEnvFlag), "output", "o", "Output format (json, yaml, env, table)")

	cmd.RegisterFlagCompletionFunc("environment", environmentCompletion)
	cmd.RegisterFlagCompletionFunc("output", outputCompletion("env"))
	cmd.RegisterFlagCompletionFunc("cpu", cpuCompletion(common.GetAllowedCPUMemoryConfigs()))
	cmd.RegisterFlagCompletionFunc("memory", memoryCompletion(common.GetAllowedCPUMemoryConfigs()))

	return cmd
}
