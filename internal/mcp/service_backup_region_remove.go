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

// ServiceBackupRegionRemoveInput represents input for service_backup_region_remove
type ServiceBackupRegionRemoveInput struct {
	ServiceID  string `json:"service_id"`
	RegionCode string `json:"region_code"`
}

func (ServiceBackupRegionRemoveInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceBackupRegionRemoveInput](nil))
	setServiceIDSchemaProperties(schema)

	schema.Properties["region_code"].Description = "The region to stop copying backups to."
	schema.Properties["region_code"].Examples = []any{"us-east-1", "eu-central-1"}

	return schema
}

// ServiceBackupRegionRemoveOutput represents output for service_backup_region_remove
type ServiceBackupRegionRemoveOutput struct {
	Message string `json:"message"`
}

func (ServiceBackupRegionRemoveOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceBackupRegionRemoveOutput](nil))
}

func newServiceBackupRegionRemoveTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceBackupRegionRemove,
		Title: "Remove Service Backup Region",
		Description: `Stop copying a service's backups to a region.

Copies already stored there are deleted in the background and cannot be recovered.`,
		InputSchema:  ServiceBackupRegionRemoveInput{}.Schema(),
		OutputSchema: ServiceBackupRegionRemoveOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // deletes existing copies in that region, irrecoverably
			IdempotentHint:  true,
			OpenWorldHint:   new(true),
			Title:           "Remove Service Backup Region",
		},
	}
}

// handleServiceBackupRegionRemove handles the service_backup_region_remove MCP tool
func (s *Server) handleServiceBackupRegionRemove(ctx context.Context, req *mcp.CallToolRequest, input ServiceBackupRegionRemoveInput) (*mcp.CallToolResult, ServiceBackupRegionRemoveOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceBackupRegionRemoveOutput{}, err
	}

	if err := common.CheckReadOnlyByServiceID(ctx, cfg, client, projectID, input.ServiceID); err != nil {
		return nil, ServiceBackupRegionRemoveOutput{}, err
	}

	s.logger.Info("MCP: Removing service backup region",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.String("region_code", input.RegionCode),
	)

	resp, err := client.DeleteBackupRegionWithResponse(ctx, projectID, input.ServiceID, input.RegionCode)
	if err != nil {
		return nil, ServiceBackupRegionRemoveOutput{}, fmt.Errorf("failed to remove backup region: %w", err)
	}

	if resp.StatusCode() != http.StatusNoContent {
		return nil, ServiceBackupRegionRemoveOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	return nil, ServiceBackupRegionRemoveOutput{
		Message: fmt.Sprintf("Backups for service '%s' will no longer be copied to '%s'.", input.ServiceID, input.RegionCode),
	}, nil
}
