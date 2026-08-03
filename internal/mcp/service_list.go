package mcp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/logging"
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
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceListOutput{}, err
	}

	logging.Debug("MCP: Listing services", zap.String("project_id", cfg.ProjectID))

	// Make API call to list services
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.GetServicesWithResponse(ctx, cfg.ProjectID)
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
