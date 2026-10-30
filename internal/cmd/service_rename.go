package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

// buildServiceRenameCmd creates the rename subcommand
func buildServiceRenameCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <name-or-id> <new-name>",
		Short: "Rename a database service",
		Long: `Rename a database service.

Only the service's display name changes: its ID, endpoints, and data are
untouched, so existing connections and connection strings keep working.

Both the service and the new name are required. There is no default service
fallback, since a single argument would be ambiguous between the service to
rename and the name to give it. The service to rename can be given by ID or by
its current name.`,
		Example: `  # Rename a service
  tiger service rename svc-12345 analytics-prod`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: serviceRefCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceArg, newName := args[0], strings.TrimSpace(args[1])

			if serviceArg == "" {
				return errors.New("service name or ID is required")
			}

			if newName == "" {
				return errors.New("new name cannot be empty")
			}

			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			service, err := resolveServiceForWrite(cmd.Context(), cfg, client, projectID, argServiceRef(serviceArg))
			if err != nil {
				return err
			}

			renamed, err := common.JSON200(client.RenameServiceWithResponse(
				cmd.Context(),
				api.ProjectID(projectID),
				api.ServiceID(service.ServiceID),
				api.ServiceRename{Name: newName},
			))
			if err != nil {
				return err
			}

			cmd.Printf("Renamed service %s to '%s'.\n", serviceLabel(*service), renamed.Name)

			return nil
		},
	}

	return cmd
}
