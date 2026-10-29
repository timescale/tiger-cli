package mcp

import (
	"context"

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

	backups, err := common.JSON200(client.GetBackupsWithResponse(ctx, projectID, input.ServiceID))
	if err != nil {
		return nil, ServiceBackupListOutput{}, err
	}

	// Default to a non-nil slice so a JSON null marshals to `[]` rather than
	// `null`.
	if backups == nil {
		backups = []api.Backup{}
	}

	return nil, ServiceBackupListOutput{Backups: backups}, nil
}
