package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// BranchResizeInput represents input for branch_resize
type BranchResizeInput struct {
	BranchID  string `json:"branch_id"`
	CPUMemory string `json:"cpu_memory"`
	Wait      bool   `json:"wait,omitempty"`
}

func (BranchResizeInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchResizeInput](nil))

	setBranchIDSchemaProperties(schema)
	setBranchCPUMemorySchemaProperties(schema, "CPU and memory allocation combination.")

	schema.Properties["wait"].Description = "Whether to wait for the branch to be done resizing before returning. Default is false (recommended). Only set to true if your next steps require connecting to or querying the branch. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

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
	schema.Properties["status"].Description = "Current branch status after resize operation"
	setResourceInfoSchemaProperties(schema.Properties["resources"])
	schema.Properties["message"].Description = "Human-readable outcome of the operation"
	return schema
}

func newBranchResizeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchResize,
		Title: "Resize Branch",
		Description: `Resize a branch by changing its CPU and memory allocation.

A branch's storage is shared with its parent service and isn't resized. The branch may be temporarily unavailable during the resize operation.

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

	resolved, err := resolveBranchByID(ctx, client, projectID, input.BranchID)
	if err != nil {
		return nil, BranchResizeOutput{}, err
	}

	cpuMemoryCfg, err := parseBranchCPUMemory(input.CPUMemory)
	if err != nil {
		return nil, BranchResizeOutput{}, err
	}

	s.logger.Info("MCP: Resizing branch",
		slog.String("project_id", projectID),
		slog.String("branch_id", input.BranchID),
		slog.String("cpu_memory", input.CPUMemory),
	)

	resp, err := client.ResizeBranchWithResponse(ctx, projectID, resolved.ServiceID, resolved.ID, api.BranchResizeInput{
		CPUMillis: strconv.Itoa(cpuMemoryCfg.CPUMillis),
		MemoryGbs: strconv.Itoa(cpuMemoryCfg.MemoryGBs),
	})
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

	message := "Resize request accepted. The branch may still be resizing."
	if input.Wait {
		if err := common.WaitForBranch(ctx, common.WaitForBranchArgs{
			Client:       client,
			ProjectID:    projectID,
			ServiceID:    resolved.ServiceID,
			BranchID:     resolved.ID,
			Branch:       &branch,
			TargetStatus: api.DeployStatusREADY,
			Timeout:      waitTimeout,
			TimeoutMsg:   "branch may still be resizing",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Branch resized."
		}
	}

	detail := s.convertToBranchDetail(cfg, branch)
	return nil, BranchResizeOutput{
		Status:    detail.Status,
		Resources: detail.Resources,
		Message:   message,
	}, nil
}
