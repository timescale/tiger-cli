package cmd

import (
	"fmt"
	"io"
	"strings"

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
func buildServiceCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service",
		Aliases: []string{"services", "svc"},
		Short:   "Manage database services",
		Long:    `Manage database services within Tiger Cloud platform.`,
	}

	// Add all subcommands
	cmd.AddCommand(buildServiceGetCmd(app))
	cmd.AddCommand(buildServiceListCmd(app))
	cmd.AddCommand(buildServiceCreateCmd(app))
	cmd.AddCommand(buildServiceDeleteCmd(app))
	cmd.AddCommand(buildServiceStartCmd(app))
	cmd.AddCommand(buildServiceStopCmd(app))
	cmd.AddCommand(buildServiceUpdatePasswordCmd(app))
	cmd.AddCommand(buildServiceForkCmd(app))
	cmd.AddCommand(buildServiceResizeCmd(app))
	cmd.AddCommand(buildServiceLogsCmd(app))

	// Experimental commands, unregistered until the preview graduates.
	if app.Experimental {
		cmd.AddCommand(buildServiceMetricsCmd(app))
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
func outputService(cmd *cobra.Command, cfg *config.Config, service api.Service, format string, withPassword bool, strict bool) error {
	// Prepare the output service with computed fields
	outputSvc := prepareServiceForOutput(cmd, cfg, service, withPassword)
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
		return outputServiceEnv(cmd, outputSvc)
	default: // table format (default)
		return outputServiceTable(outputSvc, outputWriter)
	}
}

// outputServiceEnv outputs service details in environment variable format
func outputServiceEnv(cmd *cobra.Command, service OutputService) error {
	cmd.Printf("PGHOST=%s\n", service.Host)
	cmd.Printf("PGPORT=%d\n", service.Port)
	cmd.Printf("PGDATABASE=%s\n", service.Database)
	cmd.Printf("PGUSER=%s\n", service.Role)
	if service.Password != "" {
		cmd.Printf("PGPASSWORD=%s\n", service.Password)
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

// prepareServiceForOutput builds the output view of a service. cmd may be nil,
// in which case the connection-details warning is dropped rather than printed.
func prepareServiceForOutput(cmd *cobra.Command, cfg *config.Config, service api.Service, withPassword bool) OutputService {
	outputSvc := OutputService{
		Service: service,
	}
	outputSvc.InitialPassword = nil

	opts := common.ConnectionDetailsOptions{
		Role:            "tsdbadmin",
		WithPassword:    withPassword,
		InitialPassword: util.Deref(service.InitialPassword),
	}

	if connectionDetails, err := common.GetConnectionDetails(cfg, service, opts); err != nil {
		if cmd != nil {
			cmd.PrintErrf("⚠️  Warning: Failed to get connection details: %v\n", err)
		}
	} else {
		outputSvc.ConnectionDetails = *connectionDetails
		outputSvc.ConnectionString = connectionDetails.String()
	}

	// Build console URL
	outputSvc.ConsoleURL = fmt.Sprintf("%s/dashboard/services/%s", cfg.ConsoleURL, *service.ServiceId)

	return outputSvc
}

// handlePasswordSaving handles saving password using the configured storage
// method and displaying appropriate messages. Returns true if the password was
// successfully saved, or false if not.
func handlePasswordSaving(cmd *cobra.Command, cfg *config.Config, service api.Service, initialPassword string) bool {
	// Note: We don't fail the service creation if password saving fails
	// The error is handled by displaying the appropriate message below
	result, _ := common.SavePasswordWithResult(cfg, service, initialPassword, "tsdbadmin")

	if result.Method == "none" && result.Message == "No password provided" {
		// Don't output anything for empty password
		return false
	}

	// Output the message with appropriate emoji
	if result.Success {
		cmd.PrintErrf("🔐 %s\n", result.Message)
		return true
	} else if result.Method == "none" {
		cmd.PrintErrf("💡 %s\n", result.Message)
	} else {
		cmd.PrintErrf("⚠️  %s\n", result.Message)
	}
	return false
}

// setDefaultService sets the given service as the default service in the configuration
func setDefaultService(cmd *cobra.Command, cfg *config.Config, serviceID string) error {
	if err := cfg.Set("service_id", serviceID); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	cmd.PrintErrf("🎯 Set service '%s' as default service.\n", serviceID)
	return nil
}

func printConnectMessage(cmd *cobra.Command, passwordSaved, noSetDefault bool, serviceID string) {
	if !passwordSaved {
		// We can't connect if no password was saved, so don't show message
		return
	} else if noSetDefault {
		// If the service wasn't set as the default, include the serviceID in the command
		cmd.PrintErrf("🔌 Run 'tiger db connect %s' to connect to your new service\n", serviceID)
	} else {
		// If the service was set as the default, no need to include the serviceID in the command
		cmd.PrintErrf("🔌 Run 'tiger db connect' to connect to your new service\n")
	}
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
