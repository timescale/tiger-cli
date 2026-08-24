package cmd

import (
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildServiceLogsCmd creates the logs command for viewing service logs
func buildServiceLogsCmd(app *common.App) *cobra.Command {
	var tail int
	var since time.Time
	var until time.Time
	var node int

	cmd := &cobra.Command{
		Use:     "logs [service-id]",
		Aliases: []string{"log"},
		Short:   "View logs for a service",
		Long: `View logs for a database service.

Fetches and displays logs from the specified service. By default, shows the last
100 log entries. Supports filtering by time range.

The service ID can be provided as an argument or will use the default service
from your configuration.

Examples:
  # View last 100 logs for default service (default behavior)
  tiger service logs

  # View logs for specific service
  tiger service logs svc-12345

  # View logs within a time range
  tiger service logs --since "2024-01-15T09:00:00Z" --until "2024-01-15T10:00:00Z"

  # View logs for a specific node (for services with HA replicas)
  tiger service logs --node 1

  # View last 50 lines
  tiger service logs --tail 50

  # View last 1000 lines
  tiger service logs --tail 1000`,
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

			// Prepare parameters
			var sincePtr *time.Time
			if !since.IsZero() {
				sincePtr = &since
			}

			var untilPtr *time.Time
			if !until.IsZero() {
				untilPtr = &until
			}

			// Check if node flag was explicitly set (0 is a valid node)
			// If not set, omit the parameter and let the backend fetch primaryOrdinal
			var nodePtr *int
			if cmd.Flags().Changed("node") {
				nodePtr = &node
			}

			// Fetch logs with pagination support
			logs, err := common.FetchServiceLogs(cmd.Context(), common.FetchServiceLogsArgs{
				Client:    client,
				ProjectID: projectID,
				ServiceID: serviceID,
				Tail:      tail,
				Since:     sincePtr,
				Until:     untilPtr,
				Node:      nodePtr,
			})
			if err != nil {
				return err
			}

			// Display logs based on output format
			outputWriter := cmd.OutOrStdout()
			switch strings.ToLower(cfg.Output) {
			case "json":
				return util.SerializeToJSON(outputWriter, logs)
			case "yaml":
				return util.SerializeToYAML(outputWriter, logs)
			default: // text format (default)
				// Apply colorization if color is enabled and output is a terminal
				shouldColorize := cfg.Color && util.IsTerminal(outputWriter)
				if shouldColorize {
					// Temporarily enable color for this output
					original := color.NoColor
					defer func() { color.NoColor = original }()
					color.NoColor = false
				}

				for _, entry := range logs {
					line := entry.Message
					if !entry.Timestamp.IsZero() {
						// Local timezone for terminal output; MCP and public API use UTC.
						line = entry.Timestamp.Local().Format("2006-01-02 15:04:05 MST") + " " + line
					}
					cmd.Println(colorizeLogEntry(line, entry.Severity, shouldColorize))
				}
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().IntVar(&tail, "tail", 100, "Number of log lines to show")
	cmd.Flags().TimeVar(&since, "since", time.Time{}, []string{time.RFC3339}, "Fetch logs after this timestamp (RFC3339 format, e.g., 2024-01-15T09:00:00Z)")
	cmd.Flags().TimeVar(&until, "until", time.Time{}, []string{time.RFC3339}, "Fetch logs before this timestamp (RFC3339 format, e.g., 2024-01-15T10:00:00Z)")
	cmd.Flags().IntVar(&node, "node", 0, "Specific service node to fetch logs from (for services with HA replicas, 0 is valid)")
	cmd.Flags().VarP(new(outputFlag), "output", "o", "Output format (text, json, yaml)")
	cmd.RegisterFlagCompletionFunc("output", outputCompletion())

	return cmd
}

// colorizeLogEntry colorizes the severity token (e.g. "ERROR:") within the log
// line using the API-provided severity field. Using the structured field avoids
// false positives where a severity word appears in the message body rather than
// as an actual log level. If colorEnabled is false, returns the line unchanged.
//
// PostgreSQL severity levels: https://www.postgresql.org/docs/current/runtime-config-logging.html#RUNTIME-CONFIG-SEVERITY-LEVELS
func colorizeLogEntry(line, severity string, colorEnabled bool) string {
	if !colorEnabled || severity == "" {
		return line
	}

	var colorFn func(string, ...interface{}) string
	switch strings.ToUpper(severity) {
	case "ERROR", "FATAL", "PANIC":
		colorFn = color.RedString
	case "WARNING":
		colorFn = color.YellowString
	case "LOG", "INFO", "NOTICE":
		colorFn = color.BlueString
	case "DEBUG":
		colorFn = color.MagentaString
	default:
		return line
	}

	token := strings.ToUpper(severity) + ":"
	return strings.Replace(line, token, colorFn(token), 1)
}
