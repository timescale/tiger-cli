package mcp

import (
	"context"
	"errors"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceGetInput represents input for service_get
type ServiceGetInput struct {
	ServiceID    string `json:"service_id"`
	WithPassword bool   `json:"with_password,omitempty"`
}

func (ServiceGetInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceGetInput](nil))
	setServiceIDSchemaProperties(schema)
	setWithPasswordSchemaProperties(schema)

	return schema
}

// ServiceGetOutput represents output for service_get
type ServiceGetOutput struct {
	Service ServiceDetail `json:"service"`
}

func (ServiceGetOutput) Schema(experimental bool) *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceGetOutput](nil))
	schema.Properties["service"] = ServiceDetail{}.Schema(experimental)
	return schema
}

func newServiceGetTool(experimental bool) *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceGet,
		Title: "Get Service Details",
		Description: "Get detailed information for a specific database service. " +
			"Returns connection endpoints, replica configuration, resource allocation, creation time, and status.",
		InputSchema:  ServiceGetInput{}.Schema(),
		OutputSchema: ServiceGetOutput{}.Schema(experimental),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "Get Service Details",
		},
	}
}

// handleServiceGet handles the service_get MCP tool
func (s *Server) handleServiceGet(ctx context.Context, req *mcp.CallToolRequest, input ServiceGetInput) (*mcp.CallToolResult, ServiceGetOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceGetOutput{}, err
	}

	// Make API call to get service details
	service, err := common.JSON200(client.GetServiceWithResponse(ctx, projectID, input.ServiceID))
	if err != nil {
		return nil, ServiceGetOutput{}, err
	}

	output := ServiceGetOutput{
		Service: s.convertToServiceDetail(cfg, service, input.WithPassword),
	}

	// Check if password was requested but not available
	if input.WithPassword && output.Service.Password == "" {
		return nil, ServiceGetOutput{}, errors.New("requested password but password not available")
	}

	return nil, output, nil
}
