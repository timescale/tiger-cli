package mcp

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceBackupRegionListInput represents input for service_backup_region_list
type ServiceBackupRegionListInput struct {
	ServiceID string `json:"service_id"`
}

func (ServiceBackupRegionListInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceBackupRegionListInput](nil))
	setServiceIDSchemaProperties(schema)
	return schema
}

// ServiceBackupRegionListOutput represents output for service_backup_region_list
type ServiceBackupRegionListOutput struct {
	Regions []api.BackupRegion `json:"regions"`
}

func (ServiceBackupRegionListOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceBackupRegionListOutput](nil))
}

func newServiceBackupRegionListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceBackupRegionList,
		Title: "List Service Backup Regions",
		Description: "List the additional regions a service's backups are copied to. " +
			"The region the service runs in is not listed; it always has a copy.",
		InputSchema:  ServiceBackupRegionListInput{}.Schema(),
		OutputSchema: ServiceBackupRegionListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "List Service Backup Regions",
		},
	}
}

// handleServiceBackupRegionList handles the service_backup_region_list MCP tool
func (s *Server) handleServiceBackupRegionList(ctx context.Context, req *mcp.CallToolRequest, input ServiceBackupRegionListInput) (*mcp.CallToolResult, ServiceBackupRegionListOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, ServiceBackupRegionListOutput{}, err
	}

	regions, err := common.JSON200(client.GetBackupRegionsWithResponse(ctx, projectID, input.ServiceID))
	if err != nil {
		return nil, ServiceBackupRegionListOutput{}, err
	}

	// Default to a non-nil slice so a JSON null marshals to `[]` rather than
	// `null`.
	if regions == nil {
		regions = []api.BackupRegion{}
	}

	return nil, ServiceBackupRegionListOutput{Regions: regions}, nil
}
