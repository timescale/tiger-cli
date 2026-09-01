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

// ServiceBackupRegionAddInput represents input for service_backup_region_add
type ServiceBackupRegionAddInput struct {
	ServiceID  string `json:"service_id"`
	RegionCode string `json:"region_code"`
}

func (ServiceBackupRegionAddInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceBackupRegionAddInput](nil))
	setServiceIDSchemaProperties(schema)

	schema.Properties["region_code"].Description = "The region to copy backups to. It cannot be the region the service runs in."
	schema.Properties["region_code"].Examples = []any{"us-east-1", "eu-central-1"}

	return schema
}

// ServiceBackupRegionAddOutput represents output for service_backup_region_add
type ServiceBackupRegionAddOutput struct {
	Region  api.BackupRegion `json:"region"`
	Message string           `json:"message"`
}

func (ServiceBackupRegionAddOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceBackupRegionAddOutput](nil))
}

func newServiceBackupRegionAddTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceBackupRegionAdd,
		Title: "Add Service Backup Region",
		Description: `Start copying a service's backups to another region.

The region is added immediately; existing backups are copied to it in the background.`,
		InputSchema:  ServiceBackupRegionAddInput{}.Schema(),
		OutputSchema: ServiceBackupRegionAddOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  true,
			OpenWorldHint:   new(true),
			Title:           "Add Service Backup Region",
		},
	}
}

// handleServiceBackupRegionAdd handles the service_backup_region_add MCP tool
func (s *Server) handleServiceBackupRegionAdd(ctx context.Context, req *mcp.CallToolRequest, input ServiceBackupRegionAddInput) (*mcp.CallToolResult, ServiceBackupRegionAddOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceBackupRegionAddOutput{}, err
	}

	if err := common.CheckReadOnlyByServiceID(ctx, cfg, client, projectID, input.ServiceID); err != nil {
		return nil, ServiceBackupRegionAddOutput{}, err
	}

	s.logger.Info("MCP: Adding service backup region",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.String("region_code", input.RegionCode),
	)

	resp, err := client.CreateBackupRegionWithResponse(ctx, projectID, input.ServiceID, api.BackupRegionCreate{
		RegionCode: input.RegionCode,
	})
	if err != nil {
		return nil, ServiceBackupRegionAddOutput{}, fmt.Errorf("failed to add backup region: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, ServiceBackupRegionAddOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON201 == nil {
		return nil, ServiceBackupRegionAddOutput{}, fmt.Errorf("empty response from API")
	}

	return nil, ServiceBackupRegionAddOutput{
		Region:  *resp.JSON201,
		Message: fmt.Sprintf("Backups for service '%s' will now be copied to '%s'.", input.ServiceID, input.RegionCode),
	}, nil
}
