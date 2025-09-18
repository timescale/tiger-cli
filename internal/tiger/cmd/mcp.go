package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/logging"
	tigerMCP "github.com/timescale/tiger-cli/internal/tiger/mcp"
)

// buildMCPCmd creates the MCP server command with subcommands
func buildMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start the Tiger MCP server",
		Long: `Start the Tiger Model Context Protocol (MCP) server for AI assistant integration.

The MCP server provides programmatic access to TigerData Cloud Platform resources
through Claude and other AI assistants. By default, it uses stdio transport.

Configuration:
The server automatically uses the CLI's stored authentication and configuration.
No additional setup is required beyond running 'tiger auth login'.

Available Tools:
  - tiger_service_list             List all services
  - tiger_service_show             Show service details
  - tiger_service_create           Create new services
  - tiger_service_update_password  Update service passwords`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default behavior when no subcommand is specified - use stdio
			cmd.SilenceUsage = true
			return startStdioServer(cmd.Context())
		},
	}

	// Add subcommands
	cmd.AddCommand(buildMCPStdioCmd())
	cmd.AddCommand(buildMCPHTTPCmd())

	return cmd
}

// buildMCPStdioCmd creates the stdio subcommand
func buildMCPStdioCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stdio",
		Short: "Start MCP server with stdio transport",
		Long: `Start the MCP server using standard input/output transport.

Examples:
  # Start with stdio transport (same as 'tiger mcp')
  tiger mcp stdio`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return startStdioServer(cmd.Context())
		},
	}
}

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
  tiger mcp http

  # Start HTTP server on custom port
  tiger mcp http --port 3001

  # Start HTTP server on all interfaces
  tiger mcp http --host 0.0.0.0 --port 8080

  # Start server and bind to specific interface
  tiger mcp http --host 192.168.1.100 --port 9000`,
		Args: cobra.NoArgs,
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

// startStdioServer starts the MCP server with stdio transport
func startStdioServer(ctx context.Context) error {
	logging.Info("Starting Tiger MCP server", zap.String("transport", "stdio"))

	// Create MCP server
	server, err := tigerMCP.NewServer()
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	// Setup graceful shutdown handling
	ctx, stop := signalContext(ctx)
	defer stop()

	if err := server.Run(ctx, &mcp.StdioTransport{}); !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// startHTTPServer starts the MCP server with HTTP transport
func startHTTPServer(ctx context.Context, host string, port int) error {
	logging.Info("Starting Tiger MCP server", zap.String("transport", "http"))

	// Create MCP server
	server, err := tigerMCP.NewServer()
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	// Setup graceful shutdown handling
	ctx, stop := signalContext(ctx)
	defer stop()

	// Find available port and get the listener
	listener, actualPort, err := getListener(host, port)
	if err != nil {
		return fmt.Errorf("failed to get listener: %w", err)
	}
	defer listener.Close()

	if actualPort != port {
		logging.Info("Specified port was busy, using alternative port",
			zap.Int("requested_port", port),
			zap.Int("actual_port", actualPort))
	}

	address := fmt.Sprintf("%s:%d", host, actualPort)

	// Create streamable HTTP handler that returns our server for all requests
	httpHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server.GetMCPServer()
	}, nil)

	// Create HTTP server
	httpServer := &http.Server{
		Handler: httpHandler,
	}

	fmt.Printf("🚀 Tiger MCP server listening on http://%s\n", address)
	fmt.Printf("💡 Use Ctrl+C to stop the server\n")

	// Start server in goroutine using the existing listener
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server gracefully
	logging.Info("Shutting down HTTP server...")
	return httpServer.Shutdown(context.Background())
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

// signalContext sets up graceful shutdown handling and returns a context and
// cleanup function. This is nearly identical to [signal.NotifyContext], except
// that it logs a message when a signal is received.
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigChan:
			logging.Info("Received interrupt signal, shutting down MCP server...",
				zap.Stringer("signal", sig),
			)
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, func() {
		cancel()
		signal.Stop(sigChan)
	}
}
