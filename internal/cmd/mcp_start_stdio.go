package cmd

import (
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
)

// buildMCPStdioCmd creates the stdio subcommand
func buildMCPStdioCmd(app *common.App) *cobra.Command {
	return &cobra.Command{
		Use:   "stdio",
		Short: "Start MCP server with stdio transport",
		Long:  `Start the MCP server using standard input/output transport.`,
		Example: `  # Start with stdio transport
  tiger mcp start stdio`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return startStdioServer(cmd, app)
		},
	}
}
