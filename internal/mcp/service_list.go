package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceListInput represents input for service_list
type ServiceListInput struct{}

func (ServiceListInput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceListInput](nil))
}

// ServiceListOutput represents output for service_list
type ServiceListOutput struct {
	Services []ServiceInfo `json:"services"`
}

func (ServiceListOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceListOutput](nil))
}

// ServiceInfo represents simplified service information for MCP output
type ServiceInfo struct {
	ServiceID string        `json:"id" jsonschema:"Service identifier (10-character alphanumeric string)"`
	Name      string        `json:"name"`
	Status    string        `json:"status" jsonschema:"Service status (e.g., READY, PAUSED, CONFIGURING, UPGRADING)"`
	Type      string        `json:"type"`
	Region    string        `json:"region"`
	Created   string        `json:"created,omitempty"`
	Resources *ResourceInfo `json:"resources,omitempty"`
}

func (ServiceInfo) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceInfo](nil))
	schema.Properties["type"].Enum = util.AnySlice(validServiceTypes())
	return schema
}

func newServiceListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceList,
		Title: "List Database Services",
		Description: "List all database services in your Tiger Cloud project. " +
			"Returns services with status, type, region, and resource allocation.",
		InputSchema:  ServiceListInput{}.Schema(),
		OutputSchema: ServiceListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: util.Ptr(true),
			Title:         "List Database Services",
		},
	}
}

// handleServiceList handles the service_list MCP tool
func (s *Server) handleServiceList(ctx context.Context, req *mcp.CallToolRequest, input ServiceListInput) (*mcp.CallToolResult, ServiceListOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, ServiceListOutput{}, err
	}

	s.logger.Info("MCP: Listing services", slog.String("project_id", projectID))

	// Make API call to list services
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.GetServicesWithResponse(ctx, projectID)
	if err != nil {
		return nil, ServiceListOutput{}, fmt.Errorf("failed to list services: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusOK {
		return nil, ServiceListOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

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
}

// convertToServiceInfo converts an API Service to MCP ServiceInfo
func (s *Server) convertToServiceInfo(service api.Service) ServiceInfo {
	info := ServiceInfo{
		ServiceID: util.Deref(service.ServiceID),
		Name:      util.Deref(service.Name),
		Status:    util.DerefStr(service.Status),
		Type:      util.DerefStr(service.ServiceType),
		Region:    util.Deref(service.RegionCode),
	}

	// Add creation time if available
	if service.Created != nil {
		info.Created = service.Created.Format("2006-01-02T15:04:05Z")
	}

	// Add resource information if available
	if service.Resources != nil && len(*service.Resources) > 0 {
		resource := (*service.Resources)[0]
		if resource.Spec != nil {
			info.Resources = &ResourceInfo{}

			if resource.Spec.CPUMillis != nil {
				cpuCores := float64(*resource.Spec.CPUMillis) / 1000
				if cpuCores == float64(int(cpuCores)) {
					info.Resources.CPU = fmt.Sprintf("%.0f cores", cpuCores)
				} else {
					info.Resources.CPU = fmt.Sprintf("%.1f cores", cpuCores)
				}
			} else {
				// CPU is null - this indicates a free tier service
				info.Resources.CPU = "shared"
			}

			if resource.Spec.MemoryGbs != nil {
				info.Resources.Memory = fmt.Sprintf("%d GB", *resource.Spec.MemoryGbs)
			} else {
				// Memory is null - this indicates a free tier service
				info.Resources.Memory = "shared"
			}
		}
	}

	return info
}
