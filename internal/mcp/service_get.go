package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

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
	Service ServiceGetDetail `json:"service"`
}

func (ServiceGetOutput) Schema(experimental bool) *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceGetOutput](nil))
	schema.Properties["service"] = ServiceGetDetail{}.Schema(experimental)
	return schema
}

// ServiceGetDetail is a ServiceDetail plus the fields only service_get
// returns.
type ServiceGetDetail struct {
	ServiceDetail
	BranchCount *int `json:"branch_count,omitempty"`
}

func (ServiceGetDetail) Schema(experimental bool) *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceGetDetail](nil))
	setServiceDetailSchemaProperties(schema, experimental)

	// Branches are a preview, so the count is described only behind the gate.
	if experimental {
		schema.Properties["branch_count"].Description = "Number of branches of the service (only returned for Fluid Services)"
	} else {
		delete(schema.Properties, "branch_count")
	}

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

	s.logger.Info("MCP: Getting service details",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID))

	// Make API call to get service details
	resp, err := client.GetServiceWithResponse(ctx, projectID, input.ServiceID)
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
		Service: ServiceGetDetail{ServiceDetail: s.convertToServiceDetail(cfg, *resp.JSON200, input.WithPassword)},
	}

	// Branches are a preview, so they're counted only behind the gate. The
	// rest of the output doesn't depend on the count.
	if s.app.Experimental && common.CanHaveBranches(*resp.JSON200) {
		if count, err := common.CountBranches(ctx, client, projectID, input.ServiceID); err != nil {
			s.logger.Error("MCP: Failed to count branches", slog.Any("error", err))
		} else {
			output.Service.BranchCount = &count
		}
	}

	// Check if password was requested but not available
	if input.WithPassword && output.Service.Password == "" {
		return nil, ServiceGetOutput{}, fmt.Errorf("requested password but password not available")
	}

	return nil, output, nil
}
