package cmd

import (
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

func buildDbExtensionsListCmd(app *common.App) *cobra.Command {
	var role string

	cmd := &cobra.Command{
		Use:     "list [name-or-id]",
		Aliases: []string{"ls"},
		Short:   "List the PostgreSQL extensions installed in a database",
		Long: `List the PostgreSQL extensions installed in a database service, with each
extension's version, the schema it lives in, and its description.

The service can be given by ID or name as an argument, or will use the default
service from your configuration. You can also pass a read replica set ID to
list the extensions on that replica. The connection is opened in Tiger Cloud's
immutable read-only mode.` + branchRefHelp(app.Experimental, "You can also pass a branch, by ID or name, to list the extensions on that\nbranch."),
		Example: `  # List the extensions installed in the default service
  tiger db extensions list

  # List the extensions installed in a specific service
  tiger db extensions list svc-12345

  # Output as JSON
  tiger db extensions list svc-12345 -o json`,
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

			target, err := resolveConnectionTarget(cmd.Context(), client, projectID, serviceRef, app.Experimental)
			if err != nil {
				return err
			}

			extensions, err := common.FetchExtensions(cmd.Context(), cfg, target, role)
			if err != nil {
				return handleDatabaseError(err, target)
			}

			// A bare list, where the db_extensions_list MCP tool wraps it in an
			// object because structured tool output must be one.
			switch cfg.Output {
			case "json":
				return util.SerializeToJSON(cmd.OutOrStdout(), extensions)
			case "yaml":
				return util.SerializeToYAML(cmd.OutOrStdout(), extensions)
			default:
				return outputExtensionsTable(cmd.OutOrStdout(), extensions)
			}
		},
	}

	cmd.Flags().StringVar(&role, "role", "tsdbadmin", "Database role/username")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (table, json, yaml)")
	registerFlagCompletion(cmd, "output", outputCompletion())

	return cmd
}

func outputExtensionsTable(w io.Writer, extensions []common.Extension) error {
	table := tablewriter.NewWriter(w)
	table.Header("NAME", "VERSION", "SCHEMA", "DESCRIPTION")

	for _, ext := range extensions {
		if err := table.Append(ext.Name, ext.Version, ext.Schema, ext.Description); err != nil {
			return err
		}
	}

	return table.Render()
}
