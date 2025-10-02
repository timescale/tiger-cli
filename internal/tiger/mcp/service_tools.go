package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/password"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// ServiceListInput represents input for tiger_service_list
type ServiceListInput struct{}

func (ServiceListInput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceListInput](nil))
}

// ServiceListOutput represents output for tiger_service_list
type ServiceListOutput struct {
	Services []ServiceInfo `json:"services"`
}

// ServiceInfo represents simplified service information for MCP output
type ServiceInfo struct {
	ServiceID string        `json:"id"`
	Name      string        `json:"name"`
	Status    string        `json:"status"`
	Type      string        `json:"type"`
	Region    string        `json:"region"`
	Created   string        `json:"created,omitempty"`
	Resources *ResourceInfo `json:"resources,omitempty"`
}

// ResourceInfo represents resource allocation information
type ResourceInfo struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// ServiceShowInput represents input for tiger_service_show
type ServiceShowInput struct {
	ServiceID string `json:"service_id"`
}

func (ServiceShowInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceShowInput](nil))

	schema.Properties["service_id"].Description = "The unique identifier of the service to show details for. Use tiger_service_list to find service IDs."
	schema.Properties["service_id"].Examples = []any{"fgg3zcsxw4"}

	return schema
}

// ServiceShowOutput represents output for tiger_service_show
type ServiceShowOutput struct {
	Service ServiceDetail `json:"service"`
}

// ServiceDetail represents detailed service information
type ServiceDetail struct {
	ServiceID      string        `json:"id"`
	Name           string        `json:"name"`
	Status         string        `json:"status"`
	Type           string        `json:"type"`
	Region         string        `json:"region"`
	Created        string        `json:"created,omitempty"`
	Resources      *ResourceInfo `json:"resources,omitempty"`
	Replicas       int           `json:"replicas,omitempty"`
	DirectEndpoint string        `json:"direct_endpoint,omitempty"`
	PoolerEndpoint string        `json:"pooler_endpoint,omitempty"`
	Paused         bool          `json:"paused"`
}

// ServiceCreateInput represents input for tiger_service_create
type ServiceCreateInput struct {
	Name      string   `json:"name,omitempty"`
	Addons    []string `json:"addons,omitempty"`
	Region    string   `json:"region,omitempty"`
	CPUMemory string   `json:"cpu_memory,omitempty"`
	Replicas  int      `json:"replicas,omitempty"`
	Free      bool     `json:"free,omitempty"`
	Wait      bool     `json:"wait,omitempty"`
	Timeout   int      `json:"timeout,omitempty"`
}

func (ServiceCreateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceCreateInput](nil))

	schema.Properties["name"].Description = "Human-readable name for the service (auto-generated if not provided)"
	schema.Properties["name"].MaxLength = util.Ptr(128) // Matches backend validation
	schema.Properties["name"].Examples = []any{"my-production-db", "analytics-service", "user-store"}

	schema.Properties["addons"].Description = "Array of addons to enable for the service. 'time-series' enables TimescaleDB, 'ai' enables AI/vector extensions. Use empty array for PostgreSQL-only. Defaults to time-series."
	schema.Properties["addons"].Items.Enum = []any{util.AddonTimeSeries, util.AddonAI}
	schema.Properties["addons"].UniqueItems = true

	schema.Properties["region"].Description = "AWS region where the service will be deployed. Choose the region closest to your users for optimal performance. Defaults to us-east-1."
	schema.Properties["region"].Examples = []any{"us-east-1", "us-west-2", "eu-west-1", "eu-central-1", "ap-southeast-1"}

	cpuMemoryCombinations := util.GetAllowedCPUMemoryConfigs().Strings()
	schema.Properties["cpu_memory"].Description = fmt.Sprintf("CPU and memory allocation combination. Choose from the available configurations. Defaults to %s", cpuMemoryCombinations[0])
	schema.Properties["cpu_memory"].Enum = util.AnySlice(cpuMemoryCombinations)

	schema.Properties["replicas"].Description = "Number of high-availability replicas for fault tolerance. Higher replica counts increase cost but improve availability."
	schema.Properties["replicas"].Minimum = util.Ptr(0.0)
	schema.Properties["replicas"].Maximum = util.Ptr(5.0)
	schema.Properties["replicas"].Default = util.Must(json.Marshal(0))
	schema.Properties["replicas"].Examples = []any{0, 1, 2}

	schema.Properties["free"].Description = "Create a free service. When true, addons, region, cpu_memory, and replicas cannot be specified."
	schema.Properties["free"].Default = util.Must(json.Marshal(false))
	schema.Properties["free"].Examples = []any{false, true}

	schema.Properties["wait"].Description = "Whether to wait for the service to be fully ready before returning. Set to true only if you need to use the service immediately after the tool call."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	schema.Properties["timeout"].Description = "Timeout in minutes when waiting for service to be ready. Only used when 'wait' is true."
	schema.Properties["timeout"].Minimum = util.Ptr(0.0)
	schema.Properties["timeout"].Default = util.Must(json.Marshal(30))
	schema.Properties["timeout"].Examples = []any{15, 30, 60}

	return schema
}

// ServiceCreateOutput represents output for tiger_service_create
type ServiceCreateOutput struct {
	Service         ServiceDetail                   `json:"service"`
	Message         string                          `json:"message"`
	PasswordStorage *password.PasswordStorageResult `json:"password_storage,omitempty"`
}

// ServiceUpdatePasswordInput represents input for tiger_service_update_password
type ServiceUpdatePasswordInput struct {
	ServiceID string `json:"service_id"`
	Password  string `json:"password"`
}

func (ServiceUpdatePasswordInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceUpdatePasswordInput](nil))

	schema.Properties["service_id"].Description = "The unique identifier of the service to update the password for. Use tiger_service_list to find service IDs."
	schema.Properties["service_id"].Examples = []any{"fgg3zcsxw4"}

	schema.Properties["password"].Description = "The new password for the 'tsdbadmin' user. Must be strong and secure."
	schema.Properties["password"].Examples = []any{"MySecurePassword123!"}

	return schema
}

// ServiceUpdatePasswordOutput represents output for tiger_service_update_password
type ServiceUpdatePasswordOutput struct {
	Message         string                          `json:"message"`
	PasswordStorage *password.PasswordStorageResult `json:"password_storage,omitempty"`
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
			DestructiveHint: util.Ptr(false), // Creates resources but doesn't modify existing
			IdempotentHint:  false,           // Creating with same name would fail
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
			DestructiveHint: util.Ptr(true), // Modifies authentication credentials
			IdempotentHint:  true,           // Same password can be set multiple times
			Title:           "Update Service Password",
		},
	}, s.handleServiceUpdatePassword)
}

// handleServiceList handles the tiger_service_list MCP tool
func (s *Server) handleServiceList(ctx context.Context, req *mcp.CallToolRequest, input ServiceListInput) (*mcp.CallToolResult, ServiceListOutput, error) {
	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return nil, ServiceListOutput{}, err
	}

	// Load fresh project ID from current config
	projectID, err := s.loadProjectID()
	if err != nil {
		return nil, ServiceListOutput{}, err
	}

	logging.Debug("MCP: Listing services", zap.String("project_id", projectID))

	// Make API call to list services
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := apiClient.GetProjectsProjectIdServicesWithResponse(ctx, projectID)
	if err != nil {
		return nil, ServiceListOutput{}, fmt.Errorf("failed to list services: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200:
		if resp.JSON200 == nil {
			return nil, ServiceListOutput{Services: []ServiceInfo{}}, nil
		}

		services := *resp.JSON200
		output := ServiceListOutput{
			Services: make([]ServiceInfo, len(services)),
		}

		for i, service := range services {
			output.Services[i] = s.convertToServiceInfo(service)
		}

		return nil, output, nil

	case 401:
		return nil, ServiceListOutput{}, fmt.Errorf("authentication failed: invalid API key")
	case 403:
		return nil, ServiceListOutput{}, fmt.Errorf("permission denied: insufficient access to project")
	default:
		return nil, ServiceListOutput{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}

// handleServiceShow handles the tiger_service_show MCP tool
func (s *Server) handleServiceShow(ctx context.Context, req *mcp.CallToolRequest, input ServiceShowInput) (*mcp.CallToolResult, ServiceShowOutput, error) {
	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return nil, ServiceShowOutput{}, err
	}

	// Load fresh project ID from current config
	projectID, err := s.loadProjectID()
	if err != nil {
		return nil, ServiceShowOutput{}, err
	}

	logging.Debug("MCP: Showing service details",
		zap.String("project_id", projectID),
		zap.String("service_id", input.ServiceID))

	// Make API call to get service details
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := apiClient.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceShowOutput{}, fmt.Errorf("failed to get service details: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200:
		if resp.JSON200 == nil {
			return nil, ServiceShowOutput{}, fmt.Errorf("empty response from API")
		}

		service := *resp.JSON200
		output := ServiceShowOutput{
			Service: s.convertToServiceDetail(service),
		}

		return nil, output, nil

	case 401:
		return nil, ServiceShowOutput{}, fmt.Errorf("authentication failed: invalid API key")
	case 403:
		return nil, ServiceShowOutput{}, fmt.Errorf("permission denied: insufficient access to service")
	case 404:
		return nil, ServiceShowOutput{}, fmt.Errorf("service '%s' not found in project '%s'", input.ServiceID, projectID)
	default:
		return nil, ServiceShowOutput{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}

// handleServiceCreate handles the tiger_service_create MCP tool
func (s *Server) handleServiceCreate(ctx context.Context, req *mcp.CallToolRequest, input ServiceCreateInput) (*mcp.CallToolResult, ServiceCreateOutput, error) {
	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return nil, ServiceCreateOutput{}, err
	}

	// Load fresh project ID from current config
	projectID, err := s.loadProjectID()
	if err != nil {
		return nil, ServiceCreateOutput{}, err
	}

	// Auto-generate service name if not provided
	if input.Name == "" {
		input.Name = util.GenerateServiceName()
	}

	var cpuMillis, memoryGbs int
	if input.Free {
		// Validate free service restrictions
		if input.Addons != nil {
			return nil, ServiceCreateOutput{}, fmt.Errorf("addons cannot be specified for free services")
		}
		if input.Region != "" {
			return nil, ServiceCreateOutput{}, fmt.Errorf("region cannot be specified for free services")
		}
		if input.CPUMemory != "" {
			return nil, ServiceCreateOutput{}, fmt.Errorf("cpu_memory cannot be specified for free services")
		}
		if input.Replicas != 0 {
			return nil, ServiceCreateOutput{}, fmt.Errorf("replicas cannot be specified for free services")
		}

		// Set free service defaults (in the future, we might geolocate
		// the user to the closest free region here)
		input.Region = "us-east-1"
	} else {
		// Set default addons if not provided
		if input.Addons == nil {
			input.Addons = []string{util.AddonTimeSeries}
		} else {
			// Validate addons for non-free services
			for _, addon := range input.Addons {
				if !util.IsValidAddon(addon) {
					return nil, ServiceCreateOutput{}, fmt.Errorf("invalid addon '%s'. Valid addons: %s", addon, strings.Join(util.ValidAddons(), ", "))
				}
			}
		}

		// Set default region if not provided
		if input.Region == "" {
			input.Region = "us-east-1"
		}

		// Set default CPU/Memory if not provided
		if input.CPUMemory == "" {
			config := util.GetAllowedCPUMemoryConfigs()[0] // Default to smallest config (0.5 CPU/2GB)
			cpuMillis, memoryGbs = config.CPUMillis, config.MemoryGbs
		} else {
			// Validate cpu/memory for non-free services
			cpuMillis, memoryGbs, err = util.ParseCPUMemory(input.CPUMemory)
			if err != nil {
				return nil, ServiceCreateOutput{}, fmt.Errorf("invalid CPU/Memory specification: %w", err)
			}
		}

	}

	logging.Debug("MCP: Creating service",
		zap.String("project_id", projectID),
		zap.String("name", input.Name),
		zap.Strings("addons", input.Addons),
		zap.String("region", input.Region),
		zap.Int("cpu", cpuMillis),
		zap.Int("memory", memoryGbs),
		zap.Int("replicas", input.Replicas),
		zap.Bool("free", input.Free),
	)

	// Prepare service creation request
	serviceCreateReq := api.ServiceCreate{
		Name:         input.Name,
		Addons:       util.ConvertAddonsToAPI(input.Addons),
		RegionCode:   input.Region,
		ReplicaCount: input.Replicas,
		CpuMillis:    cpuMillis,
		MemoryGbs:    memoryGbs,
	}

	// Set free flag if specified
	if input.Free {
		serviceCreateReq.Free = &input.Free
	}

	// Make API call to create service
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := apiClient.PostProjectsProjectIdServicesWithResponse(ctx, projectID, serviceCreateReq)
	if err != nil {
		return nil, ServiceCreateOutput{}, fmt.Errorf("failed to create service: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 202:
		if resp.JSON202 == nil {
			return nil, ServiceCreateOutput{}, fmt.Errorf("service creation request accepted but no response data received")
		}

		service := *resp.JSON202
		serviceID := util.Deref(service.ServiceId)
		serviceStatus := util.DerefStr(service.Status)

		// Capture initial password from creation response and save it immediately
		var initialPassword string
		if service.InitialPassword != nil {
			initialPassword = *service.InitialPassword
		}

		output := ServiceCreateOutput{
			Service: s.convertToServiceDetail(service),
			Message: "Service creation request accepted. The service may still be provisioning.",
		}

		// Save password immediately after service creation, before any waiting
		// This ensures the password is stored even if the wait fails or is interrupted
		if initialPassword != "" {
			result, err := password.SavePasswordWithResult(api.Service(service), initialPassword)
			output.PasswordStorage = &result
			if err != nil {
				logging.Debug("MCP: Password storage failed", zap.Error(err))
			} else {
				logging.Debug("MCP: Password saved successfully", zap.String("method", result.Method))
			}
		}

		// If wait is explicitly requested, wait for service to be ready
		if input.Wait {
			timeout := time.Duration(input.Timeout) * time.Minute

			output.Service, err = s.waitForServiceReady(apiClient, projectID, serviceID, timeout, serviceStatus)
			if err != nil {
				output.Message = fmt.Sprintf("Error: %s", err.Error())
			} else {
				output.Message = "Service created successfully and is ready!"
			}
		}

		return nil, output, nil
	case 400:
		return nil, ServiceCreateOutput{}, fmt.Errorf("invalid request parameters")
	case 401:
		return nil, ServiceCreateOutput{}, fmt.Errorf("authentication failed: invalid API key")
	case 403:
		return nil, ServiceCreateOutput{}, fmt.Errorf("permission denied: insufficient access to create services")
	default:
		return nil, ServiceCreateOutput{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}

// handleServiceUpdatePassword handles the tiger_service_update_password MCP tool
func (s *Server) handleServiceUpdatePassword(ctx context.Context, req *mcp.CallToolRequest, input ServiceUpdatePasswordInput) (*mcp.CallToolResult, ServiceUpdatePasswordOutput, error) {
	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	// Load fresh project ID from current config
	projectID, err := s.loadProjectID()
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	logging.Debug("MCP: Updating service password",
		zap.String("project_id", projectID),
		zap.String("service_id", input.ServiceID))

	// Prepare password update request
	updateReq := api.UpdatePasswordInput{
		Password: input.Password,
	}

	// Make API call to update password
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := apiClient.PostProjectsProjectIdServicesServiceIdUpdatePasswordWithResponse(ctx, projectID, input.ServiceID, updateReq)
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("failed to update service password: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200, 204:
		output := ServiceUpdatePasswordOutput{
			Message: "Master password for 'tsdbadmin' user updated successfully",
		}

		// Get service details for password storage (similar to CLI implementation)
		serviceResp, err := apiClient.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, input.ServiceID)
		if err == nil && serviceResp.StatusCode() == 200 && serviceResp.JSON200 != nil {
			// Save the new password using the shared util function
			result, err := password.SavePasswordWithResult(api.Service(*serviceResp.JSON200), input.Password)
			output.PasswordStorage = &result
			if err != nil {
				logging.Debug("MCP: Password storage failed", zap.Error(err))
			} else {
				logging.Debug("MCP: Password saved successfully", zap.String("method", result.Method))
			}
		}

		return nil, output, nil

	case 401:
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("authentication failed: invalid API key")
	case 403:
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("permission denied: insufficient access to update service password")
	case 404:
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("service '%s' not found in project '%s'", input.ServiceID, projectID)
	case 400:
		var errorMsg string
		if resp.JSON400 != nil && resp.JSON400.Message != nil {
			errorMsg = *resp.JSON400.Message
		} else {
			errorMsg = "Invalid password"
		}
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("invalid password: %s", errorMsg)

	default:
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}
