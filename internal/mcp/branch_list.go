package mcp

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// BranchListInput represents input for branch_list
type BranchListInput struct {
	ServiceID string `json:"service_id"`
}

func (BranchListInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchListInput](nil))
	setServiceIDSchemaProperties(schema)
	schema.Properties["service_id"].Description = "Unique identifier of the service whose branches to list (10-character alphanumeric string). Use service_list to find service IDs."
	return schema
}

// BranchListOutput represents output for branch_list
type BranchListOutput struct {
	Branches []BranchDetail `json:"branches"`
}

func (BranchListOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchListOutput](nil))
	schema.Properties["branches"].Items = BranchDetail{}.Schema()
	return schema
}

func newBranchListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchList,
		Title: "List Branches",
		Description: "List the branches of a database service. " +
			"Returns each branch's status, resource allocation, and connection details.",
		InputSchema:  BranchListInput{}.Schema(),
		OutputSchema: BranchListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "List Branches",
		},
	}
}

// handleBranchList handles the branch_list MCP tool
func (s *Server) handleBranchList(ctx context.Context, req *mcp.CallToolRequest, input BranchListInput) (*mcp.CallToolResult, BranchListOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, BranchListOutput{}, err
	}

	listed, err := common.JSON200(client.GetBranchesWithResponse(ctx, projectID, input.ServiceID, nil))
	if err != nil {
		return nil, BranchListOutput{}, err
	}

	branches := make([]BranchDetail, len(listed))
	for i, branch := range listed {
		branches[i] = s.convertToBranchDetail(cfg, branch, false)
	}

	return nil, BranchListOutput{Branches: branches}, nil
}
