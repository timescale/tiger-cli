package mcp

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// ServiceListInput represents input for tiger_service_list
type ServiceListInput struct{}

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
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Region   string  `json:"region"`
	CPU      string  `json:"cpu"`
	Memory   string  `json:"memory"`
	Replicas *int    `json:"replicas,omitempty"`
	VpcID    *string `json:"vpc_id,omitempty"`
	Wait     *bool   `json:"wait,omitempty"`
	Timeout  *int    `json:"timeout,omitempty"`
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

// ServiceUpdatePasswordOutput represents output for tiger_service_update_password
type ServiceUpdatePasswordOutput struct {
	Message string `json:"message"`
}

// createErrorResult is a helper function to create error responses
func createErrorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{
			Text: message,
		}},
	}
}

// handleServiceList handles the tiger_service_list MCP tool
func (s *Server) handleServiceList(ctx context.Context, req *mcp.CallToolRequest, input ServiceListInput) (*mcp.CallToolResult, ServiceListOutput, error) {
	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return createErrorResult(err.Error()), ServiceListOutput{}, nil
	}

	// Load fresh project ID from current config
	projectID, err := s.loadProjectID()
	if err != nil {
		return createErrorResult(err.Error()), ServiceListOutput{}, nil
	}

	logging.Debug("MCP: Listing services", zap.String("project_id", projectID))

	// Make API call to list services
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := apiClient.GetProjectsProjectIdServicesWithResponse(ctx, projectID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Failed to list services: %v", err)), ServiceListOutput{}, nil
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
		return createErrorResult("Authentication failed: invalid API key"), ServiceListOutput{}, nil
	case 403:
		return createErrorResult("Permission denied: insufficient access to project"), ServiceListOutput{}, nil
	default:
		return createErrorResult(fmt.Sprintf("API request failed with status %d", resp.StatusCode())), ServiceListOutput{}, nil
	}
}

// handleServiceShow handles the tiger_service_show MCP tool
func (s *Server) handleServiceShow(ctx context.Context, req *mcp.CallToolRequest, input ServiceShowInput) (*mcp.CallToolResult, ServiceShowOutput, error) {
	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return createErrorResult(err.Error()), ServiceShowOutput{}, nil
	}

	// Load fresh project ID from current config
	projectID, err := s.loadProjectID()
	if err != nil {
		return createErrorResult(err.Error()), ServiceShowOutput{}, nil
	}

	logging.Debug("MCP: Showing service details",
		zap.String("project_id", projectID),
		zap.String("service_id", input.ServiceID))

	// Make API call to get service details
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := apiClient.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, input.ServiceID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Failed to get service details: %v", err)), ServiceShowOutput{}, nil
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200:
		if resp.JSON200 == nil {
			return createErrorResult("Empty response from API"), ServiceShowOutput{}, nil
		}

		service := *resp.JSON200
		output := ServiceShowOutput{
			Service: s.convertToServiceDetail(service),
		}

		return nil, output, nil

	case 401:
		return createErrorResult("Authentication failed: invalid API key"), ServiceShowOutput{}, nil
	case 403:
		return createErrorResult("Permission denied: insufficient access to service"), ServiceShowOutput{}, nil
	case 404:
		return createErrorResult(fmt.Sprintf("Service '%s' not found in project '%s'", input.ServiceID, projectID)), ServiceShowOutput{}, nil
	default:
		return createErrorResult(fmt.Sprintf("API request failed with status %d", resp.StatusCode())), ServiceShowOutput{}, nil
	}
}

// handleServiceCreate handles the tiger_service_create MCP tool
func (s *Server) handleServiceCreate(ctx context.Context, req *mcp.CallToolRequest, input ServiceCreateInput) (*mcp.CallToolResult, ServiceCreateOutput, error) {
	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return createErrorResult(err.Error()), ServiceCreateOutput{}, nil
	}

	// Load fresh project ID from current config
	projectID, err := s.loadProjectID()
	if err != nil {
		return createErrorResult(err.Error()), ServiceCreateOutput{}, nil
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

	// Parse CPU and Memory
	cpuMillis, memoryGbs, err := s.parseCPUMemory(input.CPU, input.Memory)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Invalid CPU/Memory specification: %v", err)), ServiceCreateOutput{}, nil
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
		return createErrorResult(fmt.Sprintf("Failed to create service: %v", err)), ServiceCreateOutput{}, nil
	}

	// Handle API response
	switch resp.StatusCode() {
	case 202:
		if resp.JSON202 == nil {
			return createErrorResult("Service creation request accepted but no response data received"), ServiceCreateOutput{}, nil
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
		return createErrorResult("Invalid request parameters"), ServiceCreateOutput{}, nil
	case 401:
		return createErrorResult("Authentication failed: invalid API key"), ServiceCreateOutput{}, nil
	case 403:
		return createErrorResult("Permission denied: insufficient access to create services"), ServiceCreateOutput{}, nil
	default:
		return createErrorResult(fmt.Sprintf("API request failed with status %d", resp.StatusCode())), ServiceCreateOutput{}, nil
	}
}

// handleServiceUpdatePassword handles the tiger_service_update_password MCP tool
func (s *Server) handleServiceUpdatePassword(ctx context.Context, req *mcp.CallToolRequest, input ServiceUpdatePasswordInput) (*mcp.CallToolResult, ServiceUpdatePasswordOutput, error) {
	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return createErrorResult(err.Error()), ServiceUpdatePasswordOutput{}, nil
	}

	// Load fresh project ID from current config
	projectID, err := s.loadProjectID()
	if err != nil {
		return createErrorResult(err.Error()), ServiceUpdatePasswordOutput{}, nil
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
		return createErrorResult(fmt.Sprintf("Failed to update service password: %v", err)), ServiceUpdatePasswordOutput{}, nil
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200, 204:
		return nil, ServiceUpdatePasswordOutput{
			Message: "Master password for 'tsdbadmin' user updated successfully",
		}, nil

	case 401:
		return createErrorResult("Authentication failed: invalid API key"), ServiceUpdatePasswordOutput{}, nil
	case 403:
		return createErrorResult("Permission denied: insufficient access to update service password"), ServiceUpdatePasswordOutput{}, nil
	case 404:
		return createErrorResult(fmt.Sprintf("Service '%s' not found in project '%s'", input.ServiceID, projectID)), ServiceUpdatePasswordOutput{}, nil
	case 400:
		var errorMsg string
		if resp.JSON400 != nil && resp.JSON400.Message != nil {
			errorMsg = *resp.JSON400.Message
		} else {
			errorMsg = "Invalid password"
		}
		return createErrorResult(fmt.Sprintf("Invalid password: %s", errorMsg)), ServiceUpdatePasswordOutput{}, nil

	default:
		return createErrorResult(fmt.Sprintf("API request failed with status %d", resp.StatusCode())), ServiceUpdatePasswordOutput{}, nil
	}
}
