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
	ServerName    = "tiger"  // TODO: Is this right?
	ServerVersion = "v1.0.0" // TODO: Use same version as CLI?
)

// Server wraps the MCP server with Tiger-specific functionality
type Server struct {
	mcpServer *mcp.Server
	apiClient *api.ClientWithResponses
	config    *config.Config
}

// NewServer creates a new Tiger MCP server instance
func NewServer() (*Server, error) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Note: We don't get API key here since it requires importing cmd package
	// API key will be retrieved in the individual tool handlers
	var apiClient *api.ClientWithResponses

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: ServerVersion,
	}, nil)

	server := &Server{
		mcpServer: mcpServer,
		apiClient: apiClient,
		config:    cfg,
	}

	// Register all tools
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

	logging.Info("MCP tools registered successfully")
}

// registerServiceTools registers service management tools
func (s *Server) registerServiceTools() {
	// tiger_service_list
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "tiger_service_list",
		Description: "List all database services in the current project",
	}, s.handleServiceList)

	// tiger_service_show
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "tiger_service_show",
		Description: "Show detailed information about a specific service",
	}, s.handleServiceShow)

	// tiger_service_create
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "tiger_service_create",
		Description: "Create a new database service",
	}, s.handleServiceCreate)

	// tiger_service_update_password
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "tiger_service_update_password",
		Description: "Update the master password for a service",
	}, s.handleServiceUpdatePassword)
}

// ensureAuthenticated checks if we can get an API key and creates a client if needed
func (s *Server) ensureAuthenticated() error {
	if s.apiClient != nil {
		return nil
	}

	// Try to get API key and create client
	apiKey, err := config.GetAPIKey()
	if err != nil {
		return fmt.Errorf("authentication required: %w", err)
	}

	// Create API client
	s.apiClient, err = api.NewTigerClient(apiKey)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	return nil
}

// ensureProjectID checks if we have a project ID configured
func (s *Server) ensureProjectID() (string, error) {
	if s.config.ProjectID == "" {
		return "", fmt.Errorf("project ID is required. Please run 'tiger auth login' with --project-id")
	}
	return s.config.ProjectID, nil
}
