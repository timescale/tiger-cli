package cmd

import (
	"io"
	"strings"

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
		Use:          "list",
		Aliases:      []string{"ls"},
		Short:        "List all services",
		Long:         `List all database services in the current project.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Make API call to list services
			services, err := common.JSON200(client.GetServicesWithResponse(cmd.Context(), projectID, nil))
			if err != nil {
				return err
			}
			return outputServices(cmd, cfg, services, serviceOutputOptions{Experimental: app.Experimental})
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	return cmd
}

// outputServices outputs the services list in the configured output format.
func outputServices(cmd *cobra.Command, cfg *config.Config, services []api.Service, opts serviceOutputOptions) error {
	outputServices := prepareServicesForOutput(cmd, cfg, services, opts)

	outputWriter := cmd.OutOrStdout()

	switch strings.ToLower(cfg.Output) {
	case "json":
		return util.SerializeToJSON(outputWriter, outputServices)
	case "yaml":
		return util.SerializeToYAML(outputWriter, outputServices)
	default: // table format (default)
		if len(outputServices) == 0 {
			cmd.PrintErrln("No services found.")
			cmd.PrintErrln("Create your first service with: tiger service create")
			return nil
		}
		return outputServicesTable(outputWriter, outputServices)
	}
}

// prepareServicesForOutput creates copies of services with sensitive fields removed
func prepareServicesForOutput(cmd *cobra.Command, cfg *config.Config, services []api.Service, opts serviceOutputOptions) []OutputService {
	prepared := make([]OutputService, len(services))
	for i, service := range services {
		prepared[i] = prepareServiceForOutput(cmd, cfg, service, opts)
	}
	return prepared
}

// outputServicesTable outputs services in a formatted table using tablewriter
func outputServicesTable(output io.Writer, services []OutputService) error {
	table := tablewriter.NewWriter(output)
	table.Header("SERVICE ID", "NAME", "STATUS", "TYPE", "REGION", "CREATED")

	for _, service := range services {
		table.Append(
			service.ServiceID,
			service.Name,
			string(service.Status),
			string(service.ServiceType),
			service.RegionCode,
			service.Created.Format("2006-01-02 15:04"),
		)
	}

	return table.Render()
}
