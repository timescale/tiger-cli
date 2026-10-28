package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceResizeInput represents input for service_resize
type ServiceResizeInput struct {
	ServiceID string `json:"service_id"`
	CPUMemory string `json:"cpu_memory,omitempty"`
	IOPS      *int   `json:"iops,omitempty"`
	Wait      bool   `json:"wait,omitempty"`
}

// Schema requires cpu_memory unless the experimental gate adds iops, after
// which the handler requires at least one of the two.
func (ServiceResizeInput) Schema(experimental bool) *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceResizeInput](nil))
	setServiceIDSchemaProperties(schema)

	cpuMemoryDescription := "CPU and memory allocation combination."
	if experimental {
		cpuMemoryDescription = "CPU and memory allocation combination. If not specified, the service keeps its current CPU and memory, and at least one of cpu_memory or iops is required."
	} else {
		schema.Required = append(schema.Required, "cpu_memory")
	}
	setCPUMemorySchemaProperties(schema, false, experimental, cpuMemoryDescription,
		"The valid combinations depend on the service's platform (the platform field from service_get).")

	setIOPSSchemaProperties(schema, experimental, "")

	schema.Properties["wait"].Description = "Whether to wait for the service to be done resizing before returning. Default is false (recommended). Only set to true if your next steps require connecting to or querying this database. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	return schema
}

// ServiceResizeOutput represents output for service_resize
type ServiceResizeOutput struct {
	Status    string        `json:"status"`
	Resources *ResourceInfo `json:"resources,omitempty"`
	Message   string        `json:"message"`
}

func (ServiceResizeOutput) Schema(experimental bool) *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceResizeOutput](nil))
	schema.Properties["status"].Description = "Current service status after resize operation"
	setResourceInfoSchemaProperties(schema.Properties["resources"], experimental)
	return schema
}

func newServiceResizeTool(experimental bool) *mcp.Tool {
	// IOPS are a preview, described only behind the gate.
	resources := "CPU and memory allocation"
	if experimental {
		resources = "CPU and memory allocation or its provisioned IOPS"
	}

	return &mcp.Tool{
		Name:  toolServiceResize,
		Title: "Resize Database Service",
		Description: `Resize a database service by changing its ` + resources + `.

This tool changes the compute resources allocated to your database service. The service
may be temporarily unavailable during the resize operation.

WARNING: Creates billable resource changes. Increasing resources will increase costs.`,
		InputSchema:  ServiceResizeInput{}.Schema(experimental),
		OutputSchema: ServiceResizeOutput{}.Schema(experimental),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // Not destructive, just modifies resources
			IdempotentHint:  true,       // Can resize to same size multiple times
			OpenWorldHint:   new(false),
			Title:           "Resize Database Service",
		},
	}
}

// handleServiceResize handles the service_resize MCP tool
func (s *Server) handleServiceResize(ctx context.Context, req *mcp.CallToolRequest, input ServiceResizeInput) (*mcp.CallToolResult, ServiceResizeOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceResizeOutput{}, err
	}

	// The CPU/memory check needs the service, so refuse read_only=all before
	// fetching it, then gate on the fetched service's own tag.
	if cfg.ReadOnly.BlocksAll() {
		return nil, ServiceResizeOutput{}, common.ErrReadOnly
	}
	current, err := common.GetService(ctx, client, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceResizeOutput{}, err
	}
	if err := common.CheckReadOnly(cfg, common.ServiceEnvironmentTag(*current)); err != nil {
		return nil, ServiceResizeOutput{}, err
	}

	s.logger.Info("MCP: Resizing service",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.String("cpu_memory", input.CPUMemory),
		slog.Any("iops", input.IOPS),
	)

	// The schema requires cpu_memory unless the experimental gate adds iops.
	// An IOPS-only resize omits CPU and memory, which the API leaves unchanged.
	if input.CPUMemory == "" && input.IOPS == nil {
		return nil, ServiceResizeOutput{}, errors.New("at least one of cpu_memory or iops is required")
	}
	var cpuMillis, memoryGBs *string
	if input.CPUMemory != "" {
		// Parse CPU/Memory combination against the combinations of the
		// service's platform. Not gated on experimental: only a service already
		// created on the fluid platform gets the fluid combinations.
		cpuMillisStr, memoryGBsStr, err := common.CPUMemoryConfigsFor(util.Deref(current.Platform)).WithoutShared().Parse(input.CPUMemory)
		if err != nil {
			return nil, ServiceResizeOutput{}, fmt.Errorf("invalid CPU/Memory specification: %w", err)
		}
		cpuMillis, memoryGBs = &cpuMillisStr, &memoryGBsStr
	}

	// Prepare resize request
	resizeReq := api.ResizeInput{
		CPUMillis: cpuMillis,
		MemoryGbs: memoryGBs,
	}
	if s.app.Experimental {
		resizeReq.Iops = input.IOPS
	}

	// Make API call to resize service
	resp, err := client.ResizeServiceWithResponse(ctx, projectID, input.ServiceID, resizeReq)
	if err != nil {
		return nil, ServiceResizeOutput{}, fmt.Errorf("failed to resize service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusAccepted {
		return nil, ServiceResizeOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON202 == nil {
		return nil, ServiceResizeOutput{}, errors.New("empty response from API")
	}

	service := *resp.JSON202

	// If wait is requested, wait for resize to complete
	message := "Resize request accepted. The service may still be resizing."
	if input.Wait {
		if err := common.WaitForService(ctx, common.WaitForServiceArgs{
			Client:       client,
			ProjectID:    projectID,
			ServiceID:    input.ServiceID,
			Service:      &service,
			TargetStatus: api.DeployStatusREADY,
			Timeout:      waitTimeout,
			TimeoutMsg:   "service may still be resizing",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Service resized."
		}
	}

	// Return status, resources, and message (after wait so status is accurate)
	detail := s.convertToServiceDetail(cfg, service, false)
	output := ServiceResizeOutput{
		Status:    detail.Status,
		Resources: detail.Resources,
		Message:   message,
	}

	return nil, output, nil
}
