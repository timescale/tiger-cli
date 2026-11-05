package cmd

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// outputLogs outputs log entries, oldest first, in the configured output format.
func outputLogs(cmd *cobra.Command, cfg *config.Config, entries []api.ServiceLogEntry) error {
	outputWriter := cmd.OutOrStdout()
	switch strings.ToLower(cfg.Output) {
	case "json":
		return util.SerializeToJSON(outputWriter, entries)
	case "yaml":
		return util.SerializeToYAML(outputWriter, entries)
	default: // text format (default)
		for _, entry := range entries {
			line := entry.Message
			if !entry.Timestamp.IsZero() {
				// Local timezone for terminal output; MCP and public API use UTC.
				line = entry.Timestamp.Local().Format("2006-01-02 15:04:05 MST") + " " + line
			}
			cmd.Println(colorizeLogEntry(line, entry.Severity))
		}
		return nil
	}
}

// Log severity styles
var (
	logErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Red)
	logWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	logInfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Blue)
	logDebugStyle   = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
)

// colorizeLogEntry colorizes the severity token (e.g. "ERROR:") within the log
// line using the API-provided severity field. Using the structured field avoids
// false positives where a severity word appears in the message body rather than
// as an actual log level.
//
// PostgreSQL severity levels: https://www.postgresql.org/docs/current/runtime-config-logging.html#RUNTIME-CONFIG-SEVERITY-LEVELS
func colorizeLogEntry(line, severity string) string {
	var style lipgloss.Style
	switch strings.ToUpper(severity) {
	case "ERROR", "FATAL", "PANIC":
		style = logErrorStyle
	case "WARNING":
		style = logWarningStyle
	case "LOG", "INFO", "NOTICE":
		style = logInfoStyle
	case "DEBUG":
		style = logDebugStyle
	default:
		return line
	}

	token := strings.ToUpper(severity) + ":"
	return strings.Replace(line, token, style.Render(token), 1)
}
