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

// BranchDeleteInput represents input for branch_delete
type BranchDeleteInput struct {
	BranchID string `json:"branch_id"`
}

func (BranchDeleteInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchDeleteInput](nil))
	setBranchIDSchemaProperties(schema)
	return schema
}

// BranchDeleteOutput represents output for branch_delete
type BranchDeleteOutput struct {
	BranchID string `json:"branch_id"`
	Message  string `json:"message"`
}

func (BranchDeleteOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchDeleteOutput](nil))
	schema.Properties["branch_id"].Description = "Identifier of the deleted branch"
	schema.Properties["message"].Description = "Human-readable outcome of the operation"
	return schema
}

func newBranchDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchDelete,
		Title: "Delete Branch",
		Description: `Delete a branch permanently, along with any changes made on it. The parent service is not affected.

This operation is irreversible. Branches are always treated as DEV, so the tool does not prompt the user for confirmation the way deleting a PROD service does.`,
		InputSchema:  BranchDeleteInput{}.Schema(),
		OutputSchema: BranchDeleteOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // Permanently destroys the branch and the changes made on it
			IdempotentHint:  true,      // Deleting an already-deleted branch has no further effect (but returns an error)
			OpenWorldHint:   new(false),
			Title:           "Delete Branch",
		},
	}
}

// handleBranchDelete handles the branch_delete MCP tool
func (s *Server) handleBranchDelete(ctx context.Context, req *mcp.CallToolRequest, input BranchDeleteInput) (*mcp.CallToolResult, BranchDeleteOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, BranchDeleteOutput{}, err
	}

	if err := common.CheckReadOnly(cfg, common.BranchEnvironmentTag); err != nil {
		return nil, BranchDeleteOutput{}, err
	}

	resolved, err := resolveBranchByID(ctx, client, projectID, input.BranchID)
	if err != nil {
		return nil, BranchDeleteOutput{}, err
	}

	s.logger.Info("MCP: Deleting branch",
		slog.String("project_id", projectID),
		slog.String("branch_id", input.BranchID))

	resp, err := client.DeleteBranchWithResponse(ctx, projectID, resolved.ServiceID, resolved.ID)
	if err != nil {
		return nil, BranchDeleteOutput{}, fmt.Errorf("failed to delete branch: %w", err)
	}
	if resp.StatusCode() != http.StatusAccepted {
		return nil, BranchDeleteOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	return nil, BranchDeleteOutput{
		BranchID: input.BranchID,
		Message:  fmt.Sprintf("Branch %q has been deleted.", input.BranchID),
	}, nil
}
