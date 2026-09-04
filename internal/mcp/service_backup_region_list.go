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

	s.logger.Info("MCP: Listing service backup regions",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
	)

	resp, err := client.GetBackupRegionsWithResponse(ctx, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceBackupRegionListOutput{}, fmt.Errorf("failed to list backup regions: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, ServiceBackupRegionListOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, ServiceBackupRegionListOutput{Regions: []api.BackupRegion{}}, nil
	}

	return nil, ServiceBackupRegionListOutput{Regions: *resp.JSON200}, nil
}
