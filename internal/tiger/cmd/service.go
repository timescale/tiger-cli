package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/password"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

var (
	// getCredentialsForService can be overridden for testing
	getCredentialsForService = config.GetCredentials
	// fetchAllServicesFunc can be overridden for testing
	fetchAllServicesFunc = fetchAllServices
	// fetchServiceFunc can be overridden for testing
	fetchServiceFunc = fetchService
)

// buildServiceCmd creates the main service command with all subcommands
func buildServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service",
		Aliases: []string{"services", "svc"},
		Short:   "Manage database services",
		Long:    `Manage database services within TigerData Cloud Platform.`,
	}

	// Add all subcommands
	cmd.AddCommand(buildServiceGetCmd())
	cmd.AddCommand(buildServiceListCmd())
	cmd.AddCommand(buildServiceCreateCmd())
	cmd.AddCommand(buildServiceDeleteCmd())
	cmd.AddCommand(buildServiceUpdatePasswordCmd())
	cmd.AddCommand(buildServiceForkCmd())

	return cmd
}

// getProjectApiClient retrieves the API client and project ID, handling authentication errors
func getProjectApiClient() (*api.ClientWithResponses, string, error) {
	apiKey, projectID, err := getCredentialsForService()
	if err != nil {
		return nil, "", exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err))
	}

	// Create API client
	client, err := api.NewTigerClient(apiKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create API client: %w", err)
	}
	return client, projectID, nil
}

// fetchAllServices fetches all services for a project, handling authentication and response errors
func fetchAllServices() ([]api.Service, error) {
	client, projectID, err := getProjectApiClient()
	if err != nil {
		return nil, err
	}

	// Make API call to list services
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.GetProjectsProjectIdServicesWithResponse(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 200 {
		return nil, exitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil || len(*resp.JSON200) == 0 {
		return []api.Service{}, nil
	}

	return *resp.JSON200, nil
}

// fetchService fetches a specific service by ID, handling authentication and response errors
func fetchService(serviceID string) (*api.Service, error) {
	client, projectID, err := getProjectApiClient()
	if err != nil {
		return nil, err
	}

	// Make API call to get service details
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service details: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 200 {
		return nil, exitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from API")
	}

	return resp.JSON200, nil
}

// buildServiceGetCmd represents the get command under service
func buildServiceGetCmd() *cobra.Command {
	var withPassword bool
	var output string

	cmd := &cobra.Command{
		Use:     "get [service-id]",
		Aliases: []string{"describe", "show"},
		Short:   "Show detailed information about a service",
		Long: `Show detailed information about a specific database service.

The service ID or name can be provided as an argument or will use the default service
from your configuration. This command displays comprehensive information about
the service including configuration, status, endpoints, and resource usage.

If the provided name is ambiguous and matches multiple services, an error message will
list the matching services, and the process will exit with code ` + fmt.Sprintf("%d", ExitMultipleMatches) + `.

Examples:
  # Get default service details
  tiger service get

  # Get specific service details by ID
  tiger service get b0ysmfnr0y

  # Get specific service details by name
  tiger service get my-service

  # Get service details in JSON format
  tiger service get my-service -o json

  # Get service details in YAML format
  tiger service get my-service -o yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			idArg := ""
			if len(args) > 0 {
				idArg = args[0]
			}

			if idArg == "" && cfg.ServiceID == "" {
				return fmt.Errorf("target service was not specified. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'")
			}

			cmd.SilenceUsage = true

			// Use flag value if provided, otherwise use config value
			if cmd.Flags().Changed("output") {
				cfg.Output = output
			}

			// Determine service ID
			var service *api.Service
			if idArg == "" {
				service, err = fetchServiceFunc(cfg.ServiceID)
				if err != nil {
					return err
				}
			} else {
				services, err := fetchAllServicesFunc()
				if err != nil {
					return err
				}

				if len(services) == 0 {
					return exitWithCode(ExitGeneralError, fmt.Errorf("you have no services"))
				}

				// Filter services by exact name or id match
				var matches []api.Service
				for _, service := range services {
					if (service.ServiceId != nil && *service.ServiceId == idArg) || (service.Name != nil && *service.Name == idArg) {
						matches = append(matches, service)
					}
				}

				// Handle no matches
				if len(matches) == 0 {
					return exitWithCode(ExitServiceNotFound, fmt.Errorf("no services found matching '%s'", idArg))
				}

				if len(matches) > 1 {
					// Multiple matches - output like 'service list' as error
					if err := outputServices(cmd, matches, cfg.Output); err != nil {
						return err
					}
					return exitWithCode(ExitMultipleMatches, fmt.Errorf("multiple services found matching '%s'", idArg))
				}
				service = &matches[0]
			}

			if service == nil {
				return exitWithCode(ExitServiceNotFound, fmt.Errorf("service not found"))
			}

			// Output service in requested format
			return outputService(cmd, *service, cfg.Output, withPassword, true)
		},
	}

	cmd.Flags().BoolVar(&withPassword, "with-password", false, "Include password in output")
	cmd.Flags().VarP((*outputWithEnvFlag)(&output), "output", "o", "output format (json, yaml, env, table)")

	return cmd
}

// serviceListCmd represents the list command under service
func buildServiceListCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all services",
		Long:  `List all database services in the current project.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Use flag value if provided, otherwise use config value
			if cmd.Flags().Changed("output") {
				cfg.Output = output
			}

			cmd.SilenceUsage = true

			// Fetch all services using shared function
			services, err := fetchAllServicesFunc()
			if err != nil {
				return err
			}

			if len(services) == 0 {
				statusOutput := cmd.ErrOrStderr()
				fmt.Fprintln(statusOutput, "🏜️  No services found! Your project is looking a bit empty.")
				fmt.Fprintln(statusOutput, "🚀 Ready to get started? Create your first service with: tiger service create")
				cmd.SilenceErrors = true
				return exitWithCode(ExitGeneralError, nil)
			}

			// Output services in requested format
			return outputServices(cmd, services, cfg.Output)
		},
	}

	cmd.Flags().VarP((*outputFlag)(&output), "output", "o", "output format (json, yaml, table)")

	return cmd
}

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
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Use flag value if provided, otherwise use config value
			if cmd.Flags().Changed("output") {
				cfg.Output = output
			}

			// Auto-generate service name if not provided
			if createServiceName == "" {
				createServiceName = util.GenerateServiceName()
			}

			// Validate addons and resources
			addons, err := util.ValidateAddons(createAddons)
			if err != nil {
				return err
			}
			if createReplicaCount < 0 {
				return fmt.Errorf("replica count must be non-negative (--replicas)")
			}

			// Validate and normalize CPU/Memory configuration
			cpuMillis, memoryGBs, err := util.ValidateAndNormalizeCPUMemory(createCpuMillis, createMemoryGBs)
			if err != nil {
				return err
			}

			// Validate wait timeout (Cobra handles parsing automatically)
			if createWaitTimeout <= 0 {
				return fmt.Errorf("wait timeout must be positive, got %v", createWaitTimeout)
			}

			cmd.SilenceUsage = true

			client, projectID, err := getProjectApiClient()
			if err != nil {
				return err
			}

			// Prepare service creation request
			serviceCreateReq := api.ServiceCreate{
				Name:         createServiceName,
				Addons:       util.ConvertStringSlicePtr[api.ServiceCreateAddons](addons),
				ReplicaCount: &createReplicaCount,
				CpuMillis:    cpuMillis,
				MemoryGbs:    memoryGBs,
			}

			if createRegionCode != "" {
				serviceCreateReq.RegionCode = &createRegionCode
			}

			// Make API call to create service
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// All status messages go to stderr
			statusOutput := cmd.ErrOrStderr()

			if cmd.Flags().Changed("name") {
				fmt.Fprintf(statusOutput, "🚀 Creating service '%s'...\n", createServiceName)
			} else {
				fmt.Fprintf(statusOutput, "🚀 Creating service '%s' (auto-generated name)...\n", createServiceName)
			}
			resp, err := client.PostProjectsProjectIdServicesWithResponse(ctx, projectID, serviceCreateReq)
			if err != nil {
				return fmt.Errorf("failed to create service: %w", err)
			}

			// Handle API response
			switch resp.StatusCode() {
			case 202:
				// Success - service creation accepted
				if resp.JSON202 == nil {
					fmt.Fprintln(statusOutput, "✅ Service creation request accepted!")
					return nil
				}

				service := *resp.JSON202
				serviceID := util.Deref(service.ServiceId)
				fmt.Fprintf(statusOutput, "✅ Service creation request accepted!\n")
				fmt.Fprintf(statusOutput, "📋 Service ID: %s\n", serviceID)

				// Save password immediately after service creation, before any waiting
				// This ensures users have access even if they interrupt the wait or it fails
				passwordSaved := handlePasswordSaving(service, util.Deref(service.InitialPassword), statusOutput)

				// Set as default service unless --no-set-default is specified
				if !createNoSetDefault {
					if err := setDefaultService(cfg, serviceID, statusOutput); err != nil {
						// Log warning but don't fail the command
						fmt.Fprintf(statusOutput, "⚠️  Warning: Failed to set service as default: %v\n", err)
					}
				}

				// Handle wait behavior
				var serviceErr error
				if createNoWait {
					fmt.Fprintf(statusOutput, "⏳ Service is being created. Use 'tiger service list' to check status.\n")
				} else {
					// Wait for service to be ready
					fmt.Fprintf(statusOutput, "⏳ Waiting for service to be ready (wait timeout: %v)...\n", createWaitTimeout)
					service.Status, serviceErr = waitForServiceReady(client, projectID, serviceID, createWaitTimeout, service.Status, statusOutput)
					if serviceErr != nil {
						fmt.Fprintf(statusOutput, "❌ Error: %s\n", serviceErr)
					} else {
						fmt.Fprintf(statusOutput, "🎉 Service is ready and running!\n")
						printConnectMessage(statusOutput, passwordSaved, createNoSetDefault, serviceID)
					}
				}

				if err := outputService(cmd, service, cfg.Output, createWithPassword, false); err != nil {
					fmt.Fprintf(statusOutput, "⚠️  Warning: Failed to output service details: %v\n", err)
				}

				// Return error for sake of exit code, but silence it since it was already output above
				cmd.SilenceErrors = true
				return serviceErr
			default:
				return exitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}
		},
	}

	// Add flags
	cmd.Flags().StringVar(&createServiceName, "name", "", "Service name (auto-generated if not provided)")
	cmd.Flags().StringSliceVar(&createAddons, "addons", nil, fmt.Sprintf("Addons to enable (%s, or 'none' for PostgreSQL-only)", strings.Join(util.ValidAddons(), ", ")))
	cmd.Flags().StringVar(&createRegionCode, "region", "", "Region code")
	cmd.Flags().StringVar(&createCpuMillis, "cpu", "", "CPU allocation in millicores or 'shared'")
	cmd.Flags().StringVar(&createMemoryGBs, "memory", "", "Memory allocation in gigabytes or 'shared'")
	cmd.Flags().IntVar(&createReplicaCount, "replicas", 0, "Number of high-availability replicas")
	cmd.Flags().BoolVar(&createNoWait, "no-wait", false, "Don't wait for operation to complete")
	cmd.Flags().DurationVar(&createWaitTimeout, "wait-timeout", 30*time.Minute, "Wait timeout duration (e.g., 30m, 1h30m, 90s)")
	cmd.Flags().BoolVar(&createNoSetDefault, "no-set-default", false, "Don't set this service as the default service")
	cmd.Flags().BoolVar(&createWithPassword, "with-password", false, "Include password in output")
	cmd.Flags().VarP((*outputWithEnvFlag)(&output), "output", "o", "output format (json, yaml, env, table)")

	return cmd
}

// buildServiceUpdatePasswordCmd creates a new update-password command
func buildServiceUpdatePasswordCmd() *cobra.Command {
	var updatePasswordValue string

	cmd := &cobra.Command{
		Use:   "update-password [service-id]",
		Short: "Update the master password for a service",
		Long: `Update the master password for a specific database service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command updates the master password for the
'tsdbadmin' user used to authenticate to the database service.

Examples:
  # Update password for default service
  tiger service update-password --new-password new-secure-password

  # Update password for specific service
  tiger service update-password svc-12345 --new-password new-secure-password

  # Update password using environment variable (TIGER_NEW_PASSWORD)
  export TIGER_NEW_PASSWORD="new-secure-password"
  tiger service update-password svc-12345

  # Update password and save to .pgpass (default behavior)
  tiger service update-password svc-12345 --new-password new-secure-password

  # Update password without saving (using global flag)
  tiger service update-password svc-12345 --new-password new-secure-password --password-storage none`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Determine service ID
			var serviceID string
			if len(args) > 0 {
				serviceID = args[0]
			} else {
				serviceID = cfg.ServiceID
			}

			if serviceID == "" {
				return fmt.Errorf("service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'")
			}

			// Get password from flag or environment variable via viper
			password := viper.GetString("new_password")
			if password == "" {
				return fmt.Errorf("new password is required. Use --new-password flag or set TIGER_NEW_PASSWORD environment variable")
			}

			cmd.SilenceUsage = true

			client, projectID, err := getProjectApiClient()
			if err != nil {
				return err
			}

			// Prepare password update request
			updateReq := api.UpdatePasswordInput{
				Password: password,
			}

			// Make API call to update password
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := client.PostProjectsProjectIdServicesServiceIdUpdatePasswordWithResponse(ctx, projectID, serviceID, updateReq)
			if err != nil {
				return fmt.Errorf("failed to update service password: %w", err)
			}

			statusOutput := cmd.ErrOrStderr()

			// Handle API response
			if resp.StatusCode() != 200 && resp.StatusCode() != 204 {
				return exitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			fmt.Fprintf(statusOutput, "✅ Master password for 'tsdbadmin' user updated successfully\n")

			// Handle password storage using the configured method
			// Get the service details for password storage
			serviceResp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
			if err == nil && serviceResp.StatusCode() == 200 && serviceResp.JSON200 != nil {
				handlePasswordSaving(*serviceResp.JSON200, password, statusOutput)
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&updatePasswordValue, "new-password", "", "New password for the tsdbadmin user (can also be set via TIGER_NEW_PASSWORD env var)")

	// Bind flags to viper
	viper.BindPFlag("new_password", cmd.Flags().Lookup("new-password"))

	return cmd
}

// OutputService represents a service with computed fields for output
type OutputService struct {
	api.Service
	password.ConnectionDetails
	ConnectionString string `json:"connection_string,omitempty" yaml:"connection_string,omitempty"`
	ConsoleURL       string `json:"console_url,omitempty" yaml:"console_url,omitempty"`
}

// outputService formats and outputs a single service based on the specified format
func outputService(cmd *cobra.Command, service api.Service, format string, withPassword bool, strict bool) error {
	// Prepare the output service with computed fields
	outputSvc := prepareServiceForOutput(service, withPassword, cmd.ErrOrStderr())
	if strict && withPassword && outputSvc.Password == "" {
		return fmt.Errorf("password requested but not available for service %s", util.Deref(outputSvc.ServiceId))
	}
	outputWriter := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(outputWriter, outputSvc)
	case "yaml":
		return util.SerializeToYAML(outputWriter, outputSvc, true)
	case "env":
		return outputServiceEnv(outputSvc, outputWriter)
	default: // table format (default)
		return outputServiceTable(outputSvc, outputWriter)
	}
}

// outputServices formats and outputs the services list based on the specified format
func outputServices(cmd *cobra.Command, services []api.Service, format string) error {
	outputServices := prepareServicesForOutput(services, cmd.ErrOrStderr())
	outputWriter := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(outputWriter, outputServices)
	case "yaml":
		return util.SerializeToYAML(outputWriter, outputServices, true)
	case "env":
		return fmt.Errorf("environment variable output is not supported for multiple services")
	default: // table format (default)
		return outputServicesTable(outputServices, outputWriter)
	}
}

// outputServiceEnv outputs service details in environment variable format
func outputServiceEnv(service OutputService, output io.Writer) error {
	fmt.Fprintf(output, "PGHOST=%s\n", service.Host)
	fmt.Fprintf(output, "PGPORT=%d\n", service.Port)
	fmt.Fprintf(output, "PGDATABASE=%s\n", service.Database)
	fmt.Fprintf(output, "PGUSER=%s\n", service.Role)
	if service.Password != "" {
		fmt.Fprintf(output, "PGPASSWORD=%s\n", service.Password)
	}
	return nil
}

// outputServiceTable outputs detailed service information in a formatted table
func outputServiceTable(service OutputService, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("PROPERTY", "VALUE")

	// Basic service information
	table.Append("Service ID", util.Deref(service.ServiceId))
	table.Append("Name", util.Deref(service.Name))
	table.Append("Status", util.DerefStr(service.Status))
	table.Append("Type", util.DerefStr(service.ServiceType))
	table.Append("Region", util.Deref(service.RegionCode))

	// Resource information from Resources slice
	if service.Resources != nil && len(*service.Resources) > 0 {
		resource := (*service.Resources)[0] // Get first resource
		if resource.Spec != nil {
			if resource.Spec.CpuMillis != nil {
				cpuCores := float64(*resource.Spec.CpuMillis) / 1000
				if cpuCores == float64(int(cpuCores)) {
					table.Append("CPU", fmt.Sprintf("%.0f cores (%dm)", cpuCores, *resource.Spec.CpuMillis))
				} else {
					table.Append("CPU", fmt.Sprintf("%.1f cores (%dm)", cpuCores, *resource.Spec.CpuMillis))
				}
			} else {
				// CPU is null - this indicates a free tier service
				table.Append("CPU", "shared")
			}

			if resource.Spec.MemoryGbs != nil {
				table.Append("Memory", fmt.Sprintf("%d GB", *resource.Spec.MemoryGbs))
			} else {
				// Memory is null - this indicates a free tier service
				table.Append("Memory", "shared")
			}
		}
	}

	// High availability replicas
	if service.HaReplicas != nil {
		if service.HaReplicas.ReplicaCount != nil {
			table.Append("Replicas", fmt.Sprintf("%d", *service.HaReplicas.ReplicaCount))
		}
	}

	// Endpoint information
	if service.Endpoint != nil {
		if service.Endpoint.Host != nil {
			port := "5432"
			if service.Endpoint.Port != nil {
				port = fmt.Sprintf("%d", *service.Endpoint.Port)
			}
			table.Append("Direct Endpoint", fmt.Sprintf("%s:%s", *service.Endpoint.Host, port))
		}
	}

	// Connection pooler information
	if service.ConnectionPooler != nil && service.ConnectionPooler.Endpoint != nil {
		if service.ConnectionPooler.Endpoint.Host != nil {
			port := "6432"
			if service.ConnectionPooler.Endpoint.Port != nil {
				port = fmt.Sprintf("%d", *service.ConnectionPooler.Endpoint.Port)
			}
			table.Append("Pooler Endpoint", fmt.Sprintf("%s:%s", *service.ConnectionPooler.Endpoint.Host, port))
		}
	}

	// Pause status
	if service.Paused != nil && *service.Paused {
		table.Append("Paused", "Yes")
	}

	// Timestamps
	if service.Created != nil {
		table.Append("Created", service.Created.Format("2006-01-02 15:04:05 MST"))
	}

	// Output password if available
	if service.Password != "" {
		table.Append("Password", service.Password)
	}

	// Output connection string if available
	if service.ConnectionString != "" {
		table.Append("Connection String", service.ConnectionString)
	}
	if service.ConsoleURL != "" {
		table.Append("Console URL", service.ConsoleURL)
	}

	return table.Render()
}

// outputServicesTable outputs services in a formatted table using tablewriter
func outputServicesTable(services []OutputService, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("SERVICE ID", "NAME", "STATUS", "TYPE", "REGION", "CREATED")

	for _, service := range services {
		table.Append(
			util.Deref(service.ServiceId),
			util.Deref(service.Name),
			util.DerefStr(service.Status),
			util.DerefStr(service.ServiceType),
			util.Deref(service.RegionCode),
			formatTimePtr(service.Created),
		)
	}

	return table.Render()
}

func prepareServiceForOutput(service api.Service, withPassword bool, output io.Writer) OutputService {
	outputSvc := OutputService{
		Service: service,
	}
	outputSvc.InitialPassword = nil

	opts := password.ConnectionDetailsOptions{
		Role:            "tsdbadmin",
		WithPassword:    withPassword,
		InitialPassword: util.Deref(service.InitialPassword),
	}

	if connectionDetails, err := password.GetConnectionDetails(service, opts); err != nil {
		if output != nil {
			fmt.Fprintf(output, "⚠️  Warning: Failed to get connection details: %v\n", err)
		}
	} else {
		outputSvc.ConnectionDetails = *connectionDetails
		outputSvc.ConnectionString = connectionDetails.String()
	}

	// Build console URL
	if cfg, err := config.Load(); err == nil {
		url := fmt.Sprintf("%s/dashboard/services/%s", cfg.ConsoleURL, *service.ServiceId)
		outputSvc.ConsoleURL = url
	}

	return outputSvc
}

// prepareServicesForOutput creates copies of services with sensitive fields removed
func prepareServicesForOutput(services []api.Service, output io.Writer) []OutputService {
	prepared := make([]OutputService, len(services))
	for i, service := range services {
		prepared[i] = prepareServiceForOutput(service, false, output)
	}
	return prepared
}

// formatTimePtr formats a time pointer, returning empty string if nil
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// waitForServiceReady polls the service status until it's ready or timeout occurs
func waitForServiceReady(client *api.ClientWithResponses, projectID, serviceID string, waitTimeout time.Duration, initialStatus *api.DeployStatus, output io.Writer) (*api.DeployStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Start the spinner
	spinner := NewSpinner(output, "Service status: %s", util.DerefStr(initialStatus))
	defer spinner.Stop()

	lastStatus := initialStatus
	for {
		select {
		case <-ctx.Done():
			return lastStatus, exitWithCode(ExitTimeout, fmt.Errorf("wait timeout reached after %v - service may still be provisioning", waitTimeout))
		case <-ticker.C:
			resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
			if err != nil {
				spinner.Update("Error checking service status: %v", err)
				continue
			}

			if resp.StatusCode() != 200 || resp.JSON200 == nil {
				spinner.Update("Service not found or error checking status")
				continue
			}

			service := *resp.JSON200
			lastStatus = service.Status
			status := util.DerefStr(service.Status)

			switch status {
			case "READY":
				return service.Status, nil
			case "FAILED", "ERROR":
				return service.Status, fmt.Errorf("service creation failed with status: %s", status)
			default:
				spinner.Update("Service status: %s", status)
			}
		}
	}
}

// handlePasswordSaving handles saving password using the configured storage
// method and displaying appropriate messages. Returns true if the password was
// successfully saved, or false if not.
func handlePasswordSaving(service api.Service, initialPassword string, output io.Writer) bool {
	// Note: We don't fail the service creation if password saving fails
	// The error is handled by displaying the appropriate message below
	result, _ := password.SavePasswordWithResult(service, initialPassword, "tsdbadmin")

	if result.Method == "none" && result.Message == "No password provided" {
		// Don't output anything for empty password
		return false
	}

	// Output the message with appropriate emoji
	if result.Success {
		fmt.Fprintf(output, "🔐 %s\n", result.Message)
		return true
	} else if result.Method == "none" {
		fmt.Fprintf(output, "💡 %s\n", result.Message)
	} else {
		fmt.Fprintf(output, "⚠️  %s\n", result.Message)
	}
	return false
}

// setDefaultService sets the given service as the default service in the configuration
func setDefaultService(cfg *config.Config, serviceID string, output io.Writer) error {
	if err := cfg.Set("service_id", serviceID); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(output, "🎯 Set service '%s' as default service.\n", serviceID)
	return nil
}

func printConnectMessage(output io.Writer, passwordSaved, noSetDefault bool, serviceID string) {
	if !passwordSaved {
		// We can't connect if no password was saved, so don't show message
		return
	} else if noSetDefault {
		// If the service wasn't set as the default, include the serviceID in the command
		fmt.Fprintf(output, "🔌 Run 'tiger db connect %s' to connect to your new service\n", serviceID)
	} else {
		// If the service was set as the default, no need to include the serviceID in the command
		fmt.Fprintf(output, "🔌 Run 'tiger db connect' to connect to your new service\n")
	}
}

// buildServiceDeleteCmd creates the delete subcommand
func buildServiceDeleteCmd() *cobra.Command {
	var deleteNoWait bool
	var deleteWaitTimeout time.Duration
	var deleteConfirm bool

	cmd := &cobra.Command{
		Use:   "delete [service-id]",
		Short: "Delete a database service",
		Long: `Delete a database service permanently.

This operation is irreversible. By default, you will be prompted to type the service ID
to confirm deletion, unless you use the --confirm flag.

Note for AI agents: Always confirm with the user before performing this destructive operation.

Examples:
  # Delete a service (with confirmation prompt)
  tiger service delete svc-12345

  # Delete service without confirmation prompt
  tiger service delete svc-12345 --confirm

  # Delete service without waiting for completion
  tiger service delete svc-12345 --no-wait

  # Delete service with custom wait timeout
  tiger service delete svc-12345 --wait-timeout 15m`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Require explicit service ID for safety
			if len(args) < 1 {
				return fmt.Errorf("service ID is required")
			}
			serviceID := args[0]

			cmd.SilenceUsage = true

			client, projectID, err := getProjectApiClient()
			if err != nil {
				return err
			}

			statusOutput := cmd.ErrOrStderr()

			// Prompt for confirmation unless --confirm is used
			if !deleteConfirm {
				fmt.Fprintf(statusOutput, "Are you sure you want to delete service '%s'? This operation cannot be undone.\n", serviceID)
				fmt.Fprintf(statusOutput, "Type the service ID '%s' to confirm: ", serviceID)
				var confirmation string
				fmt.Scanln(&confirmation)
				if confirmation != serviceID {
					fmt.Fprintln(statusOutput, "❌ Delete operation cancelled.")
					return nil
				}
			}

			// Make the delete request
			resp, err := client.DeleteProjectsProjectIdServicesServiceIdWithResponse(
				context.Background(),
				api.ProjectId(projectID),
				api.ServiceId(serviceID),
			)
			if err != nil {
				return fmt.Errorf("failed to delete service: %w", err)
			}

			// Handle response
			if resp.StatusCode() != 202 {
				return exitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			fmt.Fprintf(statusOutput, "🗑️  Delete request accepted for service '%s'.\n", serviceID)

			// If not waiting, return early
			if deleteNoWait {
				fmt.Fprintln(statusOutput, "💡 Use 'tiger service list' to check deletion status.")
				return nil
			}

			// Wait for deletion to complete
			if err := waitForServiceDeletion(client, projectID, serviceID, deleteWaitTimeout, cmd); err != nil {
				// Return error for sake of exit code, but log ourselves for sake of icon
				fmt.Fprintf(statusOutput, "❌ Error: %s\n", err)
				cmd.SilenceErrors = true
				return err
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&deleteNoWait, "no-wait", false, "Don't wait for deletion to complete, return immediately")
	cmd.Flags().DurationVar(&deleteWaitTimeout, "wait-timeout", 30*time.Minute, "Wait timeout duration (e.g., 30m, 1h30m, 90s)")
	cmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Skip confirmation prompt (AI agents must confirm with user first)")

	return cmd
}

// waitForServiceDeletion waits for a service to be fully deleted
func waitForServiceDeletion(client *api.ClientWithResponses, projectID string, serviceID string, timeout time.Duration, cmd *cobra.Command) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	statusOutput := cmd.ErrOrStderr()

	// Start the spinner
	spinner := NewSpinner(statusOutput, "Waiting for service '%s' to be deleted", serviceID)
	defer spinner.Stop()

	for {
		select {
		case <-ctx.Done():
			return exitWithCode(ExitTimeout, fmt.Errorf("timeout waiting for service '%s' to be deleted after %v", serviceID, timeout))
		case <-ticker.C:
			// Check if service still exists
			resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(
				ctx,
				api.ProjectId(projectID),
				api.ServiceId(serviceID),
			)
			if err != nil {
				return fmt.Errorf("failed to check service status: %w", err)
			}

			if resp.StatusCode() == 404 {
				// Service is deleted
				spinner.Stop()
				fmt.Fprintf(statusOutput, "✅ Service '%s' has been successfully deleted.\n", serviceID)
				return nil
			}

			if resp.StatusCode() == 200 {
				// Service still exists, continue waiting
				continue
			}

			// Other error
			return fmt.Errorf("unexpected response while checking service status: %d", resp.StatusCode())
		}
	}
}

// buildServiceForkCmd creates the fork subcommand
func buildServiceForkCmd() *cobra.Command {
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
	var output string

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

You can override any of these defaults with the corresponding flags.

Examples:
  # Fork a service at the current state
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
		Args: cobra.MaximumNArgs(1),
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

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Use flag value if provided, otherwise use config value
			if cmd.Flags().Changed("output") {
				cfg.Output = output
			}

			// Determine source service ID
			var serviceID string
			if len(args) > 0 {
				serviceID = args[0]
			} else {
				serviceID = cfg.ServiceID
			}

			if serviceID == "" {
				return fmt.Errorf("service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'")
			}

			cmd.SilenceUsage = true

			client, projectID, err := getProjectApiClient()
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Use provided custom values, validate against allowed combinations
			cpuMillis, memoryGBs, err := util.ValidateAndNormalizeCPUMemory(forkCPU, forkMemory)
			if err != nil {
				return err
			}

			// Determine fork strategy and target time
			var forkStrategy api.ForkStrategy
			var targetTime *time.Time

			if forkNow {
				forkStrategy = api.NOW
			} else if forkLastSnapshot {
				forkStrategy = api.LASTSNAPSHOT
			} else if toTimestampSet {
				forkStrategy = api.PITR
				parsedTime := forkToTimestamp
				targetTime = &parsedTime
			}

			// Display what we're about to do
			strategyDesc := ""
			switch forkStrategy {
			case api.NOW:
				strategyDesc = "current state"
			case api.LASTSNAPSHOT:
				strategyDesc = "last snapshot"
			case api.PITR:
				strategyDesc = fmt.Sprintf("point-in-time: %s", targetTime.Format(time.RFC3339))
			}
			// Prepare output message for name
			displayName := forkServiceName
			if !cmd.Flags().Changed("name") {
				displayName = "(auto-generated)"
			}
			statusOutput := cmd.ErrOrStderr()
			fmt.Fprintf(statusOutput, "🍴 Forking service '%s' to create '%s' at %s...\n", serviceID, displayName, strategyDesc)

			// Create ForkServiceCreate request
			forkReq := api.ForkServiceCreate{
				ForkStrategy: forkStrategy,
				TargetTime:   targetTime,
				CpuMillis:    cpuMillis,
				MemoryGbs:    memoryGBs,
			}

			// Only set optional fields if flags were provided
			if forkServiceName != "" {
				forkReq.Name = &forkServiceName
			}

			// Make API call to fork service
			forkResp, err := client.PostProjectsProjectIdServicesServiceIdForkServiceWithResponse(ctx, projectID, serviceID, forkReq)
			if err != nil {
				return fmt.Errorf("failed to fork service: %w", err)
			}

			// Handle API response
			if forkResp.StatusCode() != 202 {
				return exitWithErrorFromStatusCode(forkResp.StatusCode(), forkResp.JSON4XX)
			}

			// Success - service fork accepted
			forkedService := *forkResp.JSON202
			forkedServiceID := util.DerefStr(forkedService.ServiceId)
			fmt.Fprintf(statusOutput, "✅ Fork request accepted!\n")
			fmt.Fprintf(statusOutput, "📋 New Service ID: %s\n", forkedServiceID)

			// Save password immediately after service fork
			passwordSaved := handlePasswordSaving(forkedService, util.Deref(forkedService.InitialPassword), statusOutput)

			// Set as default service unless --no-set-default is used
			if !forkNoSetDefault {
				if err := setDefaultService(cfg, forkedServiceID, statusOutput); err != nil {
					// Log warning but don't fail the command
					fmt.Fprintf(statusOutput, "⚠️  Warning: Failed to set service as default: %v\n", err)
				}
			}

			// Handle wait behavior
			var serviceErr error
			if forkNoWait {
				fmt.Fprintf(statusOutput, "⏳ Service is being forked. Use 'tiger service list' to check status.\n")
			} else {
				// Wait for service to be ready
				fmt.Fprintf(statusOutput, "⏳ Waiting for fork to complete (timeout: %v)...\n", forkWaitTimeout)
				forkedService.Status, serviceErr = waitForServiceReady(client, projectID, forkedServiceID, forkWaitTimeout, forkedService.Status, statusOutput)
				if serviceErr != nil {
					fmt.Fprintf(statusOutput, "❌ Error: %s\n", serviceErr)
				} else {
					fmt.Fprintf(statusOutput, "🎉 Service fork completed successfully!\n")
					printConnectMessage(statusOutput, passwordSaved, forkNoSetDefault, forkedServiceID)
				}
			}

			if err := outputService(cmd, forkedService, cfg.Output, forkWithPassword, false); err != nil {
				fmt.Fprintf(statusOutput, "⚠️  Warning: Failed to output service details: %v\n", err)
			}

			// Return error for sake of exit code, but silence it since it was already output above
			cmd.SilenceErrors = true
			return serviceErr
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
	cmd.Flags().BoolVar(&forkWithPassword, "with-password", false, "Include password in output")
	cmd.Flags().VarP((*outputWithEnvFlag)(&output), "output", "o", "output format (json, yaml, env, table)")

	return cmd
}
