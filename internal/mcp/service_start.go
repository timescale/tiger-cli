package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceStartInput represents input for service_start
type ServiceStartInput struct {
	ServiceID string `json:"service_id"`
	Wait      bool   `json:"wait,omitempty"`
}

func (ServiceStartInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceStartInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["wait"].Description = "Whether to wait for the service to be fully started before returning. Default is false (recommended). Only set to true if your next steps require connecting to or querying this database. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	return schema
}

// ServiceStartOutput represents output for service_start
type ServiceStartOutput struct {
	Status  string `json:"status" jsonschema:"Current service status after start operation"`
	Message string `json:"message"`
}

func (ServiceStartOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceStartOutput](nil))
}

func newServiceStartTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceStart,
		Title: "Start Database Service",
		Description: `Start a stopped database service.

This operation starts a service that is currently in a stopped/paused state. The service will transition to a ready state and become available for connections.`,
		InputSchema:  ServiceStartInput{}.Schema(),
		OutputSchema: ServiceStartOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // Starting a service cannot really break anything
			IdempotentHint:  true,       // Starting an already-started service is safe (but returns an error)
			OpenWorldHint:   new(true),
			Title:           "Start Database Service",
		},
	}
}

// handleServiceStart handles the service_start MCP tool
func (s *Server) handleServiceStart(ctx context.Context, req *mcp.CallToolRequest, input ServiceStartInput) (*mcp.CallToolResult, ServiceStartOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceStartOutput{}, err
	}

	if err := common.CheckReadOnlyByServiceID(ctx, cfg, client, projectID, input.ServiceID); err != nil {
		return nil, ServiceStartOutput{}, err
	}

	s.logger.Info("MCP: Starting service",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID))

	// Make API call to start service
	resp, err := client.StartServiceWithResponse(ctx, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceStartOutput{}, fmt.Errorf("failed to start service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusAccepted {
		return nil, ServiceStartOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON202 == nil {
		return nil, ServiceStartOutput{}, fmt.Errorf("empty response from API")
	}

	service := *resp.JSON202

	// If wait is explicitly requested, wait for service to be ready
	message := "Service start request accepted. The service may still be starting."
	if input.Wait {
		if err := common.WaitForService(ctx, common.WaitForServiceArgs{
			Client:    client,
			ProjectID: projectID,
			ServiceID: input.ServiceID,
			Handler: &common.StatusWaitHandler{
				TargetStatus: "READY",
				Service:      &service,
			},
			Timeout:    waitTimeout,
			TimeoutMsg: "service may still be starting",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Service started successfully and is ready!"
		}
	}

	// Return status and message (after wait so status is accurate)
	output := ServiceStartOutput{
		Status:  string(service.Status),
		Message: message,
	}

	return nil, output, nil
}
