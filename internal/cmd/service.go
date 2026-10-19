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
// experimental still gates `backups` and `branch`, which are backed by gateway
// endpoints marked `x-tigerdata-preview: true`; when false, they are not added
// to the tree at all. See CLAUDE.md's "Experimental Feature Gating".
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
	cmd.AddCommand(buildServiceRenameCmd(app))
	cmd.AddCommand(buildServiceResizeCmd(app))
	cmd.AddCommand(buildServiceLogsCmd(app))
	cmd.AddCommand(buildServiceMetricsCmd(app))

	// Experimental commands, unregistered until the previews graduate.
	if app.Experimental {
		cmd.AddCommand(buildServiceBackupCmd(app))
		cmd.AddCommand(buildServiceBranchCmd(app))
	}

	return cmd
}

// Tables of CPU/Memory configurations for cpuMemoryHelp: the standard
// platform's, with and without shared/shared, and the fluid platform's.
const (
	standardCPUMemoryHelp = `  shared / shared       |  0.5 CPU (500m) / 2GB    |  1 CPU (1000m) / 4GB     |  2 CPU (2000m) / 8GB
  4 CPU (4000m) / 16GB  |  8 CPU (8000m) / 32GB    |  16 CPU (16000m) / 64GB  |  32 CPU (32000m) / 128GB`
	standardNoSharedCPUMemoryHelp = `  0.5 CPU (500m) / 2GB  |  1 CPU (1000m) / 4GB     |  2 CPU (2000m) / 8GB     |  4 CPU (4000m) / 16GB
  8 CPU (8000m) / 32GB  |  16 CPU (16000m) / 64GB  |  32 CPU (32000m) / 128GB`
	fluidCPUMemoryHelp = `  0.25 CPU (250m) / 2GB  |  0.5 CPU (500m) / 4GB  |  1 CPU (1000m) / 8GB      |  2 CPU (2000m) / 16GB
  4 CPU (4000m) / 32GB   |  8 CPU (8000m) / 64GB  |  16 CPU (16000m) / 128GB  |  32 CPU (32000m) / 256GB`
)

// cpuMemoryHelp renders the allowed CPU/Memory configurations section of a
// command's help text. shared includes shared/shared, which a service can't be
// resized onto. The fluid platform is a preview, so its table is added only
// behind the experimental gate (it has no shared configuration).
func cpuMemoryHelp(shared, experimental bool) string {
	standard := standardNoSharedCPUMemoryHelp
	if shared {
		standard = standardCPUMemoryHelp
	}
	if !experimental {
		return "Allowed CPU/Memory Configurations:\n" + standard
	}
	return "Allowed CPU/Memory Configurations (standard platform):\n" + standard +
		"\n\nAllowed CPU/Memory Configurations (fluid platform):\n" + fluidCPUMemoryHelp
}

// OutputService represents a service with computed fields for output
type OutputService struct {
	api.Service
	common.ConnectionDetails
	ConnectionString string `json:"connection_string,omitempty"`
	ConsoleURL       string `json:"console_url,omitempty"`
}

// outputService formats and outputs a single service based on the specified format
func outputService(cmd *cobra.Command, cfg *config.Config, service api.Service, experimental bool, format string, withPassword bool, strict bool) error {
	// Prepare the output service with computed fields
	outputSvc := prepareServiceForOutput(cmd, cfg, service, experimental, withPassword)
	if strict && withPassword && outputSvc.Password == "" {
		return fmt.Errorf("password requested but not available for service %s", outputSvc.ServiceID)
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
	table.Append("Service ID", service.ServiceID)
	table.Append("Name", service.Name)
	table.Append("Status", string(service.Status))
	if service.Platform != nil {
		table.Append("Platform", string(*service.Platform))
	}
	table.Append("Type", string(service.ServiceType))
	table.Append("Region", service.RegionCode)

	// Environment tag
	if service.Metadata != nil && service.Metadata.Environment != nil {
		table.Append("Environment", *service.Metadata.Environment)
	}

	// Resource information from Resources slice
	if len(service.Resources) > 0 {
		resource := service.Resources[0] // Get first resource
		if resource.Spec != nil {
			if resource.Spec.CPUMillis != nil {
				table.Append("CPU", fmt.Sprintf("%s cores (%dm)", common.FormatCPUCores(*resource.Spec.CPUMillis), *resource.Spec.CPUMillis))
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
	table.Append("Created", service.Created.Format("2006-01-02 15:04:05 MST"))

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
// The platform field is a preview field, so it is dropped unless experimental
// is set.
func prepareServiceForOutput(cmd *cobra.Command, cfg *config.Config, service api.Service, experimental bool, withPassword bool) OutputService {
	outputSvc := OutputService{
		Service: service,
	}
	outputSvc.Service.InitialPassword = nil
	if !experimental {
		outputSvc.Service.Platform = nil
	}

	opts := common.ConnectionDetailsOptions{
		Role:            "tsdbadmin",
		WithPassword:    withPassword,
		InitialPassword: util.Deref(service.InitialPassword),
		ReadOnly:        common.CheckReadOnly(cfg, common.ServiceEnvironmentTag(service)) != nil,
	}

	if connectionDetails, err := common.GetConnectionDetails(cfg, service, opts); err != nil {
		if cmd != nil {
			cmd.PrintErrf("Warning: Failed to get connection details: %v\n", err)
		}
	} else {
		outputSvc.ConnectionDetails = *connectionDetails
		outputSvc.ConnectionString = connectionDetails.String()
	}

	// Build console URL
	outputSvc.ConsoleURL = fmt.Sprintf("%s/dashboard/services/%s", cfg.ConsoleURL, service.ServiceID)

	return outputSvc
}

// setDefaultService sets the given service as the default service in the configuration
func setDefaultService(cmd *cobra.Command, cfg *config.Config, serviceID string) error {
	if _, err := cfg.Set("service_id", serviceID); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	cmd.PrintErrf("Default service set to %s.\n", serviceID)
	return nil
}
