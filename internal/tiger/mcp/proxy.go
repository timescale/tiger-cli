package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/logging"
)

// ProxyConfig holds configuration for connecting to a remote MCP server
type ProxyConfig struct {
	// URL for HTTP-based MCP servers (required)
	URL string `json:"url"`

	// Whether this proxy is enabled
	Enabled bool `json:"enabled"`
}

// ProxyClient manages connection to a remote MCP server and forwards requests
type ProxyClient struct {
	config  *ProxyConfig
	client  *mcp.Client
	session *mcp.ClientSession
	server  *Server
}

// NewProxyClient creates a new proxy client for the given remote server configuration
func NewProxyClient(config *ProxyConfig, server *Server) *ProxyClient {
	return &ProxyClient{
		config: config,
		server: server,
	}
}

// Connect establishes connection to the remote MCP server
func (p *ProxyClient) Connect(ctx context.Context) error {
	if !p.config.Enabled {
		return fmt.Errorf("proxy is disabled")
	}

	logging.Debug("Connecting to remote MCP server",
		zap.String("url", p.config.URL))

	// Create HTTP transport for MCP server
	if p.config.URL == "" {
		return fmt.Errorf("URL must be specified for docs MCP proxy connection")
	}

	// Use streamable HTTP transport (preferred for HTTP-based MCP servers)
	transport := &mcp.StreamableClientTransport{
		Endpoint: p.config.URL,
	}

	// Create client and connect
	p.client = mcp.NewClient(&mcp.Implementation{
		Name:    "tiger-mcp-proxy",
		Title:   "Tiger MCP Proxy Client",
		Version: "1.0.0",
	}, nil)

	// Connect to remote server
	session, err := p.client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to remote MCP server: %w", err)
	}

	p.session = session
	logging.Info("Successfully connected to remote MCP server")

	return nil
}

// DiscoverAndRegisterTools discovers tools from remote server and registers them as proxy tools
func (p *ProxyClient) DiscoverAndRegisterTools(ctx context.Context) error {
	if p.session == nil {
		return fmt.Errorf("not connected to remote server")
	}

	logging.Debug("Discovering tools from remote MCP server")

	// List tools from remote server
	toolsResp, err := p.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return fmt.Errorf("failed to list tools from remote server: %w", err)
	}

	if toolsResp == nil || len(toolsResp.Tools) == 0 {
		logging.Debug("No tools found on remote server")
		return nil
	}

	// Register each remote tool as a proxy tool
	for _, tool := range toolsResp.Tools {
		if tool.Name == "" {
			logging.Warn("Skipping tool with empty name")
			continue
		}

		// Create handler that forwards tool calls to remote server
		handler := p.createToolProxyHandler(tool.Name)

		// Register the proxy tool with our MCP server
		p.server.mcpServer.AddTool(tool, handler)

		logging.Debug("Registered proxy tool",
			zap.String("name", tool.Name),
		)
	}

	logging.Info("Successfully registered proxy tools", zap.Int("count", len(toolsResp.Tools)))
	return nil
}

// createToolProxyHandler creates a handler function that forwards tool calls to the remote server
func (p *ProxyClient) createToolProxyHandler(remoteToolName string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logging.Debug("Proxying tool call to remote server",
			zap.String("tool_name", remoteToolName))

		if p.session == nil {
			return nil, fmt.Errorf("not connected to remote MCP server")
		}

		// Extract arguments from the request
		var arguments map[string]any
		if req.Params != nil && req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &arguments); err != nil {
				logging.Error("Failed to unmarshal tool arguments",
					zap.String("tool_name", remoteToolName),
					zap.Error(err))
				return nil, fmt.Errorf("failed to unmarshal tool arguments: %w", err)
			}
		}

		// Forward the request to remote server with original tool name
		params := &mcp.CallToolParams{
			Name:      remoteToolName, // Use original tool name for remote server
			Arguments: arguments,
		}

		// Call remote tool
		result, err := p.session.CallTool(ctx, params)
		if err != nil {
			logging.Error("Remote tool call failed",
				zap.String("tool_name", remoteToolName),
				zap.Error(err))
			return nil, fmt.Errorf("remote tool call failed: %w", err)
		}

		logging.Debug("Remote tool call successful",
			zap.String("tool_name", remoteToolName))

		return result, nil
	}
}

// DiscoverAndRegisterResources discovers resources from remote server and registers them as proxy resources
func (p *ProxyClient) DiscoverAndRegisterResources(ctx context.Context) error {
	if p.session == nil {
		return fmt.Errorf("not connected to remote server")
	}

	logging.Debug("Discovering resources from remote MCP server")

	// List resources from remote server
	resourcesResp, err := p.session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		return fmt.Errorf("failed to list resources from remote server: %w", err)
	}

	if resourcesResp == nil || len(resourcesResp.Resources) == 0 {
		logging.Debug("No resources found on remote server")
		return nil
	}

	// Register each remote resource as a proxy resource
	for _, resource := range resourcesResp.Resources {
		if resource.URI == "" {
			logging.Warn("Skipping resource with empty URI")
			continue
		}

		// Create handler that forwards resource reads to remote server
		handler := p.createProxyResourceHandler(resource.URI)

		// Register the proxy resource with our MCP server
		p.server.mcpServer.AddResource(resource, handler)

		logging.Debug("Registered proxy resource",
			zap.String("uri", resource.URI),
		)
	}

	logging.Info("Successfully registered proxy resources", zap.Int("count", len(resourcesResp.Resources)))
	return nil
}

// createProxyResourceHandler creates a handler function that forwards resource reads to the remote server
func (p *ProxyClient) createProxyResourceHandler(remoteURI string) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		logging.Debug("Proxying resource read to remote server",
			zap.String("resource_uri", remoteURI))

		if p.session == nil {
			return nil, fmt.Errorf("not connected to remote MCP server")
		}

		// Forward the request to remote server with original URI
		readParams := &mcp.ReadResourceParams{
			URI: remoteURI, // Use original URI for remote server
		}

		// Call remote resource
		result, err := p.session.ReadResource(ctx, readParams)
		if err != nil {
			logging.Error("Remote resource read failed",
				zap.String("resource_uri", remoteURI),
				zap.Error(err))
			return nil, fmt.Errorf("remote resource read failed: %w", err)
		}

		logging.Debug("Remote resource read successful",
			zap.String("resource_uri", remoteURI))

		return result, nil
	}
}

// Close closes the connection to the remote MCP server
func (p *ProxyClient) Close() error {
	if p.session != nil {
		logging.Debug("Closing connection to remote MCP server")
		return p.session.Close()
	}
	return nil
}

// DiscoverAndRegisterResourceTemplates discovers resource templates from remote server and registers them as proxy resource templates
func (p *ProxyClient) DiscoverAndRegisterResourceTemplates(ctx context.Context) error {
	if p.session == nil {
		return fmt.Errorf("not connected to remote server")
	}

	logging.Debug("Discovering resource templates from remote MCP server")

	// List resource templates from remote server
	templatesResp, err := p.session.ListResourceTemplates(ctx, &mcp.ListResourceTemplatesParams{})
	if err != nil {
		return fmt.Errorf("failed to list resource templates from remote server: %w", err)
	}

	if templatesResp == nil || len(templatesResp.ResourceTemplates) == 0 {
		logging.Debug("No resource templates found on remote server")
		return nil
	}

	// Register each remote resource template as a proxy resource template
	for _, resourceTemplate := range templatesResp.ResourceTemplates {
		if resourceTemplate.URITemplate == "" {
			logging.Warn("Skipping resource template with empty URI template")
			continue
		}

		// Create handler that forwards resource template reads to remote server
		handler := p.createProxyResourceHandler(resourceTemplate.URITemplate)

		// Register the proxy resource template with our MCP server
		p.server.mcpServer.AddResourceTemplate(resourceTemplate, handler)

		logging.Debug("Registered proxy resource template",
			zap.String("uri_template", resourceTemplate.URITemplate),
		)
	}

	logging.Info("Successfully discovered proxy resource templates", zap.Int("count", len(templatesResp.ResourceTemplates)))
	return nil
}

// DiscoverAndRegisterPrompts discovers prompts from remote server and registers them as proxy prompts
func (p *ProxyClient) DiscoverAndRegisterPrompts(ctx context.Context) error {
	if p.session == nil {
		return fmt.Errorf("not connected to remote server")
	}

	logging.Debug("Discovering prompts from remote MCP server")

	// List prompts from remote server
	promptsResp, err := p.session.ListPrompts(ctx, &mcp.ListPromptsParams{})
	if err != nil {
		return fmt.Errorf("failed to list prompts from remote server: %w", err)
	}

	if promptsResp == nil || len(promptsResp.Prompts) == 0 {
		logging.Debug("No prompts found on remote server")
		return nil
	}

	// Register each remote prompt as a proxy prompt
	for _, prompt := range promptsResp.Prompts {
		if prompt.Name == "" {
			logging.Warn("Skipping prompt with empty name")
			continue
		}

		// Create handler that forwards prompt requests to remote server
		handler := p.createProxyPromptHandler(prompt.Name)

		// Register the proxy prompt with our MCP server
		p.server.mcpServer.AddPrompt(prompt, handler)

		logging.Debug("Registered proxy prompt",
			zap.String("original_name", prompt.Name),
		)
	}

	logging.Info("Successfully discovered proxy prompts", zap.Int("count", len(promptsResp.Prompts)))
	return nil
}

// createProxyPromptHandler creates a handler function that forwards prompt requests to the remote server
func (p *ProxyClient) createProxyPromptHandler(remotePromptName string) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		logging.Debug("Proxying prompt request to remote server",
			zap.String("prompt_name", remotePromptName))

		if p.session == nil {
			return nil, fmt.Errorf("not connected to remote MCP server")
		}

		// Extract arguments from the request
		var arguments map[string]string
		if req.Params != nil && req.Params.Arguments != nil {
			arguments = req.Params.Arguments
		}

		// Forward the request to remote server with original prompt name
		getParams := &mcp.GetPromptParams{
			Name:      remotePromptName, // Use original prompt name for remote server
			Arguments: arguments,
		}

		// Call remote prompt
		result, err := p.session.GetPrompt(ctx, getParams)
		if err != nil {
			logging.Error("Remote prompt request failed",
				zap.String("prompt_name", remotePromptName),
				zap.Error(err))
			return nil, fmt.Errorf("remote prompt request failed: %w", err)
		}

		logging.Debug("Remote prompt request successful",
			zap.String("prompt_name", remotePromptName))

		return result, nil
	}
}
