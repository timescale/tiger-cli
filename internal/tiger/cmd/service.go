package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

var (
	// getAPIKeyForService can be overridden for testing
	getAPIKeyForService = config.GetAPIKey
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
	cmd.AddCommand(buildServiceDescribeCmd())
	cmd.AddCommand(buildServiceListCmd())
	cmd.AddCommand(buildServiceCreateCmd())
	cmd.AddCommand(buildServiceDeleteCmd())
	cmd.AddCommand(buildServiceUpdatePasswordCmd())

	return cmd
}

// serviceDescribeCmd represents the describe command under service
func buildServiceDescribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe [service-id]",
		Short: "Show detailed information about a service",
		Long: `Show detailed information about a specific database service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command displays comprehensive information about
the service including configuration, status, endpoints, and resource usage.

Examples:
  # Describe default service
  tiger service describe

  # Describe specific service
  tiger service describe svc-12345

  # Get service details in JSON format
  tiger service describe svc-12345 --output json

  # Get service details in YAML format
  tiger service describe svc-12345 --output yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			projectID := cfg.ProjectID
			if projectID == "" {
				return fmt.Errorf("project ID is required. Set it using login with --project-id")
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

			cmd.SilenceUsage = true

			// Get API key for authentication
			apiKey, err := getAPIKeyForService()
			if err != nil {
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w", err))
			}

			// Create API client
			client, err := api.NewTigerClient(apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			// Make API call to get service details
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
			if err != nil {
				return fmt.Errorf("failed to get service details: %w", err)
			}

			// Handle API response
			switch resp.StatusCode() {
			case 200:
				if resp.JSON200 == nil {
					return fmt.Errorf("empty response from API")
				}

				service := *resp.JSON200

				// Output service in requested format
				return outputService(cmd, service, cfg.Output)

			case 401:
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication failed: invalid API key"))
			case 403:
				return exitWithCode(ExitPermissionDenied, fmt.Errorf("permission denied: insufficient access to service"))
			case 404:
				return exitWithCode(ExitServiceNotFound, fmt.Errorf("service '%s' not found in project '%s'", serviceID, projectID))
			default:
				return fmt.Errorf("API request failed with status %d", resp.StatusCode())
			}
		},
	}

	return cmd
}

// serviceListCmd represents the list command under service
func buildServiceListCmd() *cobra.Command {
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

			projectID := cfg.ProjectID
			if projectID == "" {
				return fmt.Errorf("project ID is required. Set it using login with --project-id")
			}

			cmd.SilenceUsage = true

			// Get API key for authentication
			apiKey, err := getAPIKeyForService()
			if err != nil {
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w", err))
			}

			// Create API client
			client, err := api.NewTigerClient(apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			// Make API call to list services
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := client.GetProjectsProjectIdServicesWithResponse(ctx, projectID)
			if err != nil {
				return fmt.Errorf("failed to list services: %w", err)
			}

			// Handle API response
			switch resp.StatusCode() {
			case 200:
				// Success - process services
				if resp.JSON200 == nil {
					fmt.Fprintln(cmd.OutOrStdout(), "🏜️  No services found! Your project is looking a bit empty.")
					fmt.Fprintln(cmd.OutOrStdout(), "🚀 Ready to get started? Create your first service with: tiger service create")
					return nil
				}

				services := *resp.JSON200
				if len(services) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "🏜️  No services found! Your project is looking a bit empty.")
					fmt.Fprintln(cmd.OutOrStdout(), "🚀 Ready to get started? Create your first service with: tiger service create")
					return nil
				}

				// Output services in requested format
				return outputServices(cmd, services, cfg.Output)

			case 401:
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication failed: invalid API key"))
			case 403:
				return exitWithCode(ExitPermissionDenied, fmt.Errorf("permission denied: insufficient access to project"))
			case 404:
				return fmt.Errorf("project not found")
			default:
				return fmt.Errorf("API request failed with status %d", resp.StatusCode())
			}
		},
	}

	return cmd
}

// writeEnvFile writes PostgreSQL connection environment variables to the specified file
func writeEnvFile(client *api.ClientWithResponses, projectID, serviceID, password, envFilePath string) error {
	// Get the current service details to ensure we have endpoint information
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
	if err != nil {
		return fmt.Errorf("failed to get service details: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return fmt.Errorf("could not retrieve service details")
	}

	service := *resp.JSON200

	// Extract connection details from service
	var host, port string = "", ""

	// Use connection pooler endpoint if available, fallback to direct endpoint
	if service.ConnectionPooler != nil && service.ConnectionPooler.Endpoint != nil && service.ConnectionPooler.Endpoint.Host != nil {
		host = *service.ConnectionPooler.Endpoint.Host
		if service.ConnectionPooler.Endpoint.Port != nil {
			port = fmt.Sprintf("%d", *service.ConnectionPooler.Endpoint.Port)
		} else {
			return fmt.Errorf("service connection pooler port is not available")
		}
	} else if service.Endpoint != nil && service.Endpoint.Host != nil {
		host = *service.Endpoint.Host
		if service.Endpoint.Port != nil {
			port = fmt.Sprintf("%d", *service.Endpoint.Port)
		} else {
			return fmt.Errorf("service endpoint port is not available")
		}
	}

	// Validate that we have the required connection details
	if host == "" {
		return fmt.Errorf("service host is not available")
	}
	if port == "" {
		return fmt.Errorf("service port is not available")
	}

	// Create environment variables content
	envContent := fmt.Sprintf(`
PGHOST=%s
PGDATABASE=tsdb
PGPORT=%s
PGUSER=tsdbadmin
PGPASSWORD=%s
PGSSLMODE=require
`, host, port, password)

	// Open file for appending, create if doesn't exist
	file, err := os.OpenFile(envFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open env file '%s': %w", envFilePath, err)
	}
	defer file.Close()

	// Write environment variables
	if _, err := file.WriteString(envContent); err != nil {
		return fmt.Errorf("failed to write to env file '%s': %w", envFilePath, err)
	}

	return nil
}

// serviceCreateCmd represents the create command under service
func buildServiceCreateCmd() *cobra.Command {
	var createServiceName string
	var createServiceType string
	var createRegionCode string
	var createCpuMillis int
	var createMemoryGbs float64
	var createReplicaCount int
	var createNoWait bool
	var createWaitTimeout time.Duration
	var createNoSetDefault bool
	var createEnvFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new database service",
		Long: `Create a new database service in the current project.

By default, the newly created service will be set as your default service for future
commands. Use --no-set-default to prevent this behavior.

Examples:
  # Create a TimescaleDB service with all defaults (0.5 CPU, 2GB, us-east-1, auto-generated name)
  tiger service create

  # Create a TimescaleDB service with custom name
  tiger service create --name my-db

  # Create a PostgreSQL service with more resources (waits for ready by default)
  tiger service create --name prod-db --type postgres --cpu 2000 --memory 8 --replicas 2

  # Create service in a different region
  tiger service create --name eu-db --region google-europe-west1

  # Create service without setting it as default
  tiger service create --name temp-db --no-set-default

  # Create service specifying only CPU (memory will be auto-configured to 8GB)
  tiger service create --name auto-memory --type postgres --cpu 2000

  # Create service specifying only memory (CPU will be auto-configured to 4000m)
  tiger service create --name auto-cpu --type timescaledb --memory 16

  # Create service without waiting for completion
  tiger service create --name quick-db --type postgres --cpu 1000 --memory 4 --replicas 1 --no-wait

  # Create service with custom wait timeout
  tiger service create --name patient-db --type timescaledb --cpu 2000 --memory 8 --replicas 2 --wait-timeout 1h

  # Create service and write connection variables to .env file
  tiger service create --name my-db --env .env

  # Create service and write connection variables to custom file
  tiger service create --name my-db --env .env.dev

Allowed CPU/Memory Configurations:
  0.5 CPU (500m) / 2GB    |  1 CPU (1000m) / 4GB    |  2 CPU (2000m) / 8GB    |  4 CPU (4000m) / 16GB
  8 CPU (8000m) / 32GB    |  16 CPU (16000m) / 64GB  |  32 CPU (32000m) / 128GB

Note: You can specify both CPU and memory together, or specify only one (the other will be automatically configured).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			projectID := cfg.ProjectID
			if projectID == "" {
				return fmt.Errorf("project ID is required. Set it using login with --project-id")
			}

			// Auto-generate service name if not provided
			if createServiceName == "" {
				createServiceName = util.GenerateServiceName()
			}
			if createServiceType == "" {
				return fmt.Errorf("service type is required (--type)")
			}
			if createRegionCode == "" {
				return fmt.Errorf("region code cannot be empty (--region)")
			}
			if createReplicaCount < 0 {
				return fmt.Errorf("replica count must be non-negative (--replicas)")
			}

			// Check which flags were explicitly set
			cpuFlagSet := cmd.Flags().Changed("cpu")
			memoryFlagSet := cmd.Flags().Changed("memory")

			// Validate and normalize CPU/Memory configuration
			cpuMillis, memoryGbs, err := util.ValidateAndNormalizeCPUMemory(
				createCpuMillis, createMemoryGbs, cpuFlagSet, memoryFlagSet,
			)
			if err != nil {
				return err
			}
			createCpuMillis = cpuMillis
			createMemoryGbs = memoryGbs

			// Validate wait timeout (Cobra handles parsing automatically)
			if createWaitTimeout <= 0 {
				return fmt.Errorf("wait timeout must be positive, got %v", createWaitTimeout)
			}

			cmd.SilenceUsage = true

			// Validate service type
			if !util.IsValidServiceType(createServiceType) {
				return fmt.Errorf("invalid service type '%s'. Valid types: %s", createServiceType, strings.Join(util.ValidServiceTypes(), ", "))
			}
			serviceTypeUpper := strings.ToUpper(createServiceType)

			// Get API key for authentication
			apiKey, err := getAPIKeyForService()
			if err != nil {
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w", err))
			}

			// Create API client
			client, err := api.NewTigerClient(apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			// Prepare service creation request
			serviceCreateReq := api.ServiceCreate{
				Name:         createServiceName,
				ServiceType:  api.ServiceType(serviceTypeUpper),
				RegionCode:   createRegionCode,
				ReplicaCount: createReplicaCount,
				CpuMillis:    createCpuMillis,
				MemoryGbs:    float32(createMemoryGbs),
			}

			// Make API call to create service
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if cmd.Flags().Changed("name") {
				fmt.Fprintf(cmd.OutOrStdout(), "🚀 Creating service '%s'...\n", createServiceName)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "🚀 Creating service '%s' (auto-generated name)...\n", createServiceName)
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
					fmt.Fprintln(cmd.OutOrStdout(), "✅ Service creation request accepted!")
					return nil
				}

				service := *resp.JSON202
				serviceID := util.Deref(service.ServiceId)
				fmt.Fprintf(cmd.OutOrStdout(), "✅ Service creation request accepted!\n")
				fmt.Fprintf(cmd.OutOrStdout(), "📋 Service ID: %s\n", serviceID)

				// Capture initial password from creation response and save it immediately
				var initialPassword string
				if service.InitialPassword != nil {
					initialPassword = *service.InitialPassword
				}

				// Save password immediately after service creation, before any waiting
				// This ensures users have access even if they interrupt the wait or it fails
				handlePasswordSaving(service, initialPassword, cmd)

				// Set as default service unless --no-set-default is specified
				if !createNoSetDefault {
					if err := setDefaultService(serviceID, cmd); err != nil {
						// Log warning but don't fail the command
						fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Warning: Failed to set service as default: %v\n", err)
					}
				}

				// Handle wait behavior
				if createNoWait {
					// Check if env file is requested
					if cmd.Flags().Changed("env") {
						fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Warning: Cannot write .env file with --no-wait since service endpoints are not available until service is ready.\n")
					}
					fmt.Fprintf(cmd.OutOrStdout(), "⏳ Service is being created. Use 'tiger service list' to check status.\n")
					return nil
				}

				// Wait for service to be ready
				fmt.Fprintf(cmd.OutOrStdout(), "⏳ Waiting for service to be ready (wait timeout: %v)...\n", createWaitTimeout)
				if err := waitForServiceReady(client, projectID, serviceID, createWaitTimeout, cmd); err != nil {
					return err
				}

				// Write env file if requested, after service is ready
				if cmd.Flags().Changed("env") {
					if err := writeEnvFile(client, projectID, serviceID, initialPassword, createEnvFile); err != nil {
						fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Warning: Failed to write .env file: %v\n", err)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "📄 PostgreSQL connection variables written to %s\n", createEnvFile)
					}
				}

				return nil

			case 400:
				return fmt.Errorf("invalid request parameters")
			case 401:
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication failed: invalid API key"))
			case 403:
				return exitWithCode(ExitPermissionDenied, fmt.Errorf("permission denied: insufficient access to create services"))
			case 404:
				return fmt.Errorf("project not found")
			default:
				return fmt.Errorf("API request failed with status %d", resp.StatusCode())
			}
		},
	}

	// Add flags
	cmd.Flags().StringVar(&createServiceName, "name", "", "Service name (auto-generated if not provided)")
	cmd.Flags().StringVar(&createServiceType, "type", util.ServiceTypeTimescaleDB, fmt.Sprintf("Service type (%s)", strings.Join(util.ValidServiceTypes(), ", ")))
	cmd.Flags().StringVar(&createRegionCode, "region", "us-east-1", "Region code")
	cmd.Flags().IntVar(&createCpuMillis, "cpu", 500, "CPU allocation in millicores")
	cmd.Flags().Float64Var(&createMemoryGbs, "memory", 2.0, "Memory allocation in gigabytes")
	cmd.Flags().IntVar(&createReplicaCount, "replicas", 0, "Number of high-availability replicas")
	cmd.Flags().BoolVar(&createNoWait, "no-wait", false, "Don't wait for operation to complete")
	cmd.Flags().DurationVar(&createWaitTimeout, "wait-timeout", 30*time.Minute, "Wait timeout duration (e.g., 30m, 1h30m, 90s)")
	cmd.Flags().BoolVar(&createNoSetDefault, "no-set-default", false, "Don't set this service as the default service")
	cmd.Flags().StringVar(&createEnvFile, "env", "", "Path to .env file to write connection variables")

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

			projectID := cfg.ProjectID
			if projectID == "" {
				return fmt.Errorf("project ID is required. Set it using login with --project-id")
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

			// Get API key for authentication
			apiKey, err := getAPIKeyForService()
			if err != nil {
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w", err))
			}

			// Create API client
			client, err := api.NewTigerClient(apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
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

			// Handle API response
			switch resp.StatusCode() {
			case 200:
				fallthrough
			case 204:
				fmt.Fprintf(cmd.OutOrStdout(), "✅ Master password for 'tsdbadmin' user updated successfully\n")

				// Handle password storage using the configured method
				// Get the service details for password storage
				serviceResp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
				if err == nil && serviceResp.StatusCode() == 200 && serviceResp.JSON200 != nil {
					handlePasswordSaving(*serviceResp.JSON200, password, cmd)
				}

				return nil

			case 401:
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication failed: invalid API key"))
			case 403:
				return exitWithCode(ExitPermissionDenied, fmt.Errorf("permission denied: insufficient access to update service password"))
			case 404:
				return exitWithCode(ExitServiceNotFound, fmt.Errorf("service '%s' not found in project '%s'", serviceID, projectID))
			case 400:
				return fmt.Errorf("invalid password: %s", *resp.JSON400.Message)
			default:
				return fmt.Errorf("API request failed with status %d", resp.StatusCode())
			}
		},
	}

	// Add flags
	cmd.Flags().StringVar(&updatePasswordValue, "new-password", "", "New password for the tsdbadmin user (can also be set via TIGER_NEW_PASSWORD env var)")

	// Bind flags to viper
	viper.BindPFlag("new_password", cmd.Flags().Lookup("new-password"))

	return cmd
}

// outputService formats and outputs a single service based on the specified format
func outputService(cmd *cobra.Command, service api.Service, format string) error {
	switch strings.ToLower(format) {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(sanitizeServiceForOutput(service))
	case "yaml":
		encoder := yaml.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent(2)
		return encoder.Encode(sanitizeServiceForOutput(service))
	default: // table format (default)
		return outputServiceTable(cmd, service)
	}
}

// outputServices formats and outputs the services list based on the specified format
func outputServices(cmd *cobra.Command, services []api.Service, format string) error {
	switch strings.ToLower(format) {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(sanitizeServicesForOutput(services))
	case "yaml":
		encoder := yaml.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent(2)
		return encoder.Encode(sanitizeServicesForOutput(services))
	default: // table format (default)
		return outputServicesTable(cmd, services)
	}
}

// outputServiceTable outputs detailed service information in a formatted table
func outputServiceTable(cmd *cobra.Command, service api.Service) error {
	table := tablewriter.NewWriter(cmd.OutOrStdout())
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
			}

			if resource.Spec.MemoryGbs != nil {
				table.Append("Memory", fmt.Sprintf("%d GB", *resource.Spec.MemoryGbs))
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

	return table.Render()
}

// outputServicesTable outputs services in a formatted table using tablewriter
func outputServicesTable(cmd *cobra.Command, services []api.Service) error {
	table := tablewriter.NewWriter(cmd.OutOrStdout())
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

// sanitizeServiceForOutput creates a copy of the service with sensitive fields removed
func sanitizeServiceForOutput(service api.Service) map[string]interface{} {
	// Convert service to map and remove sensitive fields
	serviceBytes, _ := json.Marshal(service)
	var serviceMap map[string]interface{}
	json.Unmarshal(serviceBytes, &serviceMap)

	// Remove sensitive fields
	delete(serviceMap, "initial_password")
	delete(serviceMap, "initialpassword")

	return serviceMap
}

// sanitizeServicesForOutput creates copies of services with sensitive fields removed
func sanitizeServicesForOutput(services []api.Service) []map[string]interface{} {
	sanitized := make([]map[string]interface{}, len(services))
	for i, service := range services {
		sanitized[i] = sanitizeServiceForOutput(service)
	}
	return sanitized
}

// formatTimePtr formats a time pointer, returning empty string if nil
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// waitForServiceReady polls the service status until it's ready or timeout occurs
func waitForServiceReady(client *api.ClientWithResponses, projectID, serviceID string, waitTimeout time.Duration, cmd *cobra.Command) error {
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return exitWithCode(ExitTimeout, fmt.Errorf("❌ wait timeout reached after %v - service may still be provisioning", waitTimeout))
		case <-ticker.C:
			resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Error checking service status: %v\n", err)
				continue
			}

			if resp.StatusCode() != 200 || resp.JSON200 == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Service not found or error checking status\n")
				continue
			}

			service := *resp.JSON200
			status := util.DerefStr(service.Status)

			switch status {
			case "READY":
				fmt.Fprintf(cmd.OutOrStdout(), "🎉 Service is ready and running!\n")
				return nil
			case "FAILED", "ERROR":
				return fmt.Errorf("service creation failed with status: %s", status)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "⏳ Service status: %s...\n", status)
			}
		}
	}
}

// handlePasswordSaving handles saving password using the configured storage method and displaying appropriate messages
func handlePasswordSaving(service api.Service, initialPassword string, cmd *cobra.Command) {
	// Note: We don't fail the service creation if password saving fails
	// The error is handled by displaying the appropriate message below
	result, _ := util.SavePasswordWithResult(service, initialPassword)

	if result.Method == "none" && result.Message == "No password provided" {
		// Don't output anything for empty password
		return
	}

	// Output the message with appropriate emoji
	if result.Success {
		fmt.Fprintf(cmd.OutOrStdout(), "🔐 %s\n", result.Message)
	} else {
		if result.Method == "none" {
			fmt.Fprintf(cmd.OutOrStdout(), "💡 %s\n", result.Message)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "⚠️  %s\n", result.Message)
		}
	}
}

// setDefaultService sets the given service as the default service in the configuration
func setDefaultService(serviceID string, cmd *cobra.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	cfg.ServiceID = serviceID
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "🎯 Set service '%s' as default service.\n", serviceID)
	return nil
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

			// Get project ID from config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.ProjectID == "" {
				return fmt.Errorf("project ID is required. Set it using login with --project-id")
			}

			// Get API key
			apiKey, err := getAPIKeyForService()
			if err != nil {
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication required: %w", err))
			}

			// Prompt for confirmation unless --confirm is used
			if !deleteConfirm {
				fmt.Fprintf(cmd.ErrOrStderr(), "Are you sure you want to delete service '%s'? This operation cannot be undone.\n", serviceID)
				fmt.Fprintf(cmd.ErrOrStderr(), "Type the service ID '%s' to confirm: ", serviceID)
				var confirmation string
				fmt.Scanln(&confirmation)
				if confirmation != serviceID {
					fmt.Fprintln(cmd.OutOrStdout(), "❌ Delete operation cancelled.")
					return nil
				}
			}

			// Create API client
			client, err := api.NewTigerClient(apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			// Make the delete request
			resp, err := client.DeleteProjectsProjectIdServicesServiceIdWithResponse(
				context.Background(),
				api.ProjectId(cfg.ProjectID),
				api.ServiceId(serviceID),
			)
			if err != nil {
				return fmt.Errorf("failed to delete service: %w", err)
			}

			// Handle response
			switch resp.StatusCode() {
			case 202:
				fmt.Fprintf(cmd.OutOrStdout(), "🗑️  Delete request accepted for service '%s'.\n", serviceID)

				// If not waiting, return early
				if deleteNoWait {
					fmt.Fprintln(cmd.OutOrStdout(), "💡 Use 'tiger service list' to check deletion status.")
					return nil
				}

				// Wait for deletion to complete
				return waitForServiceDeletion(client, cfg.ProjectID, serviceID, deleteWaitTimeout, cmd)
			case 404:
				return exitWithCode(ExitServiceNotFound, fmt.Errorf("service '%s' not found in project '%s'", serviceID, cfg.ProjectID))
			case 401:
				return exitWithCode(ExitAuthenticationError, fmt.Errorf("authentication failed: invalid API key"))
			case 403:
				return exitWithCode(ExitPermissionDenied, fmt.Errorf("permission denied: insufficient access to delete service"))
			default:
				return fmt.Errorf("failed to delete service: API request failed with status %d", resp.StatusCode())
			}
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

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	fmt.Fprintf(cmd.OutOrStdout(), "⏳ Waiting for service '%s' to be deleted", serviceID)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(cmd.OutOrStdout(), "") // New line after dots
			return exitWithCode(ExitTimeout, fmt.Errorf("timeout waiting for service '%s' to be deleted after %v", serviceID, timeout))
		case <-ticker.C:
			// Check if service still exists
			resp, err := client.GetProjectsProjectIdServicesServiceIdWithResponse(
				ctx,
				api.ProjectId(projectID),
				api.ServiceId(serviceID),
			)
			if err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "") // New line after dots
				return fmt.Errorf("failed to check service status: %w", err)
			}

			if resp.StatusCode() == 404 {
				// Service is deleted
				fmt.Fprintln(cmd.OutOrStdout(), "") // New line after dots
				fmt.Fprintf(cmd.OutOrStdout(), "✅ Service '%s' has been successfully deleted.\n", serviceID)
				return nil
			}

			if resp.StatusCode() == 200 {
				// Service still exists, continue waiting
				fmt.Fprint(cmd.OutOrStdout(), ".")
				continue
			}

			// Other error
			fmt.Fprintln(cmd.OutOrStdout(), "") // New line after dots
			return fmt.Errorf("unexpected response while checking service status: %d", resp.StatusCode())
		}
	}
}
