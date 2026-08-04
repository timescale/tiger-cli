package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// serviceCreateCmd represents the create command under service
func buildServiceCreateCmd() *cobra.Command {
	var createServiceName string
	var createAddons []string
	var createRegionCode string
	var createCpuMillis string
	var createMemoryGBs string
	var createReplicaCount int
	var createNoWait bool
	var createWaitTimeout time.Duration
	var createNoSetDefault bool
	var createWithPassword bool
	var createEnvironment string
	var output string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new database service",
		Long: `Create a new database service in the current project.

The default type of service created depends on your plan:
- Free plan: Creates a service with shared CPU/memory and the 'time-series' and 'ai' add-ons
- Paid plans: Creates a service with 0.5 CPU / 2 GB memory and the 'time-series' add-on

By default, the newly created service will be set as your default service for future
commands. Use --no-set-default to prevent this behavior.

Examples:
  # Create a TimescaleDB service with all defaults (0.5 CPU, 2GB, us-east-1, auto-generated name)
  tiger service create

  # Create a free TimescaleDB service
  tiger service create --name free-db --cpu shared

  # Create a TimescaleDB service with AI add-ons
  tiger service create --name hybrid-db --addons time-series,ai

  # Create a plain Postgres service
  tiger service create --name postgres-db --addons none

  # Create a service with more resources (waits for ready by default)
  tiger service create --name resources-db --cpu 2000 --memory 8 --replicas 2

  # Create service in a different region
  tiger service create --name eu-db --region eu-central-1

  # Create service without setting it as default
  tiger service create --name temp-db --no-set-default

  # Create service specifying only CPU (memory will be auto-configured to 8GB)
  tiger service create --name auto-memory --cpu 2000

  # Create service specifying only memory (CPU will be auto-configured to 4000m)
  tiger service create --name auto-cpu --memory 16

  # Create service without waiting for completion
  tiger service create --name quick-db --no-wait

  # Create service with custom wait timeout
  tiger service create --name patient-db --wait-timeout 1h

Allowed CPU/Memory Configurations:
  shared / shared       |  0.5 CPU (500m) / 2GB    |  1 CPU (1000m) / 4GB     |  2 CPU (2000m) / 8GB
  4 CPU (4000m) / 16GB  |  8 CPU (8000m) / 32GB    |  16 CPU (16000m) / 64GB  |  32 CPU (32000m) / 128GB

Note: You can specify both CPU and memory together, or specify only one (the other will be automatically configured).`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Auto-generate service name if not provided
			if createServiceName == "" {
				createServiceName = common.GenerateServiceName()
			}

			// Validate addons and resources
			addons, err := common.ValidateAddons(createAddons)
			if err != nil {
				return err
			}
			if createReplicaCount < 0 {
				return fmt.Errorf("replica count must be non-negative (--replicas)")
			}

			// Validate and normalize environment tag (case-insensitive)
			createEnvironment = strings.ToUpper(createEnvironment)
			if createEnvironment != "DEV" && createEnvironment != "PROD" {
				return fmt.Errorf("environment must be either 'DEV' or 'PROD', got '%s'", createEnvironment)
			}

			// Validate and normalize CPU/Memory configuration
			cpuMemoryCfg, err := common.ValidateAndNormalizeCPUMemory(createCpuMillis, createMemoryGBs)
			if err != nil {
				return err
			}

			// Validate wait timeout (Cobra handles parsing automatically)
			if createWaitTimeout <= 0 {
				return fmt.Errorf("wait timeout must be positive, got %v", createWaitTimeout)
			}

			cmd.SilenceUsage = true

			// Load config and API client
			cfg, err := common.LoadConfig(cmd.Context(), cmd.Flags())
			if err != nil {
				return err
			}

			if err := common.CheckReadOnly(cfg.Config); err != nil {
				return err
			}

			// Prepare service creation request
			environmentTag := api.EnvironmentTag(createEnvironment)
			serviceCreateReq := api.ServiceCreate{
				Name:           createServiceName,
				Addons:         util.ConvertStringSlicePtr[api.ServiceCreateAddons](addons),
				ReplicaCount:   &createReplicaCount,
				CpuMillis:      cpuMemoryCfg.CPUMillisString(),
				MemoryGbs:      cpuMemoryCfg.MemoryGBsString(),
				EnvironmentTag: &environmentTag,
			}

			if createRegionCode != "" {
				serviceCreateReq.RegionCode = &createRegionCode
			}

			// Make API call to create service
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			// All status messages go to stderr
			statusOutput := cmd.ErrOrStderr()

			if cmd.Flags().Changed("name") {
				fmt.Fprintf(statusOutput, "🚀 Creating service '%s'...\n", createServiceName)
			} else {
				fmt.Fprintf(statusOutput, "🚀 Creating service '%s' (auto-generated name)...\n", createServiceName)
			}
			resp, err := cfg.Client.CreateServiceWithResponse(ctx, cfg.ProjectID, serviceCreateReq)
			if err != nil {
				return fmt.Errorf("failed to create Service: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}
			service := *resp.JSON202
			serviceID := util.Deref(service.ServiceId)

			fmt.Fprintf(statusOutput, "✅ Service creation request accepted!\n")
			fmt.Fprintf(statusOutput, "📋 Service ID: %s\n", serviceID)

			// Save password immediately after service creation, before any waiting
			// This ensures users have access even if they interrupt the wait or it fails
			passwordSaved := handlePasswordSaving(cfg.Config, service, util.Deref(service.InitialPassword), statusOutput)

			// Set as default service unless --no-set-default is specified
			if !createNoSetDefault {
				if err := setDefaultService(cfg.Config, serviceID, statusOutput); err != nil {
					// Log warning but don't fail the command
					fmt.Fprintf(statusOutput, "⚠️  Warning: Failed to set service as default: %v\n", err)
				}
			}

			// Handle wait behavior
			var waitErr error
			if createNoWait {
				fmt.Fprintf(statusOutput, "⏳ Service is being created. Use 'tiger service list' to check status.\n")
			} else {
				// Wait for service to be ready
				fmt.Fprintf(statusOutput, "⏳ Waiting for service to be ready (wait timeout: %v)...\n", createWaitTimeout)
				if waitErr = common.WaitForService(cmd.Context(), common.WaitForServiceArgs{
					Client:    cfg.Client,
					ProjectID: cfg.ProjectID,
					ServiceID: serviceID,
					Handler: &common.StatusWaitHandler{
						TargetStatus: "READY",
						Service:      &service,
					},
					Output:     statusOutput,
					Timeout:    createWaitTimeout,
					TimeoutMsg: "service may still be provisioning",
				}); waitErr != nil {
					fmt.Fprintf(statusOutput, "❌ Error: %s\n", waitErr)
				} else {
					fmt.Fprintf(statusOutput, "🎉 Service is ready and running!\n")
					printConnectMessage(statusOutput, passwordSaved, createNoSetDefault, serviceID)
				}
			}

			if err := outputService(cmd, cfg.Config, service, cfg.Output, createWithPassword, false); err != nil {
				fmt.Fprintf(statusOutput, "⚠️  Warning: Failed to output service details: %v\n", err)
			}

			// Return error for sake of exit code, but silence it since it was already output above
			cmd.SilenceErrors = true
			return waitErr
		},
	}

	// Add flags
	cmd.Flags().StringVar(&createServiceName, "name", "", "Service name (auto-generated if not provided)")
	cmd.Flags().StringSliceVar(&createAddons, "addons", nil, fmt.Sprintf("Addons to enable (%s, or 'none' for PostgreSQL-only)", strings.Join(common.ValidAddons(), ", ")))
	cmd.Flags().StringVar(&createRegionCode, "region", "", "Region code")
	cmd.Flags().StringVar(&createCpuMillis, "cpu", "", "CPU allocation in millicores or 'shared'")
	cmd.Flags().StringVar(&createMemoryGBs, "memory", "", "Memory allocation in gigabytes or 'shared'")
	cmd.Flags().IntVar(&createReplicaCount, "replicas", 0, "Number of high-availability replicas")
	cmd.Flags().StringVar(&createEnvironment, "environment", "DEV", "Environment tag (DEV or PROD)")
	cmd.Flags().BoolVar(&createNoWait, "no-wait", false, "Don't wait for operation to complete")
	cmd.Flags().DurationVar(&createWaitTimeout, "wait-timeout", 30*time.Minute, "Wait timeout duration (e.g., 30m, 1h30m, 90s)")
	cmd.Flags().BoolVar(&createNoSetDefault, "no-set-default", false, "Don't set this service as the default service")
	cmd.Flags().BoolVar(&createWithPassword, "with-password", false, "Include password in output")
	cmd.Flags().VarP((*outputWithEnvFlag)(&output), "output", "o", "Output format (json, yaml, env, table)")

	return cmd
}
