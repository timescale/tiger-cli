package mcp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
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
		InputSchema: &jsonschema.Schema{
			Type:        "object",
			Title:       "Service List Parameters",
			Description: "No parameters required - lists all services in the current project",
			Properties:  map[string]*jsonschema.Schema{},
		},
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
		InputSchema: &jsonschema.Schema{
			Type:        "object",
			Title:       "Service Show Parameters",
			Description: "Parameters to show detailed information about a specific service",
			Properties: map[string]*jsonschema.Schema{
				"service_id": {
					Type:        "string",
					Title:       "Service ID",
					Description: "The unique identifier of the service to show details for. Use tiger_service_list to find service IDs.",
					Examples:    []any{"fgg3zcsxw4"},
				},
			},
			Required: []string{"service_id"},
		},
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

This tool provisions a new database service with specified configuration including service type, compute resources, region, and high availability options. The service creation process may take several minutes to complete.

IMPORTANT: This operation incurs costs and creates billable resources. Always confirm requirements before proceeding.

Perfect for:
- Setting up new database infrastructure
- Creating development or production environments
- Provisioning databases with specific resource requirements
- Establishing services in different geographical regions`,
		InputSchema: &jsonschema.Schema{
			Type:        "object",
			Title:       "Service Creation Parameters",
			Description: "Complete configuration for creating a new database service",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Title:       "Service Name",
					Description: "Human-readable name for the service. Must be unique within the project.",
					Examples:    []any{"my-production-db", "analytics-service", "user-store"},
				},
				"type": {
					Type:        "string",
					Title:       "Service Type",
					Description: "The type of database service to create. TimescaleDB includes PostgreSQL with time-series extensions.",
					Enum:        []any{"timescaledb", "postgres", "vector"},
					Default:     toRawMessage("timescaledb"),
					Examples:    []any{"timescaledb"},
				},
				"region": {
					Type:        "string",
					Title:       "Region Code",
					Description: "AWS region where the service will be deployed. Choose the region closest to your users for optimal performance.",
					Examples:    []any{"us-east-1", "us-west-2", "eu-west-1", "eu-central-1", "ap-southeast-1"},
				},
				"cpu": {
					Type:        "string",
					Title:       "CPU Allocation",
					Description: "CPU allocation in cores or millicores. Examples: '1' (1 core), '2000m' (2000 millicores = 2 cores), '0.5' (0.5 cores).",
					Examples:    []any{"0.5", "1", "2", "4", "8", "16", "32", "500m", "1000m", "2000m"},
				},
				"memory": {
					Type:        "string",
					Title:       "Memory Allocation",
					Description: "Memory allocation with units. Supported units: GB, MB. Example: '8GB', '4096MB'.",
					Examples:    []any{"2GB", "4GB", "8GB", "16GB", "32GB", "64GB", "128GB"},
				},
				"replicas": {
					Type:        "integer",
					Title:       "High Availability Replicas",
					Description: "Number of high-availability replicas for fault tolerance. Higher replica counts increase cost but improve availability.",
					Default:     toRawMessage(1),
					Examples:    []any{1, 2, 3},
				},
				"vpc_id": {
					Type:        "string",
					Title:       "VPC ID (Optional)",
					Description: "Virtual Private Cloud ID to deploy the service in. Leave empty for default networking.",
					Examples:    []any{"vpc-12345678", "vpc-abcdef123456"},
				},
				"wait": {
					Type:        "boolean",
					Title:       "Wait for Ready",
					Description: "Whether to wait for the service to be fully ready before returning. Recommended for scripting.",
					Default:     toRawMessage(true),
					Examples:    []any{true, false},
				},
				"timeout": {
					Type:        "integer",
					Title:       "Wait Timeout (Minutes)",
					Description: "Timeout in minutes when waiting for service to be ready. Only used when 'wait' is true.",
					Default:     toRawMessage(30),
					Examples:    []any{15, 30, 60},
				},
			},
			Required: []string{"name", "region", "cpu", "memory"},
		},
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
		InputSchema: &jsonschema.Schema{
			Type:        "object",
			Title:       "Password Update Parameters",
			Description: "Service identification and new password",
			Properties: map[string]*jsonschema.Schema{
				"service_id": {
					Type:        "string",
					Title:       "Service ID",
					Description: "The unique identifier of the service to update the password for. Use tiger_service_list to find service IDs.",
					Examples:    []any{"fgg3zcsxw4"},
				},
				"password": {
					Type:        "string",
					Title:       "New Password",
					Description: "The new password for the 'tsdbadmin' user. Must be strong and secure.",
					Examples:    []any{"MySecurePassword123!"},
				},
			},
			Required: []string{"service_id", "password"},
		},
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: ptr(true), // Modifies authentication credentials
			IdempotentHint:  true,      // Same password can be set multiple times
			Title:           "Update Service Password",
		},
	}, s.handleServiceUpdatePassword)
}

// Helper function to convert values to json.RawMessage for schema defaults
func toRawMessage(v any) []byte {
	switch val := v.(type) {
	case string:
		return []byte(`"` + val + `"`)
	case int:
		return []byte(fmt.Sprintf("%d", val))
	case bool:
		if val {
			return []byte("true")
		}
		return []byte("false")
	default:
		return []byte("null")
	}
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
