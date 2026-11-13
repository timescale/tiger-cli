package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
)

// connectDocsProxy connects to the remote docs MCP server. Returns nil if the
// docs MCP server is disabled in the config, there is no URL in the config, or
// the connection fails.
func connectDocsProxy(ctx context.Context, cfg *config.Config, logger *slog.Logger) *ProxyClient {
	if !cfg.DocsMCP || cfg.DocsMCPURL == "" {
		logger.Info("Docs MCP proxy is disabled")
		return nil
	}

	// Create timeout for establishing proxy (matches Ghost)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	proxyClient, err := NewProxyClient(ctx, cfg.DocsMCPURL, logger)
	if err != nil {
		logger.Error("Failed to connect to docs MCP server",
			slog.String("url", cfg.DocsMCPURL),
			slog.Any("error", err),
		)
		return nil
	}
	return proxyClient
}

// registerDocsProxy registers all tools, resources, resource templates, and
// prompts exposed by the remote docs MCP server. Does nothing if the proxy
// isn't connected.
func (s *Server) registerDocsProxy(ctx context.Context) {
	proxyClient := s.docsProxyClient
	if proxyClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Only list what the remote server advertises. Servers on newer protocol
	// versions reject unsupported list methods at the HTTP level, which would
	// otherwise surface as errors.
	caps := proxyClient.capabilities()

	if caps.Tools != nil {
		if err := proxyClient.RegisterTools(ctx, s.mcpServer); err != nil {
			s.logger.Error("Failed to register tools from docs MCP server",
				slog.Any("error", err),
			)
		}
	}

	if caps.Resources != nil {
		if err := proxyClient.RegisterResources(ctx, s.mcpServer); err != nil {
			s.logger.Error("Failed to register resources from docs MCP server",
				slog.Any("error", err),
			)
		}

		if err := proxyClient.RegisterResourceTemplates(ctx, s.mcpServer); err != nil {
			s.logger.Error("Failed to register resource templates from docs MCP server",
				slog.Any("error", err),
			)
		}
	}

	if caps.Prompts != nil {
		if err := proxyClient.RegisterPrompts(ctx, s.mcpServer); err != nil {
			s.logger.Error("Failed to register prompts from docs MCP server",
				slog.Any("error", err),
			)
		}
	}
}

// ProxyClient manages connection to a remote MCP server and forwards requests.
//
// It calls the remote server as a client in its own right: it negotiates its
// own protocol version and advertises no capabilities, and it never forwards
// the agent's _meta. That _meta describes the agent's own connection to Tiger
// MCP (its protocol version, identity, and capabilities), so forwarding it
// could select a protocol version the remote server doesn't speak, or promise
// features the proxy can't relay.
//
// Input requests the remote server makes anyway (elicitation and the like)
// are passed back to the agent, whose retry is forwarded with its answers: the
// proxy has nothing of its own to answer them with.
type ProxyClient struct {
	session *mcp.ClientSession
}

// NewProxyClient creates a new proxy client for the given remote server configuration
func NewProxyClient(ctx context.Context, url string, logger *slog.Logger) (*ProxyClient, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: api.HTTPClient,
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "tiger-mcp-proxy-client",
		Title:   "Tiger MCP Proxy Client",
		Version: config.Version,
	}, &mcp.ClientOptions{
		Logger: logger,
		// Without this, the SDK advertises roots support by default.
		Capabilities:   &mcp.ClientCapabilities{},
		MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true},
	})

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to remote MCP server: %w", err)
	}

	return &ProxyClient{session: session}, nil
}

// capabilities returns the capabilities the remote server advertised when the
// session was established, or empty capabilities if it advertised none.
func (p *ProxyClient) capabilities() *mcp.ServerCapabilities {
	if res := p.session.InitializeResult(); res != nil && res.Capabilities != nil {
		return res.Capabilities
	}
	return &mcp.ServerCapabilities{}
}

// RegisterTools discovers tools from remote server and registers them as proxy tools
func (p *ProxyClient) RegisterTools(ctx context.Context, server *mcp.Server) error {
	// Register each remote tool as a proxy tool, following pagination
	for tool, err := range p.session.Tools(ctx, nil) {
		if err != nil {
			return fmt.Errorf("failed to list tools from remote server: %w", err)
		}
		if tool.Name == "" {
			continue
		}

		// Create handler that forwards tool calls to remote server
		handler := p.createProxyToolHandler()

		// Register the proxy tool with our MCP server
		server.AddTool(tool, handler)
	}

	return nil
}

// createProxyToolHandler creates a handler function that forwards tool calls to the remote server
func (p *ProxyClient) createProxyToolHandler() mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Forward the request to remote server with original tool name
		params := &mcp.CallToolParams{
			Name:           req.Params.Name,
			Arguments:      req.Params.Arguments,
			InputResponses: req.Params.InputResponses,
			RequestState:   req.Params.RequestState,
		}

		// Call remote tool
		result, err := p.session.CallTool(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("remote tool call failed: %w", err)
		}

		return result, nil
	}
}

// RegisterResources discovers resources from remote server and registers them
// as proxy resources
func (p *ProxyClient) RegisterResources(ctx context.Context, server *mcp.Server) error {
	// Register each remote resource as a proxy resource, following pagination
	for resource, err := range p.session.Resources(ctx, nil) {
		if err != nil {
			return fmt.Errorf("failed to list resources from remote server: %w", err)
		}
		if resource.URI == "" {
			continue
		}

		// Create handler that forwards resource reads to remote server
		handler := p.createProxyResourceHandler()

		// Register the proxy resource with our MCP server
		server.AddResource(resource, handler)
	}

	return nil
}

// RegisterResourceTemplates discovers resource templates from remote server and registers them as proxy resource templates
func (p *ProxyClient) RegisterResourceTemplates(ctx context.Context, server *mcp.Server) error {
	// Register each remote resource template as a proxy resource template, following pagination
	for resourceTemplate, err := range p.session.ResourceTemplates(ctx, nil) {
		if err != nil {
			return fmt.Errorf("failed to list resource templates from remote server: %w", err)
		}
		if resourceTemplate.URITemplate == "" {
			continue
		}

		// Create handler that forwards resource template reads to remote server
		handler := p.createProxyResourceHandler()

		// Register the proxy resource template with our MCP server
		server.AddResourceTemplate(resourceTemplate, handler)
	}

	return nil
}

// createProxyResourceHandler creates a handler function that forwards resource reads to the remote server
func (p *ProxyClient) createProxyResourceHandler() mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		// Call remote resource
		params := *req.Params
		params.Meta = nil
		result, err := p.session.ReadResource(ctx, &params)
		if err != nil {
			return nil, fmt.Errorf("remote resource read failed: %w", err)
		}

		return result, nil
	}
}

// RegisterPrompts discovers prompts from remote server and registers them as proxy prompts
func (p *ProxyClient) RegisterPrompts(ctx context.Context, server *mcp.Server) error {
	// Register each remote prompt as a proxy prompt, following pagination
	for prompt, err := range p.session.Prompts(ctx, nil) {
		if err != nil {
			return fmt.Errorf("failed to list prompts from remote server: %w", err)
		}
		if prompt.Name == "" {
			continue
		}

		// Create handler that forwards prompt requests to remote server
		handler := p.createProxyPromptHandler()

		// Register the proxy prompt with our MCP server
		server.AddPrompt(prompt, handler)
	}

	return nil
}

// createProxyPromptHandler creates a handler function that forwards prompt requests to the remote server
func (p *ProxyClient) createProxyPromptHandler() mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		// Call remote prompt
		params := *req.Params
		params.Meta = nil
		result, err := p.session.GetPrompt(ctx, &params)
		if err != nil {
			return nil, fmt.Errorf("remote prompt request failed: %w", err)
		}

		return result, nil
	}
}

// createProxyCompletionHandler creates a handler function that forwards
// completion requests to the remote server
func (p *ProxyClient) createProxyCompletionHandler() func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	return func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
		// Call remote completion
		params := *req.Params
		params.Meta = nil
		result, err := p.session.Complete(ctx, &params)
		if err != nil {
			return nil, fmt.Errorf("remote completion failed: %w", err)
		}

		return result, nil
	}
}

// Close closes the connection to the remote MCP server. A nil ProxyClient,
// as when the proxy is disabled, has nothing to close.
func (p *ProxyClient) Close() error {
	if p == nil {
		return nil
	}
	return p.session.Close()
}
