package cmd

import (
	"io"
	"log"
	"log/slog"
)

// newLogger configures the default log package to write to w and returns the
// default slog logger. It's intended for the MCP server, the only long-running,
// backend-like process the CLI hosts; ordinary commands write to
// stdout/stderr directly rather than logging.
func newLogger(w io.Writer) *slog.Logger {
	log.SetOutput(w)
	return slog.Default()
}
