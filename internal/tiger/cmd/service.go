package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oapi-codegen/runtime/types"
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

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceListCmd)
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
			formatUUID(service.ServiceId),
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

// formatUUID formats a UUID pointer, returning empty string if nil
func formatUUID(uuid *types.UUID) string {
	if uuid == nil {
		return ""
	}
	return uuid.String()
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