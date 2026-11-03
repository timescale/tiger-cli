package mcp

import (
	"context"
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

// BranchResizeInput represents input for branch_resize
type BranchResizeInput struct {
	BranchID  string `json:"branch_id"`
	CPUMemory string `json:"cpu_memory,omitempty"`
	IOPS      *int   `json:"iops,omitempty"`
}

// Schema leaves both cpu_memory and iops optional; the handler requires at
// least one of the two.
func (BranchResizeInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchResizeInput](nil))

	setBranchIDSchemaProperties(schema)
	setBranchCPUMemorySchemaProperties(schema, "CPU and memory allocation combination. If not specified, the branch keeps its current CPU and memory.")
	setBranchIOPSSchemaProperties(schema, "If not specified while changing CPU/memory, the IOPS is reset to the baseline for the new size.")

	return schema
}

// BranchResizeOutput represents output for branch_resize
type BranchResizeOutput struct {
	Status    string        `json:"status"`
	Resources *ResourceInfo `json:"resources,omitempty"`
	Message   string        `json:"message"`
}

func (BranchResizeOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchResizeOutput](nil))
	schema.Properties["status"].Description = "Branch status when the resize request was accepted. It doesn't indicate whether the resize has completed: the branch can still read READY before the resize starts."
	// The branch tools only exist behind the experimental gate, so the IOPS
	// are always included.
	setResourceInfoSchemaProperties(schema.Properties["resources"], true)
	schema.Properties["message"].Description = "Human-readable outcome of the operation"
	return schema
}

func newBranchResizeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchResize,
		Title: "Resize Branch",
		Description: `Resize a branch by changing its CPU and memory allocation or its provisioned IOPS. At least one of cpu_memory or iops is required.

The tool returns once the resize request is accepted, without waiting for the resize to
complete, which may take several minutes. The branch may be temporarily unavailable during
the resize operation.

WARNING: Creates billable resource changes. Increasing resources will increase costs.`,
		InputSchema:  BranchResizeInput{}.Schema(),
		OutputSchema: BranchResizeOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // Not destructive, just modifies resources
			IdempotentHint:  true,       // Can resize to same size multiple times
			OpenWorldHint:   new(false),
			Title:           "Resize Branch",
		},
	}
}

// handleBranchResize handles the branch_resize MCP tool
func (s *Server) handleBranchResize(ctx context.Context, req *mcp.CallToolRequest, input BranchResizeInput) (*mcp.CallToolResult, BranchResizeOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, BranchResizeOutput{}, err
	}

	if err := common.CheckReadOnly(cfg, common.BranchEnvironmentTag); err != nil {
		return nil, BranchResizeOutput{}, err
	}

	if input.CPUMemory == "" && input.IOPS == nil {
		return nil, BranchResizeOutput{}, errors.New("at least one of cpu_memory or iops is required")
	}

	resolved, err := resolveBranchByID(ctx, client, projectID, input.BranchID)
	if err != nil {
		return nil, BranchResizeOutput{}, err
	}

	// An IOPS-only resize omits CPU and memory, which the API leaves
	// unchanged.
	resizeReq := api.BranchResizeInput{Iops: input.IOPS}
	if input.CPUMemory != "" {
		cpuMemoryCfg, err := parseBranchCPUMemory(input.CPUMemory)
		if err != nil {
			return nil, BranchResizeOutput{}, err
		}
		resizeReq.CPUMillis = cpuMemoryCfg.CPUMillisString()
		resizeReq.MemoryGbs = cpuMemoryCfg.MemoryGBsString()
	}

	s.logger.Info("MCP: Resizing branch",
		slog.String("project_id", projectID),
		slog.String("branch_id", input.BranchID),
		slog.String("cpu_memory", input.CPUMemory),
		slog.Any("iops", input.IOPS),
	)

	resp, err := client.ResizeBranchWithResponse(ctx, projectID, resolved.ServiceID, resolved.ID, resizeReq)
	if err != nil {
		return nil, BranchResizeOutput{}, fmt.Errorf("failed to resize branch: %w", err)
	}
	if resp.StatusCode() != http.StatusAccepted {
		return nil, BranchResizeOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	if resp.JSON202 == nil {
		return nil, BranchResizeOutput{}, fmt.Errorf("empty response from API")
	}
	branch := *resp.JSON202

	detail := s.convertToBranchDetail(cfg, branch, false)
	return nil, BranchResizeOutput{
		Status:    detail.Status,
		Resources: detail.Resources,
		Message:   resizeInProgressMessage,
	}, nil
}
