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

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/logging"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceResizeInput represents input for service_resize
type ServiceResizeInput struct {
	ServiceID string `json:"service_id"`
	CPUMemory string `json:"cpu_memory"`
	Wait      bool   `json:"wait,omitempty"`
}

func (ServiceResizeInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceResizeInput](nil))
	setServiceIDSchemaProperties(schema)

	schema.Properties["cpu_memory"].Description = "CPU and memory allocation combination. Choose from the available configurations."
	schema.Properties["cpu_memory"].Enum = util.AnySlice(common.GetAllowedResizeCPUMemoryConfigs().Strings())

	schema.Properties["wait"].Description = "Whether to wait for the service to be done resizing before returning. Default is false (recommended). Only set to true if your next steps require connecting to or querying this database. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	return schema
}

// ServiceResizeOutput represents output for service_resize
type ServiceResizeOutput struct {
	Status    string        `json:"status" jsonschema:"Current service status after resize operation"`
	Resources *ResourceInfo `json:"resources,omitempty"`
	Message   string        `json:"message"`
}

func (ServiceResizeOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceResizeOutput](nil))
}

func newServiceResizeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceResize,
		Title: "Resize Database Service",
		Description: `Resize a database service by changing its CPU and memory allocation.

This tool changes the compute resources allocated to your database service. The service
may be temporarily unavailable during the resize operation.

WARNING: Creates billable resource changes. Increasing resources will increase costs.`,
		InputSchema:  ServiceResizeInput{}.Schema(),
		OutputSchema: ServiceResizeOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: util.Ptr(false), // Not destructive, just modifies resources
			IdempotentHint:  true,            // Can resize to same size multiple times
			OpenWorldHint:   util.Ptr(true),
			Title:           "Resize Database Service",
		},
	}
}

// handleServiceResize handles the service_resize MCP tool
func (s *Server) handleServiceResize(ctx context.Context, req *mcp.CallToolRequest, input ServiceResizeInput) (*mcp.CallToolResult, ServiceResizeOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx, s.flags)
	if err != nil {
		return nil, ServiceResizeOutput{}, err
	}

	if err := common.CheckReadOnly(cfg.Config); err != nil {
		return nil, ServiceResizeOutput{}, err
	}

	logging.Debug("MCP: Resizing service",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID),
		zap.String("cpu_memory", input.CPUMemory),
	)

	// Parse CPU/Memory combination
	cpuMillis, memoryGBs, err := common.ParseCPUMemory(input.CPUMemory)
	if err != nil {
		return nil, ServiceResizeOutput{}, fmt.Errorf("invalid CPU/Memory specification: %w", err)
	}

	// Prepare resize request
	resizeReq := api.ResizeInput{
		CpuMillis: cpuMillis,
		MemoryGbs: memoryGBs,
	}

	// Make API call to resize service
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.ResizeServiceWithResponse(ctx, cfg.ProjectID, input.ServiceID, resizeReq)
	if err != nil {
		return nil, ServiceResizeOutput{}, fmt.Errorf("failed to resize service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusAccepted {
		return nil, ServiceResizeOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON202 == nil {
		return nil, ServiceResizeOutput{}, fmt.Errorf("empty response from API")
	}

	service := *resp.JSON202

	// If wait is requested, wait for resize to complete
	message := "Resize request accepted. The service may still be resizing."
	if input.Wait {
		if err := common.WaitForService(ctx, common.WaitForServiceArgs{
			Client:    cfg.Client,
			ProjectID: cfg.ProjectID,
			ServiceID: input.ServiceID,
			Handler: &common.StatusWaitHandler{
				TargetStatus: "READY",
				Service:      &service,
			},
			Timeout:    waitTimeout,
			TimeoutMsg: "service may still be resizing",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Service resized successfully!"
		}
	}

	// Return status, resources, and message (after wait so status is accurate)
	detail := s.convertToServiceDetail(cfg.Config, service, false)
	output := ServiceResizeOutput{
		Status:    detail.Status,
		Resources: detail.Resources,
		Message:   message,
	}

	return nil, output, nil
}
