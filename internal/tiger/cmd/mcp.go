package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/mcp"
)

// buildMCPCmd creates the MCP server command with subcommands
func buildMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Tiger Model Context Protocol (MCP) server",
		Long: `Tiger Model Context Protocol (MCP) server for AI assistant integration.

The MCP server provides programmatic access to TigerData Cloud Platform resources
through Claude and other AI assistants. It exposes Tiger CLI functionality as MCP
tools that can be called by AI agents.

Configuration:
The server automatically uses the CLI's stored authentication and configuration.
No additional setup is required beyond running 'tiger auth login'.

Use 'tiger mcp start' to launch the MCP server.`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			// Show help when no subcommand is specified
			cmd.Help()
		},
	}

	// Add subcommands
	cmd.AddCommand(buildMCPInstallCmd())
	cmd.AddCommand(buildMCPStartCmd())

	return cmd
}

// buildMCPInstallCmd creates the install subcommand for configuring editors
func buildMCPInstallCmd() *cobra.Command {
	var noBackup bool
	var configPath string

	cmd := &cobra.Command{
		Use:   "install [client]",
		Short: "Install and configure Tiger MCP server for a client",
		Long: fmt.Sprintf(`Install and configure the Tiger MCP server for a specific MCP client or AI assistant.

This command automates the configuration process by modifying the appropriate
configuration files for the specified client.

%s
The command will:
- Automatically detect the appropriate configuration file location
- Create the configuration directory if it doesn't exist
- Create a backup of existing configuration by default
- Merge with existing MCP server configurations (doesn't overwrite other servers)
- Validate the configuration after installation

If no client is specified, you'll be prompted to select one interactively.

Examples:
  # Interactive client selection
  tiger mcp install

  # Install for Claude Code (User scope - available in all projects)
  tiger mcp install claude-code

  # Install for Cursor IDE
  tiger mcp install cursor

  # Install without creating backup
  tiger mcp install claude-code --no-backup

  # Use custom configuration file path
  tiger mcp install claude-code --config-path ~/custom/config.json`, generateSupportedEditorsHelp()),
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: getValidEditorNames(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			var clientName string
			if len(args) == 0 {
				// No client specified, prompt user to select one
				var err error
				clientName, err = selectClientInteractively(cmd.OutOrStdout())
				if err != nil {
					return fmt.Errorf("failed to select client: %w", err)
				}
				if clientName == "" {
					return fmt.Errorf("no client selected")
				}
			} else {
				clientName = args[0]
			}

			return installMCPForClient(clientName, !noBackup, configPath)
		},
	}

	// Add flags
	cmd.Flags().BoolVar(&noBackup, "no-backup", false, "Skip creating backup of existing configuration (default: create backup)")
	cmd.Flags().StringVar(&configPath, "config-path", "", "Custom path to configuration file (overrides default locations)")

	return cmd
}

// buildMCPStartCmd creates the start subcommand with transport options
func buildMCPStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Tiger MCP server",
		Long: `Start the Tiger Model Context Protocol (MCP) server for AI assistant integration.

The MCP server provides programmatic access to TigerData Cloud Platform resources
through Claude and other AI assistants. By default, it uses stdio transport.

Examples:
  # Start with stdio transport (default)
  tiger mcp start

  # Start with stdio transport (explicit)
  tiger mcp start stdio

  # Start with HTTP transport
  tiger mcp start http`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default behavior when no subcommand is specified - use stdio
			cmd.SilenceUsage = true
			return startStdioServer(cmd.Context())
		},
	}

	// Add transport subcommands
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
  # Start with stdio transport
  tiger mcp start stdio`,
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
  tiger mcp start http

  # Start HTTP server on custom port
  tiger mcp start http --port 3001

  # Start HTTP server on all interfaces
  tiger mcp start http --host 0.0.0.0 --port 8080

  # Start server and bind to specific interface
  tiger mcp start http --host 192.168.1.100 --port 9000`,
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
	server, err := mcp.NewServer(ctx)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	// Start the stdio transport
	err = server.StartStdio(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	// Close the MCP server when finished
	if err := server.Close(); err != nil {
		return fmt.Errorf("failed to close MCP server: %w", err)
	}
	return nil
}

// startHTTPServer starts the MCP server with HTTP transport
func startHTTPServer(ctx context.Context, host string, port int) error {
	logging.Info("Starting Tiger MCP server", zap.String("transport", "http"))

	// Create MCP server
	server, err := mcp.NewServer(ctx)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

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
	logging.Info("Shutting down HTTP server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
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
