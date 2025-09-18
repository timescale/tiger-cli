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
	// TODO: Is this right?
	serverName  = "tiger-mcp"
	serverTitle = "Tiger MCP"
)

// Server wraps the MCP server with Tiger-specific functionality
type Server struct {
	mcpServer *mcp.Server
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
		mcpServer: mcpServer,
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

	// TODO: Register more tool groups

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
