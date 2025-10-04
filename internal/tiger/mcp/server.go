package mcp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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

// createAPIClient loads fresh config and creates a new API client for each tool call
func (s *Server) createAPIClient() (*api.ClientWithResponses, error) {
	// Get fresh API key
	apiKey, err := config.GetAPIKey()
	if err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Create API client with fresh credentials
	apiClient, err := api.NewTigerClient(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return apiClient, nil
}

// loadConfigWithProjectID loads fresh config and validates that project ID is set
func (s *Server) loadConfigWithProjectID() (*config.Config, error) {
	// Load fresh config
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required. Please run 'tiger auth login'")
	}
	return cfg, nil
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
