package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/mcp"
)

// buildMCPStartCmd creates the start subcommand with transport options
func buildMCPStartCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Tiger MCP server",
		Long: `Start the Tiger Model Context Protocol (MCP) server for AI assistant integration.

The MCP server provides programmatic access to Tiger Cloud platform resources
through Claude and other AI assistants. By default, it uses stdio transport.

Examples:
  # Start with stdio transport (default)
  tiger mcp start

  # Start with stdio transport (explicit)
  tiger mcp start stdio

  # Start with HTTP transport
  tiger mcp start http`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default behavior when no subcommand is specified - use stdio
			return startStdioServer(cmd, app)
		},
	}

	// Add transport subcommands
	cmd.AddCommand(buildMCPStdioCmd(app))
	cmd.AddCommand(buildMCPHTTPCmd(app))

	return cmd
}

// startStdioServer starts the MCP server with stdio transport
func startStdioServer(cmd *cobra.Command, app *common.App) error {
	ctx := cmd.Context()
	logger := newLogger(cmd.ErrOrStderr())

	// Create MCP server
	server, err := mcp.NewServer(ctx, app, logger)
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}
	defer server.Close()

	// Start the stdio transport
	if err := server.StartStdio(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	// Close the MCP server when finished
	if err := server.Close(); err != nil {
		return fmt.Errorf("failed to close MCP server: %w", err)
	}
	return nil
}
