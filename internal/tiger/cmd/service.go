package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/tigerdata/tiger-cli/internal/tiger/api"
	"github.com/tigerdata/tiger-cli/internal/tiger/config"
)

var (
	// getAPIKeyForService can be overridden for testing
	getAPIKeyForService = getAPIKey
)

// serviceCmd represents the service command
var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage database services",
	Long:  `Manage database services within TigerData Cloud Platform.`,
}

// serviceListCmd represents the list command under service
var serviceListCmd = &cobra.Command{
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
			return fmt.Errorf("authentication required: %w", err)
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

		case 401, 403:
			return fmt.Errorf("authentication failed: invalid API key")
		case 404:
			return fmt.Errorf("project not found")
		default:
			return fmt.Errorf("API request failed with status %d", resp.StatusCode())
		}
	},
}

// serviceCreateCmd represents the create command under service
var serviceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new database service",
	Long: `Create a new database service in the current project.

Examples:
  # Create a TimescaleDB service with all defaults (0.5 CPU, 2GB, us-east-1, auto-generated name)
  tiger service create
  
  # Create a TimescaleDB service with custom name
  tiger service create --name my-db

  # Create a PostgreSQL service with more resources (waits for ready by default)
  tiger service create --name prod-db --type postgres --cpu 2000 --memory 8 --replicas 2

  # Create service in a different region
  tiger service create --name eu-db --region google-europe-west1

  # Create service specifying only CPU (memory will be auto-configured to 8GB)
  tiger service create --name auto-memory --type postgres --cpu 2000

  # Create service specifying only memory (CPU will be auto-configured to 4000m)
  tiger service create --name auto-cpu --type timescaledb --memory 16

  # Create service without waiting for completion
  tiger service create --name quick-db --type postgres --cpu 1000 --memory 4 --replicas 1 --no-wait

  # Create service with custom timeout
  tiger service create --name patient-db --type timescaledb --cpu 2000 --memory 8 --replicas 2 --timeout 60

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
			createServiceName = fmt.Sprintf("db-%d", rand.Intn(10000))
		}
		if createServiceType == "" {
			return fmt.Errorf("service type is required (--type)")
		}
		if createRegionCode == "" {
			return fmt.Errorf("region code cannot be empty (--region)")
		}
		if createReplicaCount <= 0 {
			return fmt.Errorf("replica count must be positive (--replicas)")
		}

		// Check which flags were explicitly set
		cpuFlagSet := cmd.Flags().Changed("cpu")
		memoryFlagSet := cmd.Flags().Changed("memory")

		// Validate and normalize CPU/Memory configuration
		cpuMillis, memoryGbs, err := validateAndNormalizeCPUMemory(createCpuMillis, createMemoryGbs, cpuFlagSet, memoryFlagSet)
		if err != nil {
			return err
		}
		createCpuMillis = cpuMillis
		createMemoryGbs = memoryGbs

		cmd.SilenceUsage = true

		// Validate service type
		validTypes := []string{"timescaledb", "postgres", "vector"}
		serviceTypeUpper := strings.ToUpper(createServiceType)
		isValidType := false
		for _, validType := range validTypes {
			if serviceTypeUpper == strings.ToUpper(validType) {
				isValidType = true
				break
			}
		}
		if !isValidType {
			return fmt.Errorf("invalid service type '%s'. Valid types: %s", createServiceType, strings.Join(validTypes, ", "))
		}

		// Get API key for authentication
		apiKey, err := getAPIKeyForService()
		if err != nil {
			return fmt.Errorf("authentication required: %w", err)
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
			serviceID := derefString(service.ServiceId)
			fmt.Fprintf(cmd.OutOrStdout(), "✅ Service creation request accepted!\n")
			fmt.Fprintf(cmd.OutOrStdout(), "📋 Service ID: %s\n", serviceID)

			// Handle wait behavior
			if createNoWait {
				fmt.Fprintf(cmd.OutOrStdout(), "⏳ Service is being created. Use 'tiger service list' to check status.\n")
				return nil
			}

			// Wait for service to be ready
			fmt.Fprintf(cmd.OutOrStdout(), "⏳ Waiting for service to be ready (timeout: %d minutes)...\n", createTimeoutMinutes)
			return waitForServiceReady(client, projectID, serviceID, createTimeoutMinutes, cmd)

		case 400:
			return fmt.Errorf("invalid request parameters")
		case 401, 403:
			return fmt.Errorf("authentication failed: invalid API key")
		case 404:
			return fmt.Errorf("project not found")
		default:
			return fmt.Errorf("API request failed with status %d", resp.StatusCode())
		}
	},
}

// Command-line flags for service create
var (
	createServiceName    string
	createServiceType    string
	createRegionCode     string
	createCpuMillis      int
	createMemoryGbs      float64
	createReplicaCount   int
	createNoWait         bool
	createTimeoutMinutes int
)

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceListCmd)
	serviceCmd.AddCommand(serviceCreateCmd)

	// Add flags for service create command
	serviceCreateCmd.Flags().StringVar(&createServiceName, "name", "", "Service name (auto-generated if not provided)")
	serviceCreateCmd.Flags().StringVar(&createServiceType, "type", "timescaledb", "Service type (timescaledb, postgres, vector)")
	serviceCreateCmd.Flags().StringVar(&createRegionCode, "region", "us-east-1", "Region code")
	serviceCreateCmd.Flags().IntVar(&createCpuMillis, "cpu", 500, "CPU allocation in millicores")
	serviceCreateCmd.Flags().Float64Var(&createMemoryGbs, "memory", 2.0, "Memory allocation in gigabytes")
	serviceCreateCmd.Flags().IntVar(&createReplicaCount, "replicas", 1, "Number of high-availability replicas")
	serviceCreateCmd.Flags().BoolVar(&createNoWait, "no-wait", false, "Don't wait for operation to complete")
	serviceCreateCmd.Flags().IntVar(&createTimeoutMinutes, "timeout", 30, "Timeout for waiting in minutes")
}

// outputServices formats and outputs the services list based on the specified format
func outputServices(cmd *cobra.Command, services []api.Service, format string) error {
	switch strings.ToLower(format) {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(services)
	case "yaml":
		encoder := yaml.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent(2)
		return encoder.Encode(services)
	default: // table format (default)
		return outputServicesTable(cmd, services)
	}
}

// outputServicesTable outputs services in a formatted table using tablewriter
func outputServicesTable(cmd *cobra.Command, services []api.Service) error {
	table := tablewriter.NewWriter(cmd.OutOrStdout())
	table.Header("SERVICE ID", "NAME", "STATUS", "TYPE", "REGION", "CREATED")

	for _, service := range services {
		table.Append(
			derefString(service.ServiceId),
			derefString(service.Name),
			formatDeployStatus(service.Status),
			formatServiceType(service.ServiceType),
			derefString(service.RegionCode),
			formatTimePtr(service.Created),
		)
	}

	return table.Render()
}

// derefString safely dereferences a string pointer, returning empty string if nil
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// formatTimePtr formats a time pointer, returning empty string if nil
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// formatDeployStatus formats a DeployStatus pointer, returning empty string if nil
func formatDeployStatus(status *api.DeployStatus) string {
	if status == nil {
		return ""
	}
	return string(*status)
}

// formatServiceType formats a ServiceType pointer, returning empty string if nil
func formatServiceType(serviceType *api.ServiceType) string {
	if serviceType == nil {
		return ""
	}
	return string(*serviceType)
}

// waitForServiceReady polls the service status until it's ready or timeout occurs
func waitForServiceReady(client *api.ClientWithResponses, projectID, serviceID string, timeoutMinutes int, cmd *cobra.Command) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMinutes)*time.Minute)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("❌ timeout waiting for service to be ready after %d minutes", timeoutMinutes)
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
			status := formatDeployStatus(service.Status)

			switch status {
			case "READY":
				fmt.Fprintf(cmd.OutOrStdout(), "🎉 Service is ready and running!\n")

				// Save password to ~/.pgpass if available
				if service.InitialPassword != nil && service.Endpoint != nil {
					err := savePgPassEntry(service, *service.InitialPassword)
					if err != nil {
						fmt.Fprintf(cmd.OutOrStdout(), "⚠️  Failed to save password to ~/.pgpass: %v\n", err)
						fmt.Fprintf(cmd.OutOrStdout(), "🔐 Initial password: %s\n", *service.InitialPassword)
						fmt.Fprintf(cmd.OutOrStdout(), "💡 Save this password - it won't be shown again!\n")
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "🔐 Password saved to ~/.pgpass for automatic authentication\n")
					}
				}

				return nil
			case "FAILED", "ERROR":
				return fmt.Errorf("service creation failed with status: %s", status)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "⏳ Service status: %s...\n", status)
			}
		}
	}
}

// savePgPassEntry saves the service credentials to ~/.pgpass file
func savePgPassEntry(service api.Service, password string) error {
	if service.Endpoint == nil || service.Endpoint.Host == nil {
		return fmt.Errorf("service endpoint not available")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	pgpassPath := filepath.Join(homeDir, ".pgpass")

	// Create entry: hostname:port:database:username:password
	host := *service.Endpoint.Host
	port := "5432" // default PostgreSQL port
	if service.Endpoint.Port != nil {
		port = fmt.Sprintf("%d", *service.Endpoint.Port)
	}
	database := "tsdb"      // TimescaleDB database name
	username := "tsdbadmin" // default admin user

	entry := fmt.Sprintf("%s:%s:%s:%s:%s\n", host, port, database, username, password)

	// Check if entry already exists
	if exists, err := pgpassEntryExists(pgpassPath, host, port, username); err == nil && exists {
		// Entry already exists, don't add duplicate
		return nil
	}

	// Append to .pgpass file with restricted permissions
	file, err := os.OpenFile(pgpassPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open .pgpass file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to .pgpass file: %w", err)
	}

	return nil
}

// pgpassEntryExists checks if a .pgpass entry already exists for the given host/port/username
func pgpassEntryExists(pgpassPath, host, port, username string) (bool, error) {
	file, err := os.Open(pgpassPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	targetPrefix := fmt.Sprintf("%s:%s:", host, port)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, targetPrefix) && strings.Contains(line, username) {
			return true, nil
		}
	}

	return false, scanner.Err()
}

// CPUMemoryConfig represents an allowed CPU/Memory configuration
type CPUMemoryConfig struct {
	CPUMillis int     // CPU in millicores
	MemoryGbs float64 // Memory in GB
}

// getAllowedCPUMemoryConfigs returns the allowed CPU/Memory configurations from the spec
func getAllowedCPUMemoryConfigs() []CPUMemoryConfig {
	return []CPUMemoryConfig{
		{CPUMillis: 500, MemoryGbs: 2},     // 0.5 CPU, 2GB
		{CPUMillis: 1000, MemoryGbs: 4},    // 1 CPU, 4GB
		{CPUMillis: 2000, MemoryGbs: 8},    // 2 CPU, 8GB
		{CPUMillis: 4000, MemoryGbs: 16},   // 4 CPU, 16GB
		{CPUMillis: 8000, MemoryGbs: 32},   // 8 CPU, 32GB
		{CPUMillis: 16000, MemoryGbs: 64},  // 16 CPU, 64GB
		{CPUMillis: 32000, MemoryGbs: 128}, // 32 CPU, 128GB
	}
}

// validateAndNormalizeCPUMemory validates CPU/Memory values and applies auto-configuration logic
func validateAndNormalizeCPUMemory(cpuMillis int, memoryGbs float64, cpuFlagSet, memoryFlagSet bool) (int, float64, error) {
	configs := getAllowedCPUMemoryConfigs()

	// If both CPU and memory flags were explicitly set, validate they match an allowed configuration
	if cpuFlagSet && memoryFlagSet {
		for _, config := range configs {
			if config.CPUMillis == cpuMillis && config.MemoryGbs == memoryGbs {
				return cpuMillis, memoryGbs, nil
			}
		}
		// If no exact match, provide helpful error
		return 0, 0, fmt.Errorf("invalid CPU/Memory combination: %dm CPU and %.0fGB memory. Allowed combinations: %s",
			cpuMillis, memoryGbs, formatAllowedCombinations(configs))
	}

	// If only CPU flag was explicitly set, find matching memory and auto-configure
	if cpuFlagSet && !memoryFlagSet {
		for _, config := range configs {
			if config.CPUMillis == cpuMillis {
				return cpuMillis, config.MemoryGbs, nil
			}
		}
		return 0, 0, fmt.Errorf("invalid CPU allocation: %dm. Allowed CPU values: %s",
			cpuMillis, formatAllowedCPUValues(configs))
	}

	// If only memory flag was explicitly set, find matching CPU and auto-configure
	if !cpuFlagSet && memoryFlagSet {
		for _, config := range configs {
			if config.MemoryGbs == memoryGbs {
				return config.CPUMillis, memoryGbs, nil
			}
		}
		return 0, 0, fmt.Errorf("invalid memory allocation: %.0fGB. Allowed memory values: %s",
			memoryGbs, formatAllowedMemoryValues(configs))
	}

	// If neither flag was explicitly set, use default values (0.5 CPU, 2GB)
	return 500, 2, nil
}

// formatAllowedCombinations returns a user-friendly string of allowed CPU/Memory combinations
func formatAllowedCombinations(configs []CPUMemoryConfig) string {
	var combinations []string
	for _, config := range configs {
		cpuCores := float64(config.CPUMillis) / 1000
		if cpuCores == float64(int(cpuCores)) {
			combinations = append(combinations, fmt.Sprintf("%.0f CPU/%.0fGB", cpuCores, config.MemoryGbs))
		} else {
			combinations = append(combinations, fmt.Sprintf("%.1f CPU/%.0fGB", cpuCores, config.MemoryGbs))
		}
	}
	return strings.Join(combinations, ", ")
}

// formatAllowedCPUValues returns a user-friendly string of allowed CPU values
func formatAllowedCPUValues(configs []CPUMemoryConfig) string {
	var cpuValues []string
	for _, config := range configs {
		cpuCores := float64(config.CPUMillis) / 1000
		if cpuCores == float64(int(cpuCores)) {
			cpuValues = append(cpuValues, fmt.Sprintf("%.0f (%.0fm)", cpuCores, float64(config.CPUMillis)))
		} else {
			cpuValues = append(cpuValues, fmt.Sprintf("%.1f (%.0fm)", cpuCores, float64(config.CPUMillis)))
		}
	}
	return strings.Join(cpuValues, ", ")
}

// formatAllowedMemoryValues returns a user-friendly string of allowed memory values
func formatAllowedMemoryValues(configs []CPUMemoryConfig) string {
	var memoryValues []string
	for _, config := range configs {
		memoryValues = append(memoryValues, fmt.Sprintf("%.0fGB", config.MemoryGbs))
	}
	return strings.Join(memoryValues, ", ")
}
