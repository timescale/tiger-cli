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

// AllowListDeleteInput represents input for allowlist_delete
type AllowListDeleteInput struct {
	AllowListID string `json:"allow_list_id"`
}

func (AllowListDeleteInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[AllowListDeleteInput](nil))
	schema.Properties["allow_list_id"].Description = "Unique identifier of the IP allow list. Use allowlist_list to find IP allow list IDs."
	schema.Properties["allow_list_id"].Examples = []any{"1234567890"}
	return schema
}

// AllowListDeleteOutput represents output for allowlist_delete
type AllowListDeleteOutput struct {
	Message string `json:"message"`
}

func (AllowListDeleteOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[AllowListDeleteOutput](nil))
}

func newAllowListDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolAllowListDelete,
		Title: "Delete IP Allow List",
		Description: `Delete an IP allow list.

This operation is irreversible. It is refused if the IP allow list is still attached to any service — detach it from every service first.`,
		InputSchema:  AllowListDeleteInput{}.Schema(),
		OutputSchema: AllowListDeleteOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
			Title:           "Delete IP Allow List",
		},
	}
}

// handleAllowListDelete handles the allowlist_delete MCP tool
func (s *Server) handleAllowListDelete(ctx context.Context, req *mcp.CallToolRequest, input AllowListDeleteInput) (*mcp.CallToolResult, AllowListDeleteOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, AllowListDeleteOutput{}, err
	}

	if cfg.ReadOnly.BlocksAll() {
		return nil, AllowListDeleteOutput{}, common.ErrReadOnly
	}

	s.logger.Info("MCP: Deleting IP allow list",
		slog.String("project_id", projectID),
		slog.String("allow_list_id", input.AllowListID),
	)

	resp, err := client.DeleteAllowListWithResponse(ctx, projectID, input.AllowListID)
	if err != nil {
		return nil, AllowListDeleteOutput{}, fmt.Errorf("failed to delete IP allow list: %w", err)
	}

	if resp.StatusCode() != http.StatusNoContent {
		return nil, AllowListDeleteOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	return nil, AllowListDeleteOutput{
		Message: fmt.Sprintf("IP allow list '%s' deleted.", input.AllowListID),
	}, nil
}
