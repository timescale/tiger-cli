package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// BranchStopInput represents input for branch_stop
type BranchStopInput struct {
	BranchID string `json:"branch_id"`
	Wait     bool   `json:"wait,omitempty"`
}

func (BranchStopInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchStopInput](nil))

	setBranchIDSchemaProperties(schema)

	schema.Properties["wait"].Description = "Whether to wait for the branch to be fully stopped before returning. Default is false (recommended). Only set to true if your next steps require confirmation that the branch is stopped. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	return schema
}

// BranchStopOutput represents output for branch_stop
type BranchStopOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (BranchStopOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchStopOutput](nil))
	schema.Properties["status"].Description = "Current branch status after stop operation"
	schema.Properties["message"].Description = "Human-readable outcome of the operation"
	return schema
}

func newBranchStopTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchStop,
		Title: "Stop Branch",
		Description: `Stop a running branch.

This operation stops a branch that is currently running. The branch will transition to a stopped/paused state and will no longer accept connections. It stops charging for compute, but keeps the data changes made on it, whose storage is still billed.`,
		InputSchema:  BranchStopInput{}.Schema(),
		OutputSchema: BranchStopOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // Stopping a branch breaks existing connections
			IdempotentHint:  true,      // Stopping an already-stopped branch is safe (but returns an error)
			OpenWorldHint:   new(false),
			Title:           "Stop Branch",
		},
	}
}

// handleBranchStop handles the branch_stop MCP tool
func (s *Server) handleBranchStop(ctx context.Context, req *mcp.CallToolRequest, input BranchStopInput) (*mcp.CallToolResult, BranchStopOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, BranchStopOutput{}, err
	}

	if err := common.CheckReadOnly(cfg, common.BranchEnvironmentTag); err != nil {
		return nil, BranchStopOutput{}, err
	}

	resolved, err := resolveBranchByID(ctx, client, projectID, input.BranchID)
	if err != nil {
		return nil, BranchStopOutput{}, err
	}

	branch, err := common.JSON202(client.StopBranchWithResponse(ctx, projectID, resolved.ServiceID, resolved.ID))
	if err != nil {
		return nil, BranchStopOutput{}, err
	}

	message := "Branch stop request accepted. The branch may still be stopping."
	if input.Wait {
		if err := common.WaitForBranch(ctx, common.WaitForBranchArgs{
			Client:       client,
			ProjectID:    projectID,
			ServiceID:    resolved.ServiceID,
			BranchID:     resolved.ID,
			Branch:       &branch,
			TargetStatus: api.DeployStatusPAUSED,
			Timeout:      waitTimeout,
			TimeoutMsg:   "branch may still be stopping",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Branch stopped."
		}
	}

	return nil, BranchStopOutput{
		Status:  string(branch.Status),
		Message: message,
	}, nil
}
