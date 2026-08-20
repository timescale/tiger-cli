package cmd

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/mcp"
	"github.com/timescale/tiger-cli/internal/util"
)

// buildMCPListCmd creates the list subcommand for displaying available MCP capabilities
func buildMCPListCmd(app *common.App) *cobra.Command {

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List available MCP tools, prompts, and resources",
		Long: `List all MCP tools, prompts, and resources exposed via the Tiger MCP server.

The output can be formatted as a table, JSON, or YAML.

Examples:
  # List all capabilities in table format (default)
  tiger mcp list

  # List as JSON
  tiger mcp list -o json

  # List as YAML
  tiger mcp list -o yaml`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.GetConfig()

			// Create MCP server
			server, err := mcp.NewServer(cmd.Context(), app, nil)
			if err != nil {
				return fmt.Errorf("failed to create MCP server: %w", err)
			}
			defer server.Close()

			// List capabilities
			capabilities, err := server.ListCapabilities(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list capabilities: %w", err)
			}

			// Close the MCP server when finished
			if err := server.Close(); err != nil {
				return fmt.Errorf("failed to close MCP server: %w", err)
			}

			// Format output
			output := cmd.OutOrStdout()
			switch cfg.Output {
			case "json":
				return util.SerializeToJSON(output, capabilities)
			case "yaml":
				return util.SerializeToYAML(output, capabilities)
			default:
				return outputCapabilitiesTable(output, capabilities)
			}
		},
	}

	cmd.Flags().VarP(new(outputFlag), "output", "o", "output format (json, yaml, table)")

	return cmd
}

// outputCapabilitiesTable outputs capabilities in table format. Results are
// ordered alphabetically by type, then name.
func outputCapabilitiesTable(output io.Writer, capabilities *mcp.Capabilities) error {
	table := tablewriter.NewWriter(output)
	table.Header("TYPE", "NAME")

	// Add prompts
	for _, prompt := range capabilities.Prompts {
		table.Append("prompt", prompt.Name)
	}

	// Add resources
	for _, resource := range capabilities.Resources {
		table.Append("resource", resource.Name)
	}

	// Add resource templates
	for _, template := range capabilities.ResourceTemplates {
		table.Append("resource_template", template.Name)
	}

	// Add tools
	for _, tool := range capabilities.Tools {
		table.Append("tool", tool.Name)
	}

	return table.Render()
}
