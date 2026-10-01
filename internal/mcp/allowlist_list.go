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

// AllowListListInput represents input for allowlist_list
type AllowListListInput struct{}

func (AllowListListInput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[AllowListListInput](nil))
}

// AllowListListOutput represents output for allowlist_list
type AllowListListOutput struct {
	AllowLists []AllowListOutput `json:"allow_lists"`
}

func (AllowListListOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[AllowListListOutput](nil))
}

func newAllowListListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolAllowListList,
		Title: "List IP Allow Lists",
		Description: "List every IP allow list in the current project. " +
			"An IP allow list restricts which IP ranges can reach a service.",
		InputSchema:  AllowListListInput{}.Schema(),
		OutputSchema: AllowListListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "List IP Allow Lists",
		},
	}
}

// handleAllowListList handles the allowlist_list MCP tool
func (s *Server) handleAllowListList(ctx context.Context, req *mcp.CallToolRequest, input AllowListListInput) (*mcp.CallToolResult, AllowListListOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, AllowListListOutput{}, err
	}

	s.logger.Info("MCP: Listing IP allow lists", slog.String("project_id", projectID))

	resp, err := client.GetAllowListsWithResponse(ctx, projectID)
	if err != nil {
		return nil, AllowListListOutput{}, fmt.Errorf("failed to list IP allow lists: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, AllowListListOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, AllowListListOutput{AllowLists: []AllowListOutput{}}, nil
	}

	outputs := make([]AllowListOutput, len(*resp.JSON200))
	for i, allowList := range *resp.JSON200 {
		outputs[i] = allowListOutputFor(allowList)
	}

	return nil, AllowListListOutput{AllowLists: outputs}, nil
}
