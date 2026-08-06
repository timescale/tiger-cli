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

// serviceListCmd represents the list command under service
func buildServiceListCmd(app *common.App) *cobra.Command {

	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List all services",
		Long:              `List all database services in the current project.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Make API call to list services
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			resp, err := client.GetServicesWithResponse(ctx, projectID)
			if err != nil {
				return fmt.Errorf("failed to list services: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}
			services := *resp.JSON200

			if len(services) == 0 {
				cmd.PrintErrln("🏜️  No services found! Your project is looking a bit empty.")
				cmd.PrintErrln("🚀 Ready to get started? Create your first service with: tiger service create")
				return nil
			}

			if resp.JSON200 == nil {
				cmd.PrintErrln("🏜️  No services found! Your project is looking a bit empty.")
				cmd.PrintErrln("🚀 Ready to get started? Create your first service with: tiger service create")
				return nil
			}

			// Output services in requested format
			return outputServices(cmd, cfg, services, cfg.Output)
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")

	return cmd
}

// outputServices formats and outputs the services list based on the specified format
func outputServices(cmd *cobra.Command, cfg *config.Config, services []api.Service, format string) error {
	outputServices := prepareServicesForOutput(cmd, cfg, services)

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

// prepareServicesForOutput creates copies of services with sensitive fields removed
func prepareServicesForOutput(cmd *cobra.Command, cfg *config.Config, services []api.Service) []OutputService {
	prepared := make([]OutputService, len(services))
	for i, service := range services {
		prepared[i] = prepareServiceForOutput(cmd, cfg, service, false)
	}
	return prepared
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

// formatTimePtr formats a time pointer, returning empty string if nil
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}
