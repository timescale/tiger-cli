package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceCmd creates the main service command with all subcommands.
// experimental gates preview-stage subcommands (currently `metrics`); when
// false, those subtrees are not added to the tree at all — matching ghost's
// TIGER_EXPERIMENTAL pattern. See CLAUDE.md's "Experimental Feature Gating".
func buildServiceCmd(experimental bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service",
		Aliases: []string{"services", "svc"},
		Short:   "Manage database services",
		Long:    `Manage database services within Tiger Cloud platform.`,
	}

	// Add all subcommands
	cmd.AddCommand(buildServiceGetCmd())
	cmd.AddCommand(buildServiceListCmd())
	cmd.AddCommand(buildServiceCreateCmd())
	cmd.AddCommand(buildServiceDeleteCmd())
	cmd.AddCommand(buildServiceStartCmd())
	cmd.AddCommand(buildServiceStopCmd())
	cmd.AddCommand(buildServiceUpdatePasswordCmd())
	cmd.AddCommand(buildServiceForkCmd())
	cmd.AddCommand(buildServiceResizeCmd())
	cmd.AddCommand(buildServiceLogsCmd())

	// Experimental commands, unregistered until the preview graduates.
	if experimental {
		cmd.AddCommand(buildServiceMetricsCmd())
	}

	return cmd
}

// OutputService represents a service with computed fields for output
type OutputService struct {
	api.Service
	common.ConnectionDetails
	ConnectionString string `json:"connection_string,omitempty"`
	ConsoleURL       string `json:"console_url,omitempty"`
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
		return util.SerializeToYAML(outputWriter, outputSvc)
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
		return util.SerializeToYAML(outputWriter, outputServices)
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

	// Environment tag
	if service.Metadata != nil && service.Metadata.Environment != nil {
		table.Append("Environment", *service.Metadata.Environment)
	}

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

	opts := common.ConnectionDetailsOptions{
		Role:            "tsdbadmin",
		WithPassword:    withPassword,
		InitialPassword: util.Deref(service.InitialPassword),
	}

	if connectionDetails, err := common.GetConnectionDetails(service, opts); err != nil {
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

// handlePasswordSaving handles saving password using the configured storage
// method and displaying appropriate messages. Returns true if the password was
// successfully saved, or false if not.
func handlePasswordSaving(service api.Service, initialPassword string, output io.Writer) bool {
	// Note: We don't fail the service creation if password saving fails
	// The error is handled by displaying the appropriate message below
	result, _ := common.SavePasswordWithResult(service, initialPassword, "tsdbadmin")

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

func serviceIDCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Service ID is always first positional argument
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	services, err := listServices(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	results := make([]string, 0, len(services))
	for _, service := range services {
		if service.ServiceId != nil && strings.HasPrefix(*service.ServiceId, toComplete) {
			results = append(results, cobra.CompletionWithDesc(*service.ServiceId, *service.Name))
		}
	}
	return results, cobra.ShellCompDirectiveNoFileComp
}

func listServices(cmd *cobra.Command) ([]api.Service, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(cmd.Context())
	if err != nil {
		return nil, err
	}

	// Make API call to list services
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.GetServicesWithResponse(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusOK {
		return nil, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil || len(*resp.JSON200) == 0 {
		return []api.Service{}, nil
	}

	return *resp.JSON200, nil
}

// getServiceID determines the service ID from args or config
func getServiceID(cfg *config.Config, args []string) (string, error) {
	var serviceID string
	if len(args) > 0 {
		serviceID = args[0]
	} else {
		serviceID = cfg.ServiceID
	}

	if serviceID == "" {
		return "", fmt.Errorf("service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'")
	}

	return serviceID, nil
}
