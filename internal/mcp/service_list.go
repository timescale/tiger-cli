package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

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

func (ServiceListOutput) Schema(experimental bool) *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceListOutput](nil))
	schema.Properties["services"].Items = ServiceInfo{}.Schema(experimental)
	return schema
}

// ServiceInfo represents simplified service information for MCP output
type ServiceInfo struct {
	ServiceID   string        `json:"id"`
	Name        string        `json:"name"`
	Status      string        `json:"status"`
	Platform    string        `json:"platform,omitempty"`
	Type        string        `json:"type"`
	Region      string        `json:"region"`
	Created     string        `json:"created,omitempty"`
	Environment string        `json:"environment"`
	Resources   *ResourceInfo `json:"resources,omitempty"`
}

func (ServiceInfo) Schema(experimental bool) *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceInfo](nil))

	schema.Properties["id"].Description = "Service identifier (10-character alphanumeric string)"

	schema.Properties["status"].Description = "Service status"
	schema.Properties["status"].Examples = []any{"READY", "PAUSED", "CONFIGURING", "UPGRADING"}

	setPlatformSchemaProperties(schema, experimental)

	schema.Properties["type"].Enum = util.AnySlice(validServiceTypes())

	schema.Properties["environment"].Description = "Environment tag (DEV or PROD). Under read_only=prod, services tagged PROD cannot be modified."

	setResourceInfoSchemaProperties(schema.Properties["resources"], experimental)

	return schema
}

func newServiceListTool(experimental bool) *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceList,
		Title: "List Database Services",
		Description: "List all database services in your Tiger Cloud project. " +
			"Returns services with status, type, region, and resource allocation.",
		InputSchema:  ServiceListInput{}.Schema(),
		OutputSchema: ServiceListOutput{}.Schema(experimental),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
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
	resp, err := client.GetServicesWithResponse(ctx, projectID, nil)
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
		ServiceID: service.ServiceID,
		Name:      service.Name,
		Status:    string(service.Status),
		Type:      string(service.ServiceType),
		Region:    service.RegionCode,
		Created:   service.Created.Format("2006-01-02T15:04:05Z"),
		// The read_only=prod instructions point the model at this field, so a list
		// result has to carry it too, not just ServiceDetail.
		Environment: string(common.ServiceEnvironmentTag(service)),
	}

	// The platform is a preview field, returned only behind the gate.
	if s.app.Experimental {
		info.Platform = string(util.Deref(service.Platform))
	}

	info.Resources = convertToResourceInfo(service, s.app.Experimental)

	return info
}
