package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

func buildDbSchemaCmd(app *common.App) *cobra.Command {
	var dbSchemaSchema string
	var dbSchemaInternal bool
	var dbSchemaDefinitions bool
	var dbSchemaComments bool
	var dbSchemaRole string
	var dbSchemaPooled bool

	cmd := &cobra.Command{
		Use:   "schema [service-id]",
		Short: "Display database schema information",
		Long: `Display the schema of a database service: tables (regular, partitioned, and
foreign), views, materialized views, enum types, functions, procedures,
indexes, triggers, and TimescaleDB hypertable and continuous aggregate
metadata.

The service ID can be provided as an argument or will use the default service
from your configuration. You can also pass a read replica set ID to introspect
that replica. Only objects the connecting role can access are returned. The
connection is opened in Tiger Cloud's immutable read-only mode.

By default only user-facing schemas and objects are shown. View and routine
definitions and object comments are omitted unless requested, since they can be
large and may embed implementation details.

Examples:
  # Show the schema of the default service
  tiger db schema

  # Show the schema of a specific service
  tiger db schema svc-12345

  # Restrict to a single schema
  tiger db schema svc-12345 --schema public

  # Include view/function definitions and comments
  tiger db schema svc-12345 --definitions --comments

  # Include catalog, TimescaleDB internals, and extension-owned objects
  tiger db schema svc-12345 --internal`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, _, err := app.GetAll()
			if err != nil {
				return err
			}

			target, err := lookupConnectionTarget(cmd, app, args)
			if err != nil {
				return err
			}

			warnReplicaPooler(cmd, target, dbSchemaPooled)

			schema, err := common.FetchServiceSchema(cmd.Context(), cfg, target, dbSchemaRole, dbSchemaPooled, common.SchemaOptions{
				Schema:             dbSchemaSchema,
				IncludeInternal:    dbSchemaInternal,
				IncludeDefinitions: dbSchemaDefinitions,
				IncludeComments:    dbSchemaComments,
			})
			if err != nil {
				return err
			}

			cmd.Print(common.FormatSchema(schema))
			return nil
		},
	}

	cmd.Flags().StringVar(&dbSchemaSchema, "schema", "", "Restrict output to a single schema")
	cmd.Flags().BoolVar(&dbSchemaInternal, "internal", false, "Include system schemas (pg_*, information_schema, TimescaleDB internals) and extension-owned objects")
	cmd.Flags().BoolVar(&dbSchemaDefinitions, "definitions", false, "Include full object definitions (view SELECTs, function/procedure bodies)")
	cmd.Flags().BoolVar(&dbSchemaComments, "comments", false, "Include object comments (COMMENT ON text)")
	cmd.Flags().StringVar(&dbSchemaRole, "role", "tsdbadmin", "Database role/username")
	cmd.Flags().BoolVar(&dbSchemaPooled, "pooled", false, "Use connection pooling")

	return cmd
}
