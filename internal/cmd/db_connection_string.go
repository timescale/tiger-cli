package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildDbConnectionStringCmd(app *common.App) *cobra.Command {
	var dbConnectionStringPooled bool
	var dbConnectionStringRole string
	var dbConnectionStringWithPassword bool
	var dbConnectionStringReadOnly bool

	cmd := &cobra.Command{
		Use:     "connection-string [service-id]",
		Aliases: []string{"uri"},
		Short:   "Get connection string for a service",
		Long: `Get a PostgreSQL connection string for connecting to a database service.

The service ID can be provided as an argument or will use the default service
from your configuration. The connection string includes all necessary parameters
for establishing a database connection to the TimescaleDB/PostgreSQL service.

You can also pass a read replica set ID to get a connection string for that replica.

By default, passwords are excluded from the connection string for security.
Use --with-password to include the password directly in the connection string.

Use --read-only to emit a connection string that opens the session in Tiger
Cloud's immutable read-only mode (writes and DDL are rejected by the server).
The global read_only config option (or TIGER_READ_ONLY) also forces this
behavior: read_only=all makes every connection string read-only, and
read_only=prod makes those for services tagged PROD read-only while leaving DEV
services writable.

Examples:
  # Get connection string for default service
  tiger db connection-string

  # Get connection string for specific service
  tiger db connection-string svc-12345

  # Get pooled connection string (uses connection pooler if available)
  tiger db connection-string svc-12345 --pooled

  # Get connection string with custom role/username
  tiger db connection-string svc-12345 --role readonly

  # Get a read-only connection string
  tiger db connection-string svc-12345 --read-only

  # Get connection string with password included (less secure)
  tiger db connection-string svc-12345 --with-password`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			target, err := common.ResolveConnectionTargetByID(cmd.Context(), client, projectID, serviceID)
			if err != nil {
				return err
			}

			warnReplicaPooler(cmd, target, dbConnectionStringPooled)

			details, err := target.Details(cfg, common.ConnectionDetailsOptions{
				Pooled:       dbConnectionStringPooled,
				Role:         dbConnectionStringRole,
				WithPassword: dbConnectionStringWithPassword,
				ReadOnly:     dbConnectionStringReadOnly || common.CheckReadOnly(cfg, common.ServiceEnvironmentTag(target.ConnectionService)) != nil,
			})
			if err != nil {
				return err
			}

			if dbConnectionStringWithPassword && details.Password == "" {
				return fmt.Errorf("password not available to include in connection string")
			}

			cmd.Println(details.String())
			return nil
		},
	}

	// Add flags for db connection-string command
	cmd.Flags().BoolVar(&dbConnectionStringPooled, "pooled", false, "Use connection pooling")
	cmd.Flags().StringVar(&dbConnectionStringRole, "role", "tsdbadmin", "Database role/username")
	cmd.Flags().BoolVar(&dbConnectionStringWithPassword, "with-password", false, "Include password in connection string (less secure)")
	cmd.Flags().BoolVar(&dbConnectionStringReadOnly, "read-only", false, "Open the connection in Tiger Cloud's immutable read-only mode")

	return cmd
}
