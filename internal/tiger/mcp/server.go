package mcp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
)

const (
	// TODO: Is this right?
	serverName  = "tiger-mcp"
	serverTitle = "Tiger MCP"
)

// Server wraps the MCP server with Tiger-specific functionality
type Server struct {
	mcpServer    *mcp.Server
	proxyClients []*ProxyClient
}

// NewServer creates a new Tiger MCP server instance
func NewServer() (*Server, error) {
	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Title:   serverTitle,
		Version: config.Version,
	}, nil)

	server := &Server{
		mcpServer:    mcpServer,
		proxyClients: make([]*ProxyClient, 0),
	}

	// Register all tools (including proxy tools)
	server.registerTools()

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
func (s *Server) registerTools() {
	// Service management tools (v0 priority)
	s.registerServiceTools()

	// Setup proxy connections to remote MCP servers
	s.setupDocsProxyConnection()

	// TODO: Register more tool groups

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

// loadProjectID loads fresh config and returns the current project ID
func (s *Server) loadProjectID() (string, error) {
	// Load fresh config
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.ProjectID == "" {
		return "", fmt.Errorf("project ID is required. Please run 'tiger auth login' with --project-id")
	}
	return cfg.ProjectID, nil
}

// setupDocsProxyConnection loads proxy configuration and establishes a connection
// to the remote docs MCP server
func (s *Server) setupDocsProxyConnection() {
	// Load proxy configuration from config
	proxyConfig, err := s.loadDocsProxyConfig()
	if err != nil {
		logging.Debug("No proxy configuration found or failed to load", zap.Error(err))
		return
	}

	if !proxyConfig.Enabled {
		logging.Debug("Docs MCP proxy is disabled")
		return
	}

	logging.Info("Setting up docs MCP proxy connection",
		zap.String("url", proxyConfig.URL),
	)

	// Set up docs MCP proxy
	proxyClient := NewProxyClient(proxyConfig, s)

	// Create timeout for establishing proxy
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Connect to remote server
	if err := proxyClient.Connect(ctx); err != nil {
		logging.Error("Failed to connect to docs MCP server",
			zap.String("url", proxyConfig.URL),
			zap.Error(err))
		cancel()
		return
	}

	// Discover and register tools from docs MCP server
	if err := proxyClient.DiscoverAndRegisterTools(ctx); err != nil {
		logging.Error("Failed to register tools from docs MCP server", zap.Error(err))
	}

	// Discover and register resources from docs MCP server
	if err := proxyClient.DiscoverAndRegisterResources(ctx); err != nil {
		logging.Error("Failed to register resources from docs MCP server", zap.Error(err))
	}

	// Discover and register resource templates from docs MCP server
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	if err := proxyClient.DiscoverAndRegisterResourceTemplates(ctx); err != nil {
		logging.Error("Failed to register resource templates from docs MCP server", zap.Error(err))
	}

	// Discover and register prompts from docs MCP server
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	if err := proxyClient.DiscoverAndRegisterPrompts(ctx); err != nil {
		logging.Error("Failed to register prompts from docs MCP server", zap.Error(err))
	}

	// Add to proxy clients list for cleanup
	s.proxyClients = append(s.proxyClients, proxyClient)

	logging.Info("Successfully connected to docs MCP server",
		zap.String("url", proxyConfig.URL),
	)
}

// loadDocsProxyConfig loads proxy configuration from the Tiger CLI config
func (s *Server) loadDocsProxyConfig() (*ProxyConfig, error) {
	// Load Tiger CLI configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Create docs MCP proxy configuration
	return &ProxyConfig{
		URL:     cfg.DocsMCPURL,
		Enabled: cfg.DocsMCPEnabled,
	}, nil
}

// Close gracefully shuts down the MCP server and all proxy connections
func (s *Server) Close() error {
	logging.Debug("Closing MCP server and proxy connections")

	// Close all proxy connections
	for i, proxyClient := range s.proxyClients {
		if err := proxyClient.Close(); err != nil {
			logging.Error("Failed to close proxy client",
				zap.Int("proxy_index", i),
				zap.Error(err))
		}
	}

	return nil
}
