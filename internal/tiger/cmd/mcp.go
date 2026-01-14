package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/mcp"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// buildMCPCmd creates the MCP server command with subcommands
func buildMCPCmd() *cobra.Command {
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
	cmd.AddCommand(buildMCPInstallCmd())
	cmd.AddCommand(buildMCPStartCmd())
	cmd.AddCommand(buildMCPListCmd())
	cmd.AddCommand(buildMCPGetCmd())

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
				clientName, err = selectClientInteractively(cmd.ErrOrStderr())
				if err != nil {
					return fmt.Errorf("failed to select client: %w", err)
				}
				if clientName == "" {
					return fmt.Errorf("no client selected")
				}
			} else {
				clientName = args[0]
			}

			return installTigerMCPForClient(clientName, !noBackup, configPath)
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
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
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

// buildMCPListCmd creates the list subcommand for displaying available MCP capabilities
func buildMCPListCmd() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available MCP tools, prompts, and resources",
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
		PreRunE:           bindFlags("output"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Create MCP server
			server, err := mcp.NewServer(cmd.Context())
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

	cmd.Flags().VarP((*outputFlag)(&outputFormat), "output", "o", "output format (json, yaml, table)")

	return cmd
}

// buildMCPGetCmd creates the get subcommand for displaying detailed info on a specific MCP capability
func buildMCPGetCmd() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "get <type> <name>",
		Short: "Get detailed information about a specific MCP capability",
		Long: `Get detailed information about a specific MCP tool, prompt, resource, or resource template.

The type argument must be one of: tool, prompt, resource, resource_template

Examples:
  # Get details about a tool
  tiger mcp get tool service_create

  # Get details about a prompt
  tiger mcp get prompt setup-timescaledb-hypertables

  # Get details as JSON
  tiger mcp get tool service_create -o json

  # Get details as YAML
  tiger mcp get tool service_create -o yaml`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: mcpGetCompletion,
		PreRunE:           bindFlags("output"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate capability type
			capabilityType, err := mcp.ValidateCapabilityType(args[0])
			if err != nil {
				return err
			}
			capabilityName := args[1]

			cmd.SilenceUsage = true

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Create MCP server
			server, err := mcp.NewServer(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to create MCP server: %w", err)
			}
			defer server.Close()

			// List all capabilities
			capabilities, err := server.ListCapabilities(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list capabilities: %w", err)
			}

			// Close the MCP server when finished
			if err := server.Close(); err != nil {
				return fmt.Errorf("failed to close MCP server: %w", err)
			}

			// Find the specific capability
			var (
				capability any
				found      bool
			)
			switch capabilityType {
			case mcp.CapabilityTypeTool:
				capability, found = capabilities.GetTool(capabilityName)
			case mcp.CapabilityTypePrompt:
				capability, found = capabilities.GetPrompt(capabilityName)
			case mcp.CapabilityTypeResource:
				capability, found = capabilities.GetResource(capabilityName)
			case mcp.CapabilityTypeResourceTemplate:
				capability, found = capabilities.GetResourceTemplate(capabilityName)
			default:
				return fmt.Errorf("unsupported capability type: %s", capabilityType)
			}

			if !found {
				return fmt.Errorf("%s %q not found", capabilityType, capabilityName)
			}

			// Format output
			output := cmd.OutOrStdout()
			switch cfg.Output {
			case "json":
				return util.SerializeToJSON(output, capability)
			case "yaml":
				return util.SerializeToYAML(output, capability)
			default:
				switch c := capability.(type) {
				case *mcpsdk.Tool:
					return outputToolText(output, c)
				case *mcpsdk.Prompt:
					return outputPromptText(output, c)
				case *mcpsdk.Resource:
					return outputResourceText(output, c)
				case *mcpsdk.ResourceTemplate:
					return outputResourceTemplateText(output, c)
				default:
					return fmt.Errorf("unsupported capability type: %T", c)
				}
			}
		},
	}

	cmd.Flags().VarP((*outputFlag)(&outputFormat), "output", "o", "output format (json, yaml, table)")

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

// startHTTPServer starts the MCP server with HTTP transport
func startHTTPServer(ctx context.Context, host string, port int) error {
	logging.Info("Starting Tiger MCP server", zap.String("transport", "http"))

	// Create MCP server
	server, err := mcp.NewServer(ctx)
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

// outputToolText outputs a tool in text format
func outputToolText(output io.Writer, tool *mcpsdk.Tool) error {
	var lines []string

	// Title line with annotation tags
	titleLine := tool.Title
	if titleLine == "" {
		titleLine = tool.Name
	}

	// Add annotation tags to title (each in separate brackets)
	if tool.Annotations != nil {
		var tags []string
		ann := tool.Annotations

		if ann.ReadOnlyHint {
			tags = append(tags, "[read-only]")
		}
		if !ann.ReadOnlyHint && ann.IdempotentHint {
			tags = append(tags, "[idempotent]")
		}
		if !ann.ReadOnlyHint && ann.DestructiveHint != nil && *ann.DestructiveHint {
			tags = append(tags, "[destructive]")
		}
		if ann.OpenWorldHint != nil && *ann.OpenWorldHint {
			tags = append(tags, "[open-world]")
		}

		if len(tags) > 0 {
			titleLine += " " + strings.Join(tags, " ")
		}
	}

	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Tool name
	lines = append(lines, "Tool name: "+tool.Name)
	lines = append(lines, "")

	// Description
	if tool.Description != "" {
		lines = append(lines, "Description:")
		lines = append(lines, tool.Description)
		lines = append(lines, "")
	}

	// Parameters (input schema)
	if tool.InputSchema != nil {
		formatted := formatJSONSchema(tool.InputSchema, 1)
		if formatted != "" {
			lines = append(lines, "Parameters:")
			lines = append(lines, formatted)
			lines = append(lines, "")
		}
	}

	// Output schema
	if tool.OutputSchema != nil {
		formatted := formatJSONSchema(tool.OutputSchema, 1)
		if formatted != "" {
			lines = append(lines, "Output:")
			lines = append(lines, formatted)
			lines = append(lines, "")
		}
	}

	// Write output
	_, err := fmt.Fprintln(output, strings.Join(lines, "\n"))
	return err
}

// outputPromptText outputs a prompt in text format
func outputPromptText(output io.Writer, prompt *mcpsdk.Prompt) error {
	var lines []string

	// Title line
	titleLine := prompt.Title
	if titleLine == "" {
		titleLine = prompt.Name
	}

	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Prompt name
	lines = append(lines, "Prompt name: "+prompt.Name)
	lines = append(lines, "")

	// Description
	if prompt.Description != "" {
		lines = append(lines, "Description:")
		lines = append(lines, prompt.Description)
		lines = append(lines, "")
	}

	// Arguments (formatted as bullet list)
	if len(prompt.Arguments) > 0 {
		lines = append(lines, "Arguments:")
		lines = append(lines, formatPromptArguments(prompt.Arguments))
		lines = append(lines, "")
	}

	// Write output
	_, err := fmt.Fprintln(output, strings.Join(lines, "\n"))
	return err
}

// outputResourceText outputs a resource in text format
func outputResourceText(output io.Writer, resource *mcpsdk.Resource) error {
	var lines []string

	// Title line
	titleLine := resource.Title
	if titleLine == "" {
		titleLine = resource.Name
	}

	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Resource name
	lines = append(lines, "Resource name: "+resource.Name)
	lines = append(lines, "")

	// Description
	if resource.Description != "" {
		lines = append(lines, "Description:")
		lines = append(lines, resource.Description)
		lines = append(lines, "")
	}

	// URI
	lines = append(lines, "URI: "+resource.URI)
	lines = append(lines, "")

	// Optional fields
	if resource.MIMEType != "" {
		lines = append(lines, "MIME Type: "+resource.MIMEType)
		lines = append(lines, "")
	}

	if resource.Size > 0 {
		lines = append(lines, fmt.Sprintf("Size: %d bytes", resource.Size))
		lines = append(lines, "")
	}

	// Annotations
	if resource.Annotations != nil {
		var annotations []string
		ann := resource.Annotations

		if len(ann.Audience) > 0 {
			audiences := make([]string, len(ann.Audience))
			for i, role := range ann.Audience {
				audiences[i] = string(role)
			}
			annotations = append(annotations, fmt.Sprintf("  • Audience: %v", audiences))
		}
		if ann.Priority != 0 {
			annotations = append(annotations, fmt.Sprintf("  • Priority: %f", ann.Priority))
		}
		if ann.LastModified != "" {
			annotations = append(annotations, "  • Last Modified: "+ann.LastModified)
		}

		if len(annotations) > 0 {
			lines = append(lines, "Annotations:")
			lines = append(lines, annotations...)
			lines = append(lines, "")
		}
	}

	// Write output
	_, err := fmt.Fprintln(output, strings.Join(lines, "\n"))
	return err
}

// outputResourceTemplateText outputs a resource template in text format
func outputResourceTemplateText(output io.Writer, template *mcpsdk.ResourceTemplate) error {
	var lines []string

	// Title line
	titleLine := template.Title
	if titleLine == "" {
		titleLine = template.Name
	}

	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Resource template name
	lines = append(lines, "Resource template name: "+template.Name)
	lines = append(lines, "")

	// Description
	if template.Description != "" {
		lines = append(lines, "Description:")
		lines = append(lines, template.Description)
		lines = append(lines, "")
	}

	// URI Template
	lines = append(lines, "URI Template: "+template.URITemplate)
	lines = append(lines, "")

	// Optional fields
	if template.MIMEType != "" {
		lines = append(lines, "MIME Type: "+template.MIMEType)
		lines = append(lines, "")
	}

	// Annotations
	if template.Annotations != nil {
		var annotations []string
		ann := template.Annotations

		if len(ann.Audience) > 0 {
			audiences := make([]string, len(ann.Audience))
			for i, role := range ann.Audience {
				audiences[i] = string(role)
			}
			annotations = append(annotations, fmt.Sprintf("  • Audience: %v", audiences))
		}
		if ann.Priority != 0 {
			annotations = append(annotations, fmt.Sprintf("  • Priority: %f", ann.Priority))
		}
		if ann.LastModified != "" {
			annotations = append(annotations, "  • Last Modified: "+ann.LastModified)
		}

		if len(annotations) > 0 {
			lines = append(lines, "Annotations:")
			lines = append(lines, annotations...)
			lines = append(lines, "")
		}
	}

	// Write output
	_, err := fmt.Fprintln(output, strings.Join(lines, "\n"))
	return err
}

// formatSchemaType recursively formats a JSON schema type into TypeScript-style syntax
func formatSchemaType(prop *jsonschema.Schema) string {
	if prop == nil {
		return ""
	}

	// Handle union types
	if len(prop.Types) > 0 {
		var types []string
		var hasNull bool
		for _, t := range prop.Types {
			if t == "array" && prop.Items != nil {
				// Recursively format array items
				itemType := formatSchemaType(prop.Items)
				if itemType == "" {
					itemType = "any"
				}
				types = append(types, "[]"+itemType)
			} else if t == "null" {
				hasNull = true
			} else {
				types = append(types, t)
			}
		}
		// Put null type at end
		if hasNull {
			types = append(types, "null")
		}
		return strings.Join(types, ", ")
	}

	// Handle single type
	if prop.Type == "array" && prop.Items != nil {
		// Recursively format array items
		itemType := formatSchemaType(prop.Items)
		if itemType == "" {
			itemType = "any"
		}
		return "[]" + itemType
	}

	// Return the base type, or "any" if no type is specified
	if prop.Type != "" {
		return prop.Type
	}
	return "any"
}

// formatJSONSchema formats a JSON schema into a readable parameter list
func formatJSONSchema(s *jsonschema.Schema, indent int) string {
	if s == nil || len(s.Properties) == 0 {
		return ""
	}

	// Build formatted output
	indentStr := strings.Repeat("  ", indent)

	// Get property names and sort them alphabetically
	propNames := make([]string, 0, len(s.Properties))
	for propName := range s.Properties {
		propNames = append(propNames, propName)
	}
	slices.Sort(propNames)

	var lines []string
	for _, propName := range propNames {
		prop := s.Properties[propName]
		if prop == nil {
			continue
		}

		// Build property line with bullet point
		line := indentStr + "• " + propName

		// Add required marker
		if slices.Contains(s.Required, propName) {
			line += " (required)"
		}

		// Add type using recursive formatter
		if typeStr := formatSchemaType(prop); typeStr != "" && typeStr != "any" {
			line += ": " + typeStr
		}

		// Add description
		if prop.Description != "" {
			line += " - " + prop.Description
		}

		// Add default value
		if len(prop.Default) > 0 {
			line += " (default: " + string(prop.Default) + ")"
		}

		lines = append(lines, line)

		if len(prop.Properties) > 0 {
			// Handle nested objects
			nested := formatJSONSchema(prop, indent+1)
			if nested != "" {
				lines = append(lines, nested)
			}
		} else if prop.Items != nil && len(prop.Items.Properties) > 0 {
			// Handle nested arrays of objects
			nested := formatJSONSchema(prop.Items, indent+1)
			if nested != "" {
				lines = append(lines, nested)
			}
		}
	}

	return strings.Join(lines, "\n")
}

// formatPromptArguments formats prompt arguments into a readable bullet-point list
func formatPromptArguments(arguments []*mcpsdk.PromptArgument) string {
	if len(arguments) == 0 {
		return ""
	}

	// Sort arguments alphabetically by name
	sortedArgs := make([]*mcpsdk.PromptArgument, len(arguments))
	copy(sortedArgs, arguments)
	slices.SortFunc(sortedArgs, func(a, b *mcpsdk.PromptArgument) int {
		return strings.Compare(a.Name, b.Name)
	})

	var lines []string
	for _, arg := range sortedArgs {
		// Build argument line with bullet point (2-space indent to match schema formatting)
		line := "  • " + arg.Name

		// Add required marker
		if arg.Required {
			line += " (required)"
		}

		// Add description
		if arg.Description != "" {
			line += " - " + arg.Description
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// mcpGetCompletion provides custom completions for the get command
func mcpGetCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// First argument: capability type
	if len(args) == 0 {
		return filterCompletionsByPrefix(
			mcp.ValidCapabilityTypes().Strings(), toComplete,
		), cobra.ShellCompDirectiveNoFileComp
	}

	// Second argument: capability name based on type
	if len(args) == 1 {
		// Validate capability type
		capabilityType, err := mcp.ValidateCapabilityType(args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Create MCP server to get capabilities
		server, err := mcp.NewServer(cmd.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer server.Close()

		capabilities, err := server.ListCapabilities(cmd.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Close the MCP server when finished
		if err := server.Close(); err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		var names []string
		switch capabilityType {
		case mcp.CapabilityTypeTool:
			for _, tool := range capabilities.Tools {
				names = append(names, tool.Name)
			}
		case mcp.CapabilityTypePrompt:
			for _, prompt := range capabilities.Prompts {
				names = append(names, prompt.Name)
			}
		case mcp.CapabilityTypeResource:
			for _, resource := range capabilities.Resources {
				names = append(names, resource.Name)
			}
		case mcp.CapabilityTypeResourceTemplate:
			for _, template := range capabilities.ResourceTemplates {
				names = append(names, template.Name)
			}
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return filterCompletionsByPrefix(names, toComplete), cobra.ShellCompDirectiveNoFileComp
	}

	return nil, cobra.ShellCompDirectiveNoFileComp
}
