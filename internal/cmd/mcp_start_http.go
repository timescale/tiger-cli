package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/mcp"
)

// buildMCPHTTPCmd creates the http subcommand with port/host flags
func buildMCPHTTPCmd(app *common.App) *cobra.Command {
	var port int
	var host string

	cmd := &cobra.Command{
		Use:   "http",
		Short: "Start MCP server with HTTP transport",
		Long: `Start the MCP server using HTTP transport.

The server will automatically find an available port if the specified port is busy.`,
		Example: `  # Start HTTP server on default port 8080
  tiger mcp start http

  # Start HTTP server on custom port
  tiger mcp start http --port 3001

  # Start HTTP server on all interfaces
  tiger mcp start http --host 0.0.0.0 --port 8080

  # Start server and bind to specific interface
  tiger mcp start http --host 192.168.1.100 --port 9000`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		SilenceErrors:     true, // HTTP server uses slog for all output, including errors
		RunE: func(cmd *cobra.Command, args []string) error {
			return startHTTPServer(cmd, app, host, port)
		},
	}

	// Add HTTP-specific flags
	cmd.Flags().IntVar(&port, "port", 8080, "Port to run HTTP server on")
	cmd.Flags().StringVar(&host, "host", "localhost", "Host to bind to")

	return cmd
}

// startHTTPServer starts the MCP server with HTTP transport
func startHTTPServer(cmd *cobra.Command, app *common.App, host string, port int) error {
	ctx := cmd.Context()
	logger := newLogger(cmd.ErrOrStderr())

	// Create MCP server
	server, err := mcp.NewServer(ctx, app, logger)
	if err != nil {
		logger.Error("Failed to create MCP server", slog.Any("error", err))
		return fmt.Errorf("failed to create MCP server: %w", err)
	}
	defer server.Close()

	// Find available port and get the listener
	listener, actualPort, err := getListener(host, port)
	if err != nil {
		logger.Error("Failed to get listener",
			slog.String("host", host),
			slog.Int("port", port),
			slog.Any("error", err),
		)
		return fmt.Errorf("failed to get listener: %w", err)
	}
	defer listener.Close()

	if actualPort != port {
		logger.Info("Specified port was busy, using alternative port",
			slog.Int("requested_port", port),
			slog.Int("actual_port", actualPort),
		)
	}

	address := fmt.Sprintf("%s:%d", host, actualPort)

	// Create HTTP server
	httpServer := &http.Server{
		Handler: server.HTTPHandler(),
	}

	logger.Info("Tiger MCP server started", slog.String("address", address))
	logger.Info("Use Ctrl+C to stop the server")

	// Start server in goroutine using the existing listener
	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for a server error or context cancellation. Once canceled, stop
	// handling signals and revert to default signal handling behavior. This
	// allows a second SIGINT/SIGTERM to forcibly kill the server (useful if
	// there's currently an active MCP session but you want to kill it anyways).
	// Note that stop() is idempotent and safe to call multiple times, so it's
	// okay that it's called here and via the deferred call above.
	select {
	case err := <-errCh:
		logger.Error("HTTP server error", slog.Any("error", err))
		return fmt.Errorf("HTTP server error: %w", err)
	case <-ctx.Done():
	}

	// Shutdown server gracefully
	logger.Info("Gracefully shutting down HTTP server, press control-C twice to immediately shutdown")
	if err := httpServer.Shutdown(context.Background()); err != nil {
		logger.Error("Failed to shut down HTTP server", slog.Any("error", err))
		return fmt.Errorf("failed to shut down HTTP server: %w", err)
	}

	// Close the MCP server when finished
	if err := server.Close(); err != nil {
		logger.Error("Failed to close MCP server", slog.Any("error", err))
		return fmt.Errorf("failed to close MCP server: %w", err)
	}
	return nil
}

// getListener finds an available port starting from the specified port and returns the listener
func getListener(host string, startPort int) (net.Listener, int, error) {
	for port := startPort; port < startPort+100; port++ {
		address := fmt.Sprintf("%s:%d", host, port)
		listener, err := net.Listen("tcp", address)
		if err == nil {
			return listener, port, nil
		}
	}
	return nil, 0, fmt.Errorf("no available port found in range %d-%d", startPort, startPort+99)
}
