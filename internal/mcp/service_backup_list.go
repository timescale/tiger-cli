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

// ServiceBackupListInput represents input for service_backup_list
type ServiceBackupListInput struct {
	ServiceID string `json:"service_id"`
}

func (ServiceBackupListInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceBackupListInput](nil))
	setServiceIDSchemaProperties(schema)
	return schema
}

// ServiceBackupListOutput represents output for service_backup_list
type ServiceBackupListOutput struct {
	Backups []api.Backup `json:"backups"`
}

func (ServiceBackupListOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceBackupListOutput](nil))
}

func newServiceBackupListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceBackupList,
		Title: "List Service Backups",
		Description: "List the full and incremental backups taken for a service. " +
			"Backups run automatically on a schedule; there is no tool to create or delete one. " +
			"To restore data, fork the service with service_fork.",
		InputSchema:  ServiceBackupListInput{}.Schema(),
		OutputSchema: ServiceBackupListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "List Service Backups",
		},
	}
}

// handleServiceBackupList handles the service_backup_list MCP tool
func (s *Server) handleServiceBackupList(ctx context.Context, req *mcp.CallToolRequest, input ServiceBackupListInput) (*mcp.CallToolResult, ServiceBackupListOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, ServiceBackupListOutput{}, err
	}

	s.logger.Info("MCP: Listing service backups",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
	)

	resp, err := client.GetBackupsWithResponse(ctx, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceBackupListOutput{}, fmt.Errorf("failed to list backups: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, ServiceBackupListOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, ServiceBackupListOutput{Backups: []api.Backup{}}, nil
	}

	return nil, ServiceBackupListOutput{Backups: *resp.JSON200}, nil
}
