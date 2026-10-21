package mcp

import (
	"context"
	"log/slog"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// BranchGetInput represents input for branch_get
type BranchGetInput struct {
	BranchID string `json:"branch_id"`
}

func (BranchGetInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchGetInput](nil))
	setBranchIDSchemaProperties(schema)
	return schema
}

// BranchGetOutput represents output for branch_get
type BranchGetOutput struct {
	Branch BranchDetail `json:"branch"`
}

func (BranchGetOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchGetOutput](nil))
	schema.Properties["branch"] = BranchDetail{}.Schema()
	return schema
}

func newBranchGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchGet,
		Title: "Get Branch Details",
		Description: "Get detailed information for a specific branch of a database service. " +
			"Returns its parent service, status, resource allocation, creation time, and connection details.",
		InputSchema:  BranchGetInput{}.Schema(),
		OutputSchema: BranchGetOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "Get Branch Details",
		},
	}
}

// handleBranchGet handles the branch_get MCP tool
func (s *Server) handleBranchGet(ctx context.Context, req *mcp.CallToolRequest, input BranchGetInput) (*mcp.CallToolResult, BranchGetOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, BranchGetOutput{}, err
	}

	s.logger.Info("MCP: Getting branch details",
		slog.String("project_id", projectID),
		slog.String("branch_id", input.BranchID))

	branch, err := resolveBranchByID(ctx, client, projectID, input.BranchID)
	if err != nil {
		return nil, BranchGetOutput{}, err
	}

	detail := s.convertToBranchDetail(cfg, *branch)

	// The branch carries only its parent's ID, so the name takes a second
	// fetch. The rest of the output doesn't depend on it.
	if parent, err := common.GetService(ctx, client, projectID, branch.ServiceID); err != nil {
		s.logger.Error("MCP: Failed to get parent service", slog.Any("error", err))
	} else {
		detail.ServiceName = parent.Name
	}

	return nil, BranchGetOutput{Branch: detail}, nil
}
