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

// ServiceBackupsInput represents input for service_backups
type ServiceBackupsInput struct {
	ServiceID string `json:"service_id"`
}

func (ServiceBackupsInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceBackupsInput](nil))
	setServiceIDSchemaProperties(schema)
	return schema
}

// ServiceBackupsOutput represents output for service_backups
type ServiceBackupsOutput struct {
	Backups []api.Backup `json:"backups"`
}

func (ServiceBackupsOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceBackupsOutput](nil))
}

func newServiceBackupsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceBackups,
		Title: "List Service Backups",
		Description: "List the full and incremental backups taken for a service. " +
			"Backups run automatically on a schedule; there is no tool to create or delete one. " +
			"To restore data, fork the service with service_fork.",
		InputSchema:  ServiceBackupsInput{}.Schema(),
		OutputSchema: ServiceBackupsOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: util.Ptr(false),
			Title:         "List Service Backups",
		},
	}
}

// handleServiceBackups handles the service_backups MCP tool
func (s *Server) handleServiceBackups(ctx context.Context, req *mcp.CallToolRequest, input ServiceBackupsInput) (*mcp.CallToolResult, ServiceBackupsOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, ServiceBackupsOutput{}, err
	}

	s.logger.Info("MCP: Listing service backups",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
	)

	resp, err := client.GetBackupsWithResponse(ctx, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceBackupsOutput{}, fmt.Errorf("failed to list backups: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, ServiceBackupsOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, ServiceBackupsOutput{Backups: []api.Backup{}}, nil
	}

	return nil, ServiceBackupsOutput{Backups: *resp.JSON200}, nil
}
