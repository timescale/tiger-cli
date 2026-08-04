package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildMCPCmd creates the MCP server command with subcommands
func buildMCPCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Tiger Model Context Protocol (MCP) server",
		Long: `Tiger Model Context Protocol (MCP) server for AI assistant integration.

The MCP server provides programmatic access to Tiger Cloud platform resources
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
	cmd.AddCommand(buildMCPInstallCmd(app))
	cmd.AddCommand(buildMCPStartCmd(app))
	cmd.AddCommand(buildMCPListCmd(app))
	cmd.AddCommand(buildMCPGetCmd(app))

	return cmd
}
