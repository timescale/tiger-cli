package cmd

import (
	"context"
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

// buildMCPCmd creates the MCP server command
func buildMCPCmd() *cobra.Command {
	var httpPort int
	var httpHost string

	cmd := &cobra.Command{
		Use:   "mcp [stdio|http]",
		Short: "Start the Tiger MCP server",
		Long: `Start the Tiger Model Context Protocol (MCP) server for AI assistant integration.

The MCP server provides programmatic access to TigerData Cloud Platform resources
through Claude and other AI assistants. It mirrors the functionality of the Tiger
CLI and shares the same authentication, configuration, and API client.

The server runs in the foreground and can be stopped with Ctrl+C.

Available transports:
  stdio  Standard input/output transport for AI assistant integration (default)
  http   HTTP server transport for web-based integrations

Examples:
  # Start MCP server with stdio transport (default)
  tiger mcp
  tiger mcp stdio

  # Start HTTP server for web integrations
  tiger mcp http
  tiger mcp http --port 3001
  tiger mcp http --port 8080 --host 0.0.0.0

Configuration:
The MCP server automatically uses the CLI's stored authentication and configuration.
No additional setup is required beyond running 'tiger auth login'.

Available Tools:
  - tiger_service_list             List all services
  - tiger_service_show             Show service details
  - tiger_service_create           Create new services
  - tiger_service_update_password  Update service passwords`,
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []cobra.Completion{"stdio", "http"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine transport (default to stdio)
			transport := "stdio"
			if len(args) > 0 {
				transport = args[0]
			}

			cmd.SilenceUsage = true

			logging.Info("Starting Tiger MCP server",
				zap.String("transport", transport))

			// Create MCP server
			server, err := tigerMCP.NewServer()
			if err != nil {
				return fmt.Errorf("failed to create MCP server: %w", err)
			}

			// Create context for server shutdown
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle interrupts gracefully
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
			quitChan := make(chan struct{})
			doneChan := make(chan struct{})
			defer func() {
				// Wait for signal handler goroutine to exit before returning
				close(quitChan)
				<-doneChan
			}()
			go func() {
				defer close(doneChan)

				// Wait for interrupt signal
				select {
				case <-sigChan:
				case <-quitChan:
				}

				logging.Info("Received interrupt signal, shutting down MCP server...")
				cancel()
			}()

			// Start server with appropriate transport
			switch transport {
			case "stdio":
				logging.Debug("Starting MCP server with stdio transport")
				return server.Run(ctx, &mcp.StdioTransport{})
			case "http":
				logging.Debug("Starting MCP server with http transport")
				return startHTTPServer(ctx, server, httpHost, httpPort)
			default:
				return fmt.Errorf("transport '%s' not implemented", transport)
			}
		},
	}

	// Add HTTP transport flags
	cmd.Flags().IntVar(&httpPort, "port", 8080, "Port to run HTTP server on")
	cmd.Flags().StringVar(&httpHost, "host", "localhost", "Host to bind to")

	return cmd
}

// startHTTPServer starts the MCP server with HTTP transport
func startHTTPServer(ctx context.Context, server *tigerMCP.Server, host string, port int) error {
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

	// Create SSE handler that returns our server for all requests
	sseHandler := mcp.NewSSEHandler(func(req *http.Request) *mcp.Server {
		return server.GetMCPServer()
	})

	// Create HTTP server
	httpServer := &http.Server{
		Handler: sseHandler,
	}

	logging.Info("Starting MCP server with HTTP transport",
		zap.String("address", address))

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
