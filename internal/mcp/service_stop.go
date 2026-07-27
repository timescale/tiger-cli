package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/logging"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceStopInput represents input for service_stop
type ServiceStopInput struct {
	ServiceID string `json:"service_id"`
	Wait      bool   `json:"wait,omitempty"`
}

func (ServiceStopInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceStopInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["wait"].Description = "Whether to wait for the service to be fully stopped before returning. Default is false (recommended). Only set to true if your next steps require confirmation that the service is stopped. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	return schema
}

// ServiceStopOutput represents output for service_stop
type ServiceStopOutput struct {
	Status  string `json:"status" jsonschema:"Current service status after stop operation"`
	Message string `json:"message"`
}

func (ServiceStopOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceStopOutput](nil))
}

func newServiceStopTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceStop,
		Title: "Stop Database Service",
		Description: `Stop a running database service.

This operation stops a service that is currently running. The service will transition to a stopped/paused state and will no longer accept connections.`,
		InputSchema:  ServiceStopInput{}.Schema(),
		OutputSchema: ServiceStopOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: util.Ptr(true), // Stopping a service breaks existing connections and could cause app downtime
			IdempotentHint:  true,           // Stopping an already-stopped service is safe (but returns an error)
			OpenWorldHint:   util.Ptr(true),
			Title:           "Stop Database Service",
		},
	}
}

// handleServiceStop handles the service_stop MCP tool
func (s *Server) handleServiceStop(ctx context.Context, req *mcp.CallToolRequest, input ServiceStopInput) (*mcp.CallToolResult, ServiceStopOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceStopOutput{}, err
	}

	if err := common.CheckReadOnly(cfg.Config); err != nil {
		return nil, ServiceStopOutput{}, err
	}

	logging.Debug("MCP: Stopping service",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID))

	// Make API call to stop service
	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.StopServiceWithResponse(stopCtx, cfg.ProjectID, input.ServiceID)
	if err != nil {
		return nil, ServiceStopOutput{}, fmt.Errorf("failed to stop service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusAccepted {
		return nil, ServiceStopOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON202 == nil {
		return nil, ServiceStopOutput{}, fmt.Errorf("empty response from API")
	}

	service := *resp.JSON202

	// If wait is explicitly requested, wait for service to be paused
	message := "Service stop request accepted. The service may still be stopping."
	if input.Wait {
		if err := common.WaitForService(ctx, common.WaitForServiceArgs{
			Client:    cfg.Client,
			ProjectID: cfg.ProjectID,
			ServiceID: input.ServiceID,
			Handler: &common.StatusWaitHandler{
				TargetStatus: "PAUSED",
				Service:      &service,
			},
			Timeout:    waitTimeout,
			TimeoutMsg: "service may still be stopping",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Service stopped successfully!"
		}
	}

	// Return status and message (after wait so status is accurate)
	output := ServiceStopOutput{
		Status:  util.DerefStr(service.Status),
		Message: message,
	}

	return nil, output, nil
}
