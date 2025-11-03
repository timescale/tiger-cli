package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/analytics"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
)

const (
	ServerName  = "tiger"
	serverTitle = "Tiger MCP"
)

// Server wraps the MCP server with Tiger-specific functionality
type Server struct {
	mcpServer       *mcp.Server
	docsProxyClient *ProxyClient
}

// NewServer creates a new Tiger MCP server instance
func NewServer(ctx context.Context) (*Server, error) {
	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Title:   serverTitle,
		Version: config.Version,
	}, nil)

	server := &Server{
		mcpServer: mcpServer,
	}

	// Register all tools (including proxied docs tools)
	server.registerTools(ctx)

	// Add analytics tracking middleware
	server.mcpServer.AddReceivingMiddleware(server.analyticsMiddleware)

	return server, nil
}

// Run starts the MCP server with the stdio transport
func (s *Server) StartStdio(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

// Returns an HTTP handler that implements the http transport
func (s *Server) HTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return s.mcpServer
	}, nil)
}

// registerTools registers all available MCP tools
func (s *Server) registerTools(ctx context.Context) {
	// Service management tools
	s.registerServiceTools()

	// Database operation tools
	s.registerDatabaseTools()

	// TODO: Register more tool groups

	// Register remote docs MCP server proxy
	s.registerDocsProxy(ctx)

	logging.Info("MCP tools registered successfully")
}

// createAPIClient creates a new API client and returns it with the project ID
func (s *Server) createAPIClient() (*api.ClientWithResponses, string, error) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return nil, "", fmt.Errorf("failed to load config: %w", err)
	}

	// Get credentials (API key + project ID)
	apiKey, projectID, err := config.GetCredentials()
	if err != nil {
		return nil, "", fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err)
	}

	// Create API client with fresh credentials
	apiClient, err := api.NewTigerClient(cfg, apiKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create API client: %w", err)
	}

	return apiClient, projectID, nil
}

// analyticsMiddleware tracks analytics for all MCP requests
func (s *Server) analyticsMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, runErr error) {
		// Load config for analytics
		cfg, err := config.Load()
		if err != nil {
			// If we can't load config, just skip analytics and continue
			return next(ctx, method, req)
		}

		a := analytics.TryInit(cfg)

		switch r := req.(type) {
		case *mcp.CallToolRequest:
			// Extract arguments from the tool call
			var args map[string]any
			if len(r.Params.Arguments) > 0 {
				if err := json.Unmarshal(r.Params.Arguments, &args); err != nil {
					logging.Error("Error unmarshaling tool call arguments", zap.Error(err))
				}
			}

			defer func() {
				a.Track(fmt.Sprintf("Call %s tool", r.Params.Name),
					analytics.Map(args, "password", "query", "parameters"),
					analytics.Error(runErr),
				)
			}()
		case *mcp.ReadResourceRequest:
			defer func() {
				a.Track("Read proxied resource",
					analytics.Property("resource_uri", r.Params.URI),
					analytics.Error(runErr),
				)
			}()
		case *mcp.GetPromptRequest:
			defer func() {
				a.Track(fmt.Sprintf("Get %s prompt", r.Params.Name),
					analytics.Error(runErr),
				)
			}()
		}

		// Execute the actual handler
		return next(ctx, method, req)
	}
}

// Close gracefully shuts down the MCP server and all proxy connections
func (s *Server) Close() error {
	logging.Debug("Closing MCP server and proxy connections")

	// Close docs proxy connection
	if err := s.docsProxyClient.Close(); err != nil {
		return fmt.Errorf("failed to close docs proxy client: %w", err)
	}

	return nil
}
