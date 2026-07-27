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

func (ServiceGetOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceGetOutput](nil))
}

func newServiceGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceGet,
		Title: "Get Service Details",
		Description: "Get detailed information for a specific database service. " +
			"Returns connection endpoints, replica configuration, resource allocation, creation time, and status.",
		InputSchema:  ServiceGetInput{}.Schema(),
		OutputSchema: ServiceGetOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: util.Ptr(true),
			Title:         "Get Service Details",
		},
	}
}

// handleServiceGet handles the service_get MCP tool
func (s *Server) handleServiceGet(ctx context.Context, req *mcp.CallToolRequest, input ServiceGetInput) (*mcp.CallToolResult, ServiceGetOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceGetOutput{}, err
	}

	logging.Debug("MCP: Getting service details",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID))

	// Make API call to get service details
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.GetServiceWithResponse(ctx, cfg.ProjectID, input.ServiceID)
	if err != nil {
		return nil, ServiceGetOutput{}, fmt.Errorf("failed to get service details: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusOK {
		return nil, ServiceGetOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, ServiceGetOutput{}, fmt.Errorf("empty response from API")
	}

	output := ServiceGetOutput{
		Service: s.convertToServiceDetail(*resp.JSON200, input.WithPassword),
	}

	// Check if password was requested but not available
	if input.WithPassword && output.Service.Password == "" {
		return nil, ServiceGetOutput{}, fmt.Errorf("requested password but password not available")
	}

	return nil, output, nil
}
