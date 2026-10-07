package cmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceAllowListAttachCmd creates the service allowlist attach subcommand.
func buildServiceAllowListAttachCmd(app *common.App) *cobra.Command {
	var allowListID string

	cmd := &cobra.Command{
		Use:   "attach [name-or-id]",
		Short: "Attach a service to an IP allow list",
		Long: `Restrict a service's connections to the IP ranges in an IP allow list.

A service has at most one IP allow list; attaching a second one without
detaching the first is refused.

The service can be given by ID or name as an argument, or will use the default
service from your configuration.`,
		Example: `  # Attach the default service to an IP allow list
  tiger service allowlist attach --allow-list 1234567890

  # Attach a specific service to an IP allow list
  tiger service allowlist attach svc-12345 --allow-list 1234567890`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			serviceRef, err := getServiceRef(cmd, cfg, args)
			if err != nil {
				return err
			}

			service, err := resolveServiceForWrite(cmd.Context(), cfg, client, projectID, serviceRef)
			if err != nil {
				return err
			}

			resp, err := client.AttachServiceToAllowListWithResponse(cmd.Context(), projectID, service.ServiceID, api.ServiceAllowListInput{
				AllowListID: allowListID,
			})
			if err != nil {
				return fmt.Errorf("failed to attach service to IP allow list: %w", err)
			}

			if resp.StatusCode() != http.StatusAccepted {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON202 == nil {
				return fmt.Errorf("empty response from API")
			}

			cmd.PrintErrf("Service %s attached to IP allow list '%s'.\n", serviceLabel(*service), allowListID)

			switch strings.ToLower(cfg.Output) {
			case "json", "yaml":
				return outputService(cmd, cfg, *resp.JSON202, cfg.Output, false, false)
			default:
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&allowListID, "allow-list", "", "IP allow list to attach the service to")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (json, yaml, table)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	markFlagRequired(cmd, "allow-list")

	return cmd
}
