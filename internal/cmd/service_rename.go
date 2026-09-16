package cmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceRenameCmd creates the rename subcommand
func buildServiceRenameCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <service-id> <new-name>",
		Short: "Rename a database service",
		Long: `Rename a database service.

Only the service's display name changes: its ID, endpoints, and data are
untouched, so existing connections and connection strings keep working.

Both the service and the new name are required. There is no default service
fallback, since a single argument would be ambiguous between the service to
rename and the name to give it.`,
		Example: `  # Rename a service
  tiger service rename svc-12345 analytics-prod`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceID, newName := args[0], strings.TrimSpace(args[1])

			if newName == "" {
				return fmt.Errorf("new name cannot be empty")
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			if err := common.CheckReadOnlyByServiceID(cmd.Context(), cfg, client, projectID, serviceID); err != nil {
				return err
			}

			resp, err := client.RenameServiceWithResponse(
				cmd.Context(),
				api.ProjectID(projectID),
				api.ServiceID(serviceID),
				api.ServiceRename{Name: newName},
			)
			if err != nil {
				return fmt.Errorf("failed to rename service: %w", err)
			}

			if resp.StatusCode() != http.StatusOK {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}
			service := *resp.JSON200

			cmd.Printf("✅ Renamed service '%s' to '%s'.\n", serviceID, service.Name)

			return nil
		},
	}

	return cmd
}
