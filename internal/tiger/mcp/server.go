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

// registerServiceTools registers service management tools with comprehensive schemas and descriptions
func (s *Server) registerServiceTools() {
	// tiger_service_list
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "tiger_service_list",
		Title: "List Database Services",
		Description: `List all database services in your current TigerData project.

This tool retrieves a complete list of database services with their basic information including status, type, region, and resource allocation. Use this to get an overview of all your services before performing operations on specific services.

Perfect for:
- Getting an overview of your database infrastructure
- Finding service IDs for other operations
- Checking service status and resource allocation
- Discovering services across different regions`,
		InputSchema: ServiceListInput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
			Title:        "List Database Services",
		},
	}, s.handleServiceList)

	// tiger_service_show
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "tiger_service_show",
		Title: "Show Service Details",
		Description: `Show detailed information about a specific database service.

This tool provides comprehensive information about a service including connection endpoints, replica configuration, resource allocation, creation time, and current operational status. Essential for debugging, monitoring, and connection management.

Perfect for:
- Getting connection endpoints (direct and pooled)
- Checking detailed service configuration
- Monitoring service health and status
- Obtaining service specifications for scaling decisions`,
		InputSchema: ServiceShowInput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
			Title:        "Show Service Details",
		},
	}, s.handleServiceShow)

	// tiger_service_create
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "tiger_service_create",
		Title: "Create Database Service",
		Description: `Create a new database service in TigerData Cloud.

This tool provisions a new database service with specified configuration including service type, compute resources, region, and high availability options. By default, the tool returns immediately after the creation request is accepted, but the service may still be provisioning and not ready for connections yet.

Only set 'wait: true' if you need the service to be fully ready immediately after the tool call returns. In most cases, leave wait as false (default) for faster responses.

IMPORTANT: This operation incurs costs and creates billable resources. Always confirm requirements before proceeding.

Perfect for:
- Setting up new database infrastructure
- Creating development or production environments
- Provisioning databases with specific resource requirements
- Establishing services in different geographical regions`,
		InputSchema: ServiceCreateInput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(false), // Creates resources but doesn't modify existing
			IdempotentHint:  false,      // Creating with same name would fail
			Title:           "Create Database Service",
		},
	}, s.handleServiceCreate)

	// tiger_service_update_password
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "tiger_service_update_password",
		Title: "Update Service Password",
		Description: `Update the master password for the 'tsdbadmin' user of a database service.

This tool changes the master database password used for the default administrative user. The new password will be required for all future database connections. Existing connections may be terminated.

SECURITY NOTE: Ensure new passwords are strong and stored securely. Password changes take effect immediately.

Perfect for:
- Password rotation for security compliance
- Recovering from compromised credentials
- Setting initial passwords for new services
- Meeting organizational security policies`,
		InputSchema: ServiceUpdatePasswordInput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(true), // Modifies authentication credentials
			IdempotentHint:  true,      // Same password can be set multiple times
			Title:           "Update Service Password",
		},
	}, s.handleServiceUpdatePassword)
}

// Helper function to create pointer to int for schema validation
func ptr[T any](v T) *T {
	return &v
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
