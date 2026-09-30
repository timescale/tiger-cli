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

// ServiceAllowListAttachInput represents input for service_allowlist_attach
type ServiceAllowListAttachInput struct {
	ServiceID   string `json:"service_id"`
	AllowListID string `json:"allow_list_id"`
}

func (ServiceAllowListAttachInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceAllowListAttachInput](nil))
	setServiceIDSchemaProperties(schema)
	schema.Properties["allow_list_id"].Description = "The IP allow list to attach the service to. Use allowlist_list to find IP allow list IDs."
	schema.Properties["allow_list_id"].Examples = []any{"1234567890"}
	return schema
}

// ServiceAllowListAttachOutput represents output for service_allowlist_attach
type ServiceAllowListAttachOutput struct {
	Message string `json:"message"`
}

func (ServiceAllowListAttachOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceAllowListAttachOutput](nil))
}

func newServiceAllowListAttachTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceAllowListAttach,
		Title: "Attach Service to IP Allow List",
		Description: `Restrict a service's connections to the IP ranges in an IP allow list.

A service has at most one IP allow list; attaching a second one without detaching the first is refused.`,
		InputSchema:  ServiceAllowListAttachInput{}.Schema(),
		OutputSchema: ServiceAllowListAttachOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
			Title:           "Attach Service to IP Allow List",
		},
	}
}

// handleServiceAllowListAttach handles the service_allowlist_attach MCP tool
func (s *Server) handleServiceAllowListAttach(ctx context.Context, req *mcp.CallToolRequest, input ServiceAllowListAttachInput) (*mcp.CallToolResult, ServiceAllowListAttachOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceAllowListAttachOutput{}, err
	}

	if err := common.CheckReadOnlyByServiceID(ctx, cfg, client, projectID, input.ServiceID); err != nil {
		return nil, ServiceAllowListAttachOutput{}, err
	}

	s.logger.Info("MCP: Attaching service to IP allow list",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.String("allow_list_id", input.AllowListID),
	)

	resp, err := client.AttachServiceToAllowListWithResponse(ctx, projectID, input.ServiceID, api.ServiceAllowListInput{
		AllowListID: input.AllowListID,
	})
	if err != nil {
		return nil, ServiceAllowListAttachOutput{}, fmt.Errorf("failed to attach service to IP allow list: %w", err)
	}

	if resp.StatusCode() != http.StatusAccepted {
		return nil, ServiceAllowListAttachOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON202 == nil {
		return nil, ServiceAllowListAttachOutput{}, fmt.Errorf("empty response from API")
	}

	return nil, ServiceAllowListAttachOutput{
		Message: fmt.Sprintf("Service '%s' attached to IP allow list '%s'.", input.ServiceID, input.AllowListID),
	}, nil
}
