package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// BranchStartInput represents input for branch_start
type BranchStartInput struct {
	BranchID string `json:"branch_id"`
	Wait     bool   `json:"wait,omitempty"`
}

func (BranchStartInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchStartInput](nil))

	setBranchIDSchemaProperties(schema)

	schema.Properties["wait"].Description = "Whether to wait for the branch to be fully started before returning. Default is false (recommended). Only set to true if your next steps require connecting to or querying the branch. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	return schema
}

// BranchStartOutput represents output for branch_start
type BranchStartOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (BranchStartOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchStartOutput](nil))
	schema.Properties["status"].Description = "Current branch status after start operation"
	schema.Properties["message"].Description = "Human-readable outcome of the operation"
	return schema
}

func newBranchStartTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchStart,
		Title: "Start Branch",
		Description: `Start a stopped branch.

Starting a branch resumes charging for its compute.`,
		InputSchema:  BranchStartInput{}.Schema(),
		OutputSchema: BranchStartOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // Starting a branch doesn't modify or destroy anything
			IdempotentHint:  true,       // Starting an already-started branch is safe (but returns an error)
			OpenWorldHint:   new(false),
			Title:           "Start Branch",
		},
	}
}

// handleBranchStart handles the branch_start MCP tool
func (s *Server) handleBranchStart(ctx context.Context, req *mcp.CallToolRequest, input BranchStartInput) (*mcp.CallToolResult, BranchStartOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, BranchStartOutput{}, err
	}

	if err := common.CheckReadOnly(cfg, common.BranchEnvironmentTag); err != nil {
		return nil, BranchStartOutput{}, err
	}

	resolved, err := resolveBranchByID(ctx, client, projectID, input.BranchID)
	if err != nil {
		return nil, BranchStartOutput{}, err
	}

	s.logger.Info("MCP: Starting branch",
		slog.String("project_id", projectID),
		slog.String("branch_id", input.BranchID))

	resp, err := client.StartBranchWithResponse(ctx, projectID, resolved.ServiceID, resolved.ID)
	if err != nil {
		return nil, BranchStartOutput{}, fmt.Errorf("failed to start branch: %w", err)
	}
	if resp.StatusCode() != http.StatusAccepted {
		return nil, BranchStartOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	if resp.JSON202 == nil {
		return nil, BranchStartOutput{}, fmt.Errorf("empty response from API")
	}
	branch := *resp.JSON202

	message := "Branch start request accepted. The branch may still be starting."
	if input.Wait {
		if err := common.WaitForBranch(ctx, common.WaitForBranchArgs{
			Client:       client,
			ProjectID:    projectID,
			ServiceID:    resolved.ServiceID,
			BranchID:     resolved.ID,
			Branch:       &branch,
			TargetStatus: api.DeployStatusREADY,
			Timeout:      waitTimeout,
			TimeoutMsg:   "branch may still be starting",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Branch started."
		}
	}

	return nil, BranchStartOutput{
		Status:  string(branch.Status),
		Message: message,
	}, nil
}
