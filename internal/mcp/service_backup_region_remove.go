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

// removeBackupRegionConfirmationKey is the InputRequests/InputResponses key of
// the PROD removal prompt.
const removeBackupRegionConfirmationKey = "confirm_remove_backup_region"

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
	Removed bool   `json:"removed"`
	Message string `json:"message"`
}

func (ServiceBackupRegionRemoveOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceBackupRegionRemoveOutput](nil))

	schema.Properties["removed"].Description = "Whether the region stopped receiving backup copies. False when the user declined the confirmation prompt for a PROD service; do not retry unless the user asks again."
	schema.Properties["message"].Description = "Human-readable outcome of the operation"

	return schema
}

func newServiceBackupRegionRemoveTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceBackupRegionRemove,
		Title: "Remove Service Backup Region",
		Description: `Stop copying a service's backups to a region.

Copies already stored there are deleted in the background. Removing a region from a service tagged PROD automatically prompts the user, via an elicitation request through the MCP client, to confirm before it proceeds, so agents don't need to ask the user for confirmation themselves; if the client cannot prompt, the removal is refused and the user must run 'tiger service backup region remove' from the CLI instead.`,
		InputSchema:  ServiceBackupRegionRemoveInput{}.Schema(),
		OutputSchema: ServiceBackupRegionRemoveOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // deletes existing copies in that region
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
			Title:           "Remove Service Backup Region",
		},
	}
}

// handleServiceBackupRegionRemove handles the service_backup_region_remove MCP
// tool.
//
// Removing a backup region from a PROD service is a multi round-trip call: the
// first invocation returns an elicitation asking the user to confirm, and the
// SDK re-invokes the handler with the answer in InputResponses — client-side
// on protocol 2026-07-28 and later, server-side (via ServerSession.Elicit) for
// older clients — so the handler never calls Elicit itself.
func (s *Server) handleServiceBackupRegionRemove(ctx context.Context, req *mcp.CallToolRequest, input ServiceBackupRegionRemoveInput) (*mcp.CallToolResult, ServiceBackupRegionRemoveOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceBackupRegionRemoveOutput{}, err
	}

	// Refuse without an API call under read_only=all. prod needs the tag, so the
	// real gate waits for the fetch below.
	if cfg.ReadOnly.BlocksAll() {
		return nil, ServiceBackupRegionRemoveOutput{}, common.ErrReadOnly
	}

	// The service's tag decides both the prod half of the read-only gate and
	// whether the user has to confirm, so fetch it unconditionally.
	service, err := common.GetService(ctx, client, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceBackupRegionRemoveOutput{}, err
	}

	tag := common.ServiceEnvironmentTag(*service)
	if err := common.CheckReadOnly(cfg, tag); err != nil {
		return nil, ServiceBackupRegionRemoveOutput{}, err
	}

	if tag == api.EnvironmentTagPROD {
		answer, answered := req.Params.InputResponses[removeBackupRegionConfirmationKey]
		if !answered {
			result, err := promptProdBackupRegionRemove(req, *service, input.RegionCode)
			return result, ServiceBackupRegionRemoveOutput{}, err
		}
		if !serviceIDConfirmed(answer, input.ServiceID) {
			return nil, ServiceBackupRegionRemoveOutput{
				Removed: false,
				Message: fmt.Sprintf("Removal cancelled: the user did not confirm removing backup region %q from PROD service %q by typing its ID.", input.RegionCode, input.ServiceID),
			}, nil
		}
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
		Removed: true,
		Message: fmt.Sprintf("Backups for service '%s' will no longer be copied to '%s'.", input.ServiceID, input.RegionCode),
	}, nil
}

// promptProdBackupRegionRemove returns the input-required result that asks the
// user to confirm removing a backup region from a PROD service. It refuses
// outright when the client can't show the prompt: silently deleting existing
// backup copies in a production service's region without a human in the loop
// is the one outcome this tool must never produce.
func promptProdBackupRegionRemove(req *mcp.CallToolRequest, service api.Service, regionCode string) (*mcp.CallToolResult, error) {
	if !clientSupportsFormElicitation(req) {
		return nil, fmt.Errorf("removing backup region %s from service %s requires the user's confirmation because it is tagged PROD, but this MCP client does not support elicitation; ask the user to run 'tiger service backup region remove %s --region %s' instead", regionCode, service.ServiceID, service.ServiceID, regionCode)
	}
	return serviceIDConfirmationRequest(
		removeBackupRegionConfirmationKey,
		fmt.Sprintf("Stop copying PRODUCTION service %q (%s) backups to %q? Backup copies in that region will be deleted.", service.Name, service.ServiceID, regionCode),
		service.ServiceID,
	), nil
}
