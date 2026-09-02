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

// serviceCreateCmd represents the create command under service
func buildServiceCreateCmd(app *common.App) *cobra.Command {
	var name string
	var addons []string
	var region string
	var cpu string
	var memory string
	var replicas int
	var noWait bool
	var waitTimeout time.Duration
	var noSetDefault bool
	var withPassword bool
	var environment string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new database service",
		Long: `Create a new database service in the current project.

The default type of service created depends on your plan:
- Free plan: Creates a service with shared CPU/memory and the 'time-series' and 'ai' add-ons
- Paid plans: Creates a service with 0.5 CPU / 2 GB memory and the 'time-series' add-on

By default, the newly created service will be set as your default service for future
commands. Use --no-set-default to prevent this behavior.

Allowed CPU/Memory Configurations:
  shared / shared       |  0.5 CPU (500m) / 2GB    |  1 CPU (1000m) / 4GB     |  2 CPU (2000m) / 8GB
  4 CPU (4000m) / 16GB  |  8 CPU (8000m) / 32GB    |  16 CPU (16000m) / 64GB  |  32 CPU (32000m) / 128GB

Note: You can specify both CPU and memory together, or specify only one (the other will be automatically configured).`,
		Example: `  # Create a TimescaleDB service with all defaults (0.5 CPU, 2GB, us-east-1, auto-generated name)
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
  tiger service create --name patient-db --wait-timeout 1h`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Auto-generate service name if not provided
			if name == "" {
				name = common.GenerateServiceName()
			}

			// Validate addons and resources
			validAddons, err := common.ValidateAddons(addons)
			if err != nil {
				return err
			}
			if replicas < 0 {
				return fmt.Errorf("replica count must be non-negative (--replicas)")
			}

			// Validate and normalize environment tag (case-insensitive)
			environment = strings.ToUpper(environment)
			if !slices.Contains(validEnvironmentTags, environment) {
				return fmt.Errorf("environment must be one of: %s (got '%s')", strings.Join(validEnvironmentTags, ", "), environment)
			}

			// Validate and normalize CPU/Memory configuration
			cpuMemoryCfg, err := common.ValidateAndNormalizeCPUMemory(cpu, memory)
			if err != nil {
				return err
			}

			// Validate wait timeout (Cobra handles parsing automatically)
			if waitTimeout <= 0 {
				return fmt.Errorf("wait timeout must be positive, got %v", waitTimeout)
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Gate on the requested tag: under prod mode, creating DEV is
			// allowed and creating PROD is not.
			environmentTag := api.EnvironmentTag(environment)
			if err := common.CheckReadOnly(cfg, environmentTag); err != nil {
				return err
			}

			// Prepare service creation request
			serviceCreateReq := api.ServiceCreate{
				Name:           name,
				Addons:         util.ConvertStringSlicePtr[api.ServiceCreateAddons](validAddons),
				ReplicaCount:   &replicas,
				CPUMillis:      cpuMemoryCfg.CPUMillisString(),
				MemoryGbs:      cpuMemoryCfg.MemoryGBsString(),
				EnvironmentTag: &environmentTag,
			}

			if region != "" {
				serviceCreateReq.RegionCode = &region
			}

			// Make API call to create service
			// All status messages go to stderr
			if cmd.Flags().Changed("name") {
				cmd.PrintErrf("🚀 Creating service '%s'...\n", name)
			} else {
				cmd.PrintErrf("🚀 Creating service '%s' (auto-generated name)...\n", name)
			}
			resp, err := client.CreateServiceWithResponse(cmd.Context(), projectID, serviceCreateReq)
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
			serviceID := service.ServiceID

			cmd.PrintErrf("✅ Service creation request accepted!\n")
			cmd.PrintErrf("📋 Service ID: %s\n", serviceID)

			// Save password immediately after service creation, before any waiting
			// This ensures users have access even if they interrupt the wait or it fails
			passwordSaved := handlePasswordSaving(cmd, cfg, service, util.Deref(service.InitialPassword))

			// Set as default service unless --no-set-default is specified
			if !noSetDefault {
				if err := setDefaultService(cmd, cfg, serviceID); err != nil {
					// Log warning but don't fail the command
					cmd.PrintErrf("⚠️  Warning: Failed to set service as default: %v\n", err)
				}
			}

			// Handle wait behavior
			var waitErr error
			if noWait {
				cmd.PrintErrf("⏳ Service is being created. Use 'tiger service list' to check status.\n")
			} else {
				// Wait for service to be ready
				cmd.PrintErrf("⏳ Waiting for service to be ready (wait timeout: %v)...\n", waitTimeout)
				if waitErr = common.WaitForService(cmd.Context(), common.WaitForServiceArgs{
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
					TimeoutMsg: "service may still be provisioning",
				}); waitErr != nil {
					cmd.PrintErrf("❌ Error: %s\n", waitErr)
				} else {
					cmd.PrintErrf("🎉 Service is ready and running!\n")
					printConnectMessage(cmd, passwordSaved, noSetDefault, serviceID)
				}
			}

			if err := outputService(cmd, cfg, service, cfg.Output, withPassword, false); err != nil {
				cmd.PrintErrf("⚠️  Warning: Failed to output service details: %v\n", err)
			}

			// Return error for sake of exit code, but silence it since it was already output above
			cmd.SilenceErrors = true
			return waitErr
		},
	}

	// Add flags
	cmd.Flags().StringVar(&name, "name", "", "Service name (auto-generated if not provided)")
	cmd.Flags().StringSliceVar(&addons, "addons", nil, fmt.Sprintf("Addons to enable (%s, or 'none' for PostgreSQL-only)", strings.Join(common.ValidAddons(), ", ")))
	cmd.Flags().StringVar(&region, "region", "", "Region code")
	cmd.Flags().StringVar(&cpu, "cpu", "", "CPU allocation in millicores or 'shared'")
	cmd.Flags().StringVar(&memory, "memory", "", "Memory allocation in gigabytes or 'shared'")
	cmd.Flags().IntVar(&replicas, "replicas", 0, "Number of high-availability replicas")
	cmd.Flags().StringVar(&environment, "environment", "DEV", "Environment tag (DEV or PROD)")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Don't wait for operation to complete")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 30*time.Minute, "Wait timeout duration (e.g., 30m, 1h30m, 90s)")
	cmd.Flags().BoolVar(&noSetDefault, "no-set-default", false, "Don't set this service as the default service")
	cmd.Flags().BoolVar(&withPassword, "with-password", false, "Include password in output")
	cmd.Flags().VarP(new(outputWithEnvFlag), "output", "o", "Output format (json, yaml, env, table)")

	cmd.RegisterFlagCompletionFunc("addons", addonsCompletion)
	cmd.RegisterFlagCompletionFunc("environment", environmentCompletion)
	cmd.RegisterFlagCompletionFunc("output", outputCompletion("env"))
	cmd.RegisterFlagCompletionFunc("cpu", cpuCompletion(common.GetAllowedCPUMemoryConfigs()))
	cmd.RegisterFlagCompletionFunc("memory", memoryCompletion(common.GetAllowedCPUMemoryConfigs()))

	return cmd
}
