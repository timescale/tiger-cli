package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

func buildDbQueryCmd(app *common.App) *cobra.Command {
	var command string
	var file string
	var role string
	var pooled bool
	var readOnly bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:     "query [service-id]",
		Aliases: []string{"sql"},
		Short:   "Execute a SQL query on a database",
		Long: `Execute a SQL query against a database service and display the results.

Unlike 'tiger db connect', this runs the query directly and does not require a
local psql installation.

The service ID can be provided as an argument or will use the default service
from your configuration. You can also pass a read replica set ID to query that
replica.

The query comes from --command, from the SQL file named by --file, or, if
neither is given, from stdin.

Multi-statement queries (semicolon-separated) are supported. Results from all
statements that return rows are displayed. The statements run in an implicit
transaction that commits on success and rolls back on error; a transaction
opened with BEGIN must be committed explicitly or it rolls back when the
connection closes.

Use --read-only to open the session in Tiger Cloud's immutable read-only mode
(writes and DDL are rejected by the server). The global read_only config option
(or TIGER_READ_ONLY) also forces this behavior: read_only=all makes every session
read-only, and read_only=prod makes sessions against services tagged PROD
read-only while leaving DEV services writable.`,
		Example: `  # Select data from a table
  tiger db query svc-12345 -c "SELECT * FROM users LIMIT 5"

  # Query the default service
  tiger db query -c "SELECT now()"

  # Execute DDL
  tiger db query svc-12345 -c "CREATE TABLE todos (id SERIAL PRIMARY KEY, title TEXT)"

  # Multi-statement query
  tiger db query svc-12345 -c "INSERT INTO users (name) VALUES ('alice'); SELECT * FROM users"

  # Run a SQL file
  tiger db query svc-12345 -f schema.sql

  # Read the query from stdin
  echo "SELECT 1" | tiger db query svc-12345
  tiger db query svc-12345 < schema.sql

  # Get the results as JSON
  tiger db query svc-12345 -c "SELECT * FROM users" -o json

  # Query a read replica
  tiger db query rep1234567 -c "SELECT count(*) FROM events"`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: serviceIDCompletion(app),
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, projectID, err := app.GetAll()
			if err != nil {
				return err
			}

			if timeout < 0 {
				return fmt.Errorf("timeout must be positive or zero, got %v", timeout)
			}

			// Resolve the service before reading the query: with no --command
			// or --file, reading comes last so a missing service ID fails
			// immediately instead of after waiting on stdin.
			serviceID, err := getServiceID(cfg, args)
			if err != nil {
				return err
			}

			target, err := common.ResolveConnectionTargetByID(cmd.Context(), client, projectID, serviceID)
			if err != nil {
				return err
			}

			query, err := readQuery(cmd, command, file)
			if err != nil {
				return err
			}

			warnReplicaPooler(cmd, target, pooled)

			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			result, err := common.ExecuteQuery(ctx, cfg, target, common.ExecuteQueryArgs{
				Query:  query,
				Role:   role,
				Pooled: pooled,
				// --read-only only adds to whatever read-only mode already
				// imposes, which under prod mode depends on the target's tag.
				ReadOnly: readOnly || common.CheckReadOnly(cfg, common.ServiceEnvironmentTag(target.ConnectionService)) != nil,
			})
			if err != nil {
				return handleDatabaseError(err, target)
			}

			switch cfg.Output {
			case "json":
				return util.SerializeToJSON(cmd.OutOrStdout(), newDbQueryOutput(result))
			case "yaml":
				return util.SerializeToYAML(cmd.OutOrStdout(), newDbQueryOutput(result))
			default:
				return outputQueryResults(cmd, result)
			}
		},
	}

	cmd.Flags().StringVarP(&command, "command", "c", "", "SQL query to execute (reads from stdin if neither --command nor --file is given)")
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to a SQL file to execute")
	cmd.Flags().StringVar(&role, "role", "tsdbadmin", "Database role/username")
	cmd.Flags().BoolVar(&pooled, "pooled", false, "Use connection pooling")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Open the connection in Tiger Cloud's immutable read-only mode")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Query timeout duration (e.g., 30s, 5m). Use 0 for no timeout")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (table, json, yaml)")
	registerFlagCompletion(cmd, "file", fileCompletion())
	registerFlagCompletion(cmd, "output", outputCompletion())
	cmd.MarkFlagsMutuallyExclusive("command", "file")

	return cmd
}

// dbQueryOutput is the json/yaml rendering of a query result. It matches the
// db_query MCP tool's output shape.
type dbQueryOutput struct {
	ResultSets    []common.ResultSet `json:"result_sets"`
	ExecutionTime string             `json:"execution_time"`
}

func newDbQueryOutput(result *common.QueryResult) dbQueryOutput {
	return dbQueryOutput{
		ResultSets:    result.ResultSets,
		ExecutionTime: result.ExecutionTime.String(),
	}
}

// readQuery returns the SQL to run, from --command, --file, or stdin. It keys
// off whether each flag was given rather than whether its value is non-empty,
// so an explicit `-c ""` is an empty query rather than a silent read of stdin.
func readQuery(cmd *cobra.Command, command, file string) (string, error) {
	var query string
	switch {
	case cmd.Flags().Changed("command"):
		query = command
	case cmd.Flags().Changed("file"):
		data, err := os.ReadFile(util.ExpandPath(file))
		if err != nil {
			return "", fmt.Errorf("failed to read SQL file: %w", err)
		}
		query = string(data)
	default:
		// Reading piped input, so this is deliberately not gated on a TTY —
		// only the hint is, for someone who ran the command with no query.
		if util.IsTerminal(cmd.InOrStdin()) {
			cmd.PrintErrln("Enter your query (press Ctrl+D when done):")
		}
		input, err := util.ReadAll(cmd.Context(), cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("failed to read query: %w", err)
		}
		query = input
	}

	if query == "" {
		return "", errors.New("query cannot be empty")
	}
	// Anything else — whitespace, comments, a stray semicolon — goes through to
	// the database, which reports on it the way it would for any other client.
	return query, nil
}

// outputQueryResults renders each result set as a table, psql-style: one table
// per row-returning statement, the command tag alone for the others.
func outputQueryResults(cmd *cobra.Command, result *common.QueryResult) error {
	for i, rs := range result.ResultSets {
		// A statement that returns no columns (DDL, INSERT/UPDATE/DELETE) has
		// nothing to tabulate, so report what it did instead.
		if len(rs.Columns) == 0 {
			cmd.Println(rs.CommandTag)
			continue
		}

		if err := renderResultSet(cmd, rs); err != nil {
			return fmt.Errorf("failed to render table for result set %d: %w", i, err)
		}
	}

	return nil
}

func renderResultSet(cmd *cobra.Command, rs common.ResultSet) error {
	// Borderless, pipe-separated columns under a single header rule, so the
	// output reads like psql's.
	table := tablewriter.NewTable(cmd.OutOrStdout(),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithHeaderAutoFormat(tw.Off),
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.Border{
				Left:   tw.Off,
				Right:  tw.Off,
				Top:    tw.Off,
				Bottom: tw.Off,
			},
			Settings: tw.Settings{
				Separators: tw.Separators{
					ShowHeader:     tw.On,
					ShowFooter:     tw.Off,
					BetweenRows:    tw.Off,
					BetweenColumns: tw.On,
				},
				Lines: tw.Lines{
					ShowHeaderLine: tw.On,
				},
			},
		}),
	)

	headers := make([]any, len(rs.Columns))
	for i, col := range rs.Columns {
		headers[i] = col.Name
	}
	table.Header(headers...)

	for _, row := range rs.Rows {
		values := make([]any, len(row))
		for i, val := range row {
			if val == nil {
				values[i] = "NULL"
			} else {
				values[i] = *val
			}
		}
		table.Append(values...)
	}

	if err := table.Render(); err != nil {
		return err
	}

	if len(rs.Rows) == 1 {
		cmd.Printf("(%d row)\n\n", len(rs.Rows))
	} else {
		cmd.Printf("(%d rows)\n\n", len(rs.Rows))
	}
	return nil
}
