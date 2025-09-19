package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
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
	Name      string  `json:"name"`
	Type      string  `json:"type,omitempty"`
	Region    string  `json:"region"`
	CPUMemory string  `json:"cpu_memory"`
	Replicas  *int    `json:"replicas,omitempty"`
	VpcID     *string `json:"vpc_id,omitempty"`
	Wait      *bool   `json:"wait,omitempty"`
	Timeout   *int    `json:"timeout,omitempty"`
}

func (ServiceCreateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceCreateInput](nil))

	schema.Properties["name"].Description = "Human-readable name for the service."
	schema.Properties["name"].Examples = []any{"my-production-db", "analytics-service", "user-store"}

	schema.Properties["type"].Description = "The type of database service to create. TimescaleDB includes PostgreSQL with time-series extensions."
	schema.Properties["type"].Enum = []any{"timescaledb", "postgres", "vector"}
	schema.Properties["type"].Default = util.Must(json.Marshal("timescaledb"))
	schema.Properties["type"].Examples = []any{"timescaledb"}

	schema.Properties["region"].Description = "AWS region where the service will be deployed. Choose the region closest to your users for optimal performance."
	schema.Properties["region"].Examples = []any{"us-east-1", "us-west-2", "eu-west-1", "eu-central-1", "ap-southeast-1"}

	schema.Properties["cpu_memory"].Description = "CPU and memory allocation combination. Choose from the available configurations."
	schema.Properties["cpu_memory"].Enum = util.AnySlice(util.GetAllowedCPUMemoryConfigs().Strings())

	schema.Properties["replicas"].Description = "Number of high-availability replicas for fault tolerance. Higher replica counts increase cost but improve availability."
	schema.Properties["replicas"].Default = util.Must(json.Marshal(0))
	schema.Properties["replicas"].Examples = []any{0, 1, 2}

	schema.Properties["vpc_id"].Description = "Virtual Private Cloud ID to deploy the service in. Leave empty for default networking."
	schema.Properties["vpc_id"].Examples = []any{"vpc-12345678", "vpc-abcdef123456"}

	schema.Properties["wait"].Description = "Whether to wait for the service to be fully ready before returning. Recommended for scripting."
	schema.Properties["wait"].Default = util.Must(json.Marshal(true))
	schema.Properties["wait"].Examples = []any{true, false}

	schema.Properties["timeout"].Description = "Timeout in minutes when waiting for service to be ready. Only used when 'wait' is true."
	schema.Properties["timeout"].Default = util.Must(json.Marshal(30))
	schema.Properties["timeout"].Examples = []any{15, 30, 60}

	return schema
}

// ServiceCreateOutput represents output for tiger_service_create
type ServiceCreateOutput struct {
	Service ServiceDetail `json:"service"`
	Message string        `json:"message"`
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
	Message string `json:"message"`
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

	// Set defaults
	serviceType := input.Type
	if serviceType == "" {
		serviceType = "timescaledb"
	}

	replicas := 1
	if input.Replicas != nil {
		replicas = *input.Replicas
	}

	// Parse CPU and Memory from combined string
	cpuMillis, memoryGbs, err := util.ParseCPUMemory(input.CPUMemory)
	if err != nil {
		return nil, ServiceCreateOutput{}, fmt.Errorf("invalid CPU/Memory specification: %w", err)
	}

	// Auto-generate service name if not provided
	name := input.Name
	if name == "" {
		name = fmt.Sprintf("mcp-service-%d", rand.Intn(10000))
	}

	logging.Debug("MCP: Creating service",
		zap.String("project_id", projectID),
		zap.String("name", name),
		zap.String("type", serviceType),
		zap.String("region", input.Region))

	// Prepare service creation request
	serviceCreateReq := api.ServiceCreate{
		Name:         name,
		ServiceType:  api.ServiceType(strings.ToUpper(serviceType)),
		RegionCode:   input.Region,
		ReplicaCount: replicas,
		CpuMillis:    cpuMillis,
		MemoryGbs:    float32(memoryGbs),
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

		output := ServiceCreateOutput{
			Service: s.convertToServiceDetail(api.Service(service)),
			Message: fmt.Sprintf("Service '%s' creation request accepted. Service ID: %s", name, serviceID),
		}

		// If wait is requested, wait for service to be ready
		wait := true
		if input.Wait != nil {
			wait = *input.Wait
		}

		if wait {
			timeout := 30
			if input.Timeout != nil {
				timeout = *input.Timeout
			}

			if err := s.waitForServiceReady(ctx, apiClient, projectID, serviceID, time.Duration(timeout)*time.Minute); err != nil {
				output.Message += fmt.Sprintf(". Warning: %v", err)
			} else {
				output.Message += ". Service is now ready!"
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
		return nil, ServiceUpdatePasswordOutput{
			Message: "Master password for 'tsdbadmin' user updated successfully",
		}, nil

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
