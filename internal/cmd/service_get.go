package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceGetCmd represents the get command under service
func buildServiceGetCmd(app *common.App) *cobra.Command {
	var withPassword bool

	cmd := &cobra.Command{
		Use:     "get [service]",
		Aliases: []string{"describe", "show"},
		Short:   "Show detailed information about a service",
		Long: `Show detailed information about a specific database service.

The service can be given by ID or name as an argument, or will use the default
service from your configuration. This command displays comprehensive information about
the service including configuration, status, endpoints, and resource usage.`,
		Example: `  # Get default service details
  tiger service get

  # Get specific service details
  tiger service get svc-12345

  # Get service details in JSON format
  tiger service get svc-12345 --output json

  # Get service details in YAML format
  tiger service get svc-12345 --output yaml`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			// Determine the service ref
			serviceRef, err := getServiceRef(cfg, args)
			if err != nil {
				return err
			}

			// Resolving returns the whole service, so there is nothing left to fetch.
			resolved, err := resolveService(cmd.Context(), client, projectID, serviceRef)
			if err != nil {
				return err
			}
			service := *resolved

			// Output service in requested format
			return outputService(cmd, cfg, service, cfg.Output, withPassword, true)
		},
	}

	cmd.Flags().BoolVar(&withPassword, "with-password", false, "Include password in output")
	cmd.Flags().VarP(new(outputWithEnvFlag), "output", "o", "Output format (json, yaml, env, table)")
	registerFlagCompletion(cmd, "output", outputCompletion("env"))

	return cmd
}
