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

// ServiceRenameInput represents input for service_rename
type ServiceRenameInput struct {
	ServiceID string `json:"service_id"`
	Name      string `json:"name"`
}

func (ServiceRenameInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceRenameInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["name"].Description = "The new name for the service."
	schema.Properties["name"].Examples = []any{"analytics-prod", "billing-staging"}

	return schema
}

// ServiceRenameOutput represents output for service_rename
type ServiceRenameOutput struct {
	Message   string `json:"message"`
	ServiceID string `json:"service_id"`
	Name      string `json:"name"`
}

func (ServiceRenameOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceRenameOutput](nil))
	schema.Properties["message"].Description = "Human-readable result of the rename operation"
	schema.Properties["service_id"].Description = "Unique identifier of the renamed service, unchanged by the rename"
	schema.Properties["name"].Description = "Name of the service as stored after the rename"
	return schema
}

func newServiceRenameTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceRename,
		Title: "Rename Database Service",
		Description: "Rename a database service. " +
			"Only the display name changes: the service ID, endpoints, and data are untouched, so existing connections and connection strings keep working.",
		InputSchema:  ServiceRenameInput{}.Schema(),
		OutputSchema: ServiceRenameOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // A rename loses nothing and can be reversed by renaming back
			IdempotentHint:  true,       // Renaming to the name a service already has changes nothing
			OpenWorldHint:   new(false),
			Title:           "Rename Database Service",
		},
	}
}

// handleServiceRename handles the service_rename MCP tool
func (s *Server) handleServiceRename(ctx context.Context, req *mcp.CallToolRequest, input ServiceRenameInput) (*mcp.CallToolResult, ServiceRenameOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceRenameOutput{}, err
	}

	if err := common.CheckReadOnlyByServiceID(ctx, cfg, client, projectID, input.ServiceID); err != nil {
		return nil, ServiceRenameOutput{}, err
	}

	s.logger.Info("MCP: Renaming service",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID))

	resp, err := client.RenameServiceWithResponse(ctx, projectID, input.ServiceID, api.ServiceRename{Name: input.Name})
	if err != nil {
		return nil, ServiceRenameOutput{}, fmt.Errorf("failed to rename service: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, ServiceRenameOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, ServiceRenameOutput{}, fmt.Errorf("empty response from API")
	}
	service := *resp.JSON200

	output := ServiceRenameOutput{
		Message:   "Service renamed successfully.",
		ServiceID: service.ServiceID,
		Name:      service.Name,
	}

	return nil, output, nil
}
