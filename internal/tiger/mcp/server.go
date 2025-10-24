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

// initToolCall loads the config and credentials, creates a new API client, and
// returns the config, client, and projectID.
func (s *Server) initToolCall() (*config.Config, *api.ClientWithResponses, string, error) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to load config: %w", err)
	}

	// Get credentials (API key + project ID)
	apiKey, projectID, err := config.GetCredentials()
	if err != nil {
		return nil, nil, "", fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", err)
	}

	// Create API client with fresh credentials
	apiClient, err := api.NewTigerClient(cfg, apiKey)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to create API client: %w", err)
	}

	return cfg, apiClient, projectID, nil
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
