package cmd

import (
	"context"
	"testing"
)

// TestMCPStartCmd covers the stdio transport, which `tiger mcp start` runs both
// by default and under the explicit `stdio` subcommand.
//
// Every case runs under an already-cancelled context (see withContext), so the
// server starts, registers its capabilities, and unwinds instead of blocking
// on stdin for the length of the test. Cancellation is the normal way the
// server stops, so the command must still exit successfully.
//
// The "server ..." lines come from the MCP SDK rather than from us: they are
// asserted because they land on the same stream as our own (see CLAUDE.md's
// "Logging Architecture"), so a change in what an operator sees is a change
// worth noticing.
func TestMCPStartCmd(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	const stdioLog = `INFO Docs MCP proxy is disabled
INFO server run start
INFO server connecting
INFO server session connected session_id=""
INFO server session disconnected session_id=""
ERROR server run cancelled error="context canceled"
`

	runCmdTests(t, []cmdTest{
		{
			name:       "start defaults to stdio",
			args:       []string{"mcp", "start"},
			opts:       append(noDocsProxy(nil), withContext(cancelled)),
			wantStderr: matchLog(stdioLog),
		},
		{
			name:       "stdio subcommand",
			args:       []string{"mcp", "start", "stdio"},
			opts:       append(noDocsProxy(nil), withContext(cancelled)),
			wantStderr: matchLog(stdioLog),
		},
		{
			name:    "start rejects positional args",
			args:    []string{"mcp", "start", "bogus"},
			wantErr: `unknown command "bogus" for "tiger mcp start"`,
		},
		{
			name:    "stdio rejects positional args",
			args:    []string{"mcp", "start", "stdio", "bogus"},
			wantErr: `unknown command "bogus" for "tiger mcp start stdio"`,
		},
		{
			// Read-only mode removes capabilities, so each skipped write tool
			// is logged: a client that can't find a tool it expected can see
			// why. The read tools stay registered and are not logged.
			name: "read-only mode skips write tools",
			args: []string{"mcp", "start"},
			opts: append(noDocsProxy(map[string]any{"read_only": true}), withContext(cancelled)),
			wantStderr: matchLog(`INFO Skipping write tool in read-only mode tool=service_create
INFO Skipping write tool in read-only mode tool=service_fork
INFO Skipping write tool in read-only mode tool=service_update_password
INFO Skipping write tool in read-only mode tool=service_start
INFO Skipping write tool in read-only mode tool=service_stop
INFO Skipping write tool in read-only mode tool=service_resize
` + stdioLog),
		},
	})
}
