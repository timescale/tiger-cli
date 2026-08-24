package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceGetCmd represents the get command under service
func buildServiceGetCmd(app *common.App) *cobra.Command {
	var withPassword bool

	cmd := &cobra.Command{
		Use:     "get [service-id]",
		Aliases: []string{"describe", "show"},
		Short:   "Show detailed information about a service",
		Long: `Show detailed information about a specific database service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command displays comprehensive information about
the service including configuration, status, endpoints, and resource usage.

Examples:
  # Get default service details
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

			// Determine service ID
			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			// Make API call to get service details
			resp, err := client.GetServiceWithResponse(cmd.Context(), projectID, serviceID)
			if err != nil {
				return fmt.Errorf("failed to get service details: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}
			service := *resp.JSON200

			// Output service in requested format
			return outputService(cmd, cfg, service, cfg.Output, withPassword, true)
		},
	}

	cmd.Flags().BoolVar(&withPassword, "with-password", false, "Include password in output")
	cmd.Flags().VarP(new(outputWithEnvFlag), "output", "o", "Output format (json, yaml, env, table)")
	cmd.RegisterFlagCompletionFunc("output", outputCompletion("env"))

	return cmd
}
