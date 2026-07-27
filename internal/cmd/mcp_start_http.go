package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/logging"
	"github.com/timescale/tiger-cli/internal/mcp"
)

// buildMCPHTTPCmd creates the http subcommand with port/host flags
func buildMCPHTTPCmd() *cobra.Command {
	var httpPort int
	var httpHost string

	cmd := &cobra.Command{
		Use:   "http",
		Short: "Start MCP server with HTTP transport",
		Long: `Start the MCP server using HTTP transport.

The server will automatically find an available port if the specified port is busy.

Examples:
  # Start HTTP server on default port 8080
  tiger mcp start http

  # Start HTTP server on custom port
  tiger mcp start http --port 3001

  # Start HTTP server on all interfaces
  tiger mcp start http --host 0.0.0.0 --port 8080

  # Start server and bind to specific interface
  tiger mcp start http --host 192.168.1.100 --port 9000`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return startHTTPServer(cmd.Context(), httpHost, httpPort)
		},
	}

	// Add HTTP-specific flags
	cmd.Flags().IntVar(&httpPort, "port", 8080, "Port to run HTTP server on")
	cmd.Flags().StringVar(&httpHost, "host", "localhost", "Host to bind to")

	return cmd
}

// startHTTPServer starts the MCP server with HTTP transport
func startHTTPServer(ctx context.Context, host string, port int) error {
	logging.Info("Starting Tiger MCP server", zap.String("transport", "http"))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create MCP server
	server, err := mcp.NewServer(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}
	defer server.Close()

	// Find available port and get the listener
	listener, actualPort, err := getListener(host, port)
	if err != nil {
		return fmt.Errorf("failed to get listener: %w", err)
	}
	defer listener.Close()

	if actualPort != port {
		logging.Info("Specified port was busy, using alternative port",
			zap.Int("requested_port", port),
			zap.Int("actual_port", actualPort),
		)
	}

	address := fmt.Sprintf("%s:%d", host, actualPort)

	// Create HTTP server
	httpServer := &http.Server{
		Handler: server.HTTPHandler(),
	}

	fmt.Printf("🚀 Tiger MCP server listening on http://%s\n", address)
	fmt.Printf("💡 Use Ctrl+C to stop the server\n")

	// Start server in goroutine using the existing listener
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Wait for context cancellation. Once canceled, stop handling signals and
	// revert to default signal handling behavior. This allows a second
	// SIGINT/SIGTERM to forcibly kill the server (useful if there's currently
	// an active MCP session but you want to kill it anyways). Note that stop()
	// is idempotent and safe to call multiple times, so it's okay that it's
	// called here and via the deferred call above.
	<-ctx.Done()

	// Shutdown server gracefully
	logging.Info("Gracefully shutting down HTTP server..., press control-C twice to immediately shutdown")
	if err := httpServer.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("failed to shut down HTTP server: %w", err)
	}

	// Close the MCP server when finished
	if err := server.Close(); err != nil {
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
