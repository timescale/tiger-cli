package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// freePort returns a port nothing is listening on, and (when hold is true)
// keeps it occupied for the rest of the test so binding it fails.
func freePort(t *testing.T, hold bool) int {
	t.Helper()
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if hold {
		t.Cleanup(func() { listener.Close() })
	} else if err := listener.Close(); err != nil {
		t.Fatalf("failed to release reserved port: %v", err)
	}
	return port
}

// TestMCPStartHTTPCmd covers the HTTP transport. Like the stdio cases, each
// runs under an already-cancelled context so the server binds, reports itself,
// and then takes its graceful-shutdown path immediately.
//
// `mcp start http` sets SilenceErrors — it reports failures through slog
// before returning them — so its error cases assert stderr explicitly rather
// than letting the table derive Cobra's "Error:" line.
func TestMCPStartHTTPCmd(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	const shutdown = `INFO Use Ctrl+C to stop the server
INFO Gracefully shutting down HTTP server, press control-C twice to immediately shutdown
`

	port := freePort(t, false)
	busyPort := freePort(t, true)

	runCmdTests(t, []cmdTest{
		{
			name: "binds the requested port and shuts down on cancel",
			args: []string{"mcp", "start", "http", "--port", fmt.Sprint(port)},
			opts: append(noDocsProxy(nil), withContext(cancelled)),
			wantStderr: matchLog(fmt.Sprintf(`INFO Docs MCP proxy is disabled
INFO Tiger MCP server started address=localhost:%d
`, port) + shutdown),
		},
		{
			name:    "fails when the requested port is busy",
			args:    []string{"mcp", "start", "http", "--port", fmt.Sprint(busyPort)},
			opts:    append(noDocsProxy(nil), withContext(cancelled)),
			wantErr: fmt.Sprintf("failed to listen: listen tcp 127.0.0.1:%d: bind: address already in use", busyPort),
			wantStderr: matchLog(fmt.Sprintf(`INFO Docs MCP proxy is disabled
ERROR Failed to listen error="listen tcp 127.0.0.1:%d: bind: address already in use"
`, busyPort)),
		},
		{
			name:    "rejects positional args",
			args:    []string{"mcp", "start", "http", "bogus"},
			wantErr: `unknown command "bogus" for "tiger mcp start http"`,
			// SilenceErrors suppresses Cobra's "Error:" line for this command,
			// including for an argument error it reports before RunE runs.
			wantStderr: "",
		},
	})

	// Not in the table: needs a live server rather than an immediately
	// cancelled one. The handler must actually answer MCP requests on the
	// bound port — everything above only proves the process started.
	t.Run("serves MCP requests on the bound port", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		port := freePort(t, false)
		done := make(chan cmdResult, 1)
		go func() {
			// Running runCommand off the test goroutine is fine for the
			// options used here, but withEnv must never be added: t.Setenv
			// isn't safe from another goroutine.
			done <- runCommand(t, []string{"mcp", "start", "http", "--port", fmt.Sprint(port)}, nil,
				append(noDocsProxy(nil), withContext(ctx))...)
		}()

		url := fmt.Sprintf("http://localhost:%d", port)
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
		var resp *http.Response
		var err error
		// The server is starting on another goroutine; a refused connection
		// returns near-instantly, so retry with a pause until it answers.
		for start := time.Now(); ; time.Sleep(10 * time.Millisecond) {
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
			if reqErr != nil {
				t.Fatalf("failed to build request: %v", reqErr)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if resp, err = http.DefaultClient.Do(req); err == nil {
				break
			}
			if time.Since(start) > 10*time.Second {
				t.Fatalf("MCP server never answered on %s: %v", url, err)
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response: %v", err)
		}
		// The full tool list is asserted by TestMCPListCmd; here it only has
		// to prove the handler is wired to the MCP server.
		if !strings.Contains(string(payload), `"service_list"`) {
			t.Errorf("tools/list response does not mention service_list:\n%s", payload)
		}

		cancel()
		if result := <-done; result.err != nil {
			t.Errorf("unexpected error after shutdown: %v", result.err)
		}
	})
}
