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

// AllowListUpdateInput represents input for allowlist_update. Description and
// CidrBlocks are both optional, but at least one is required.
type AllowListUpdateInput struct {
	AllowListID string    `json:"allow_list_id"`
	Description *string   `json:"description,omitempty"`
	CidrBlocks  *[]string `json:"cidr_blocks,omitempty"`
}

func (AllowListUpdateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[AllowListUpdateInput](nil))
	schema.Properties["allow_list_id"].Description = "Unique identifier of the IP allow list. Use allowlist_list to find IP allow list IDs."
	schema.Properties["allow_list_id"].Examples = []any{"1234567890"}
	schema.Properties["description"].Description = "New human-readable label for the IP allow list. Omit to leave it unchanged."
	schema.Properties["description"].Examples = []any{"Office network"}
	schema.Properties["cidr_blocks"].Description = "Replacement CIDR blocks to permit, replacing the current set. Omit to leave them unchanged."
	schema.Properties["cidr_blocks"].Examples = []any{[]string{"203.0.113.0/24"}}
	return schema
}

func newAllowListUpdateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolAllowListUpdate,
		Title: "Update IP Allow List",
		Description: `Update an IP allow list in place.

Send description and/or cidr_blocks to change them; at least one is required. Omitting either one leaves it unchanged. cidr_blocks replaces the whole set of CIDR blocks, not just the ones changing.

Every service the IP allow list is attached to picks up the new CIDR blocks; there is no need to detach and reattach.`,
		InputSchema:  AllowListUpdateInput{}.Schema(),
		OutputSchema: AllowListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
			Title:           "Update IP Allow List",
		},
	}
}

// handleAllowListUpdate handles the allowlist_update MCP tool
func (s *Server) handleAllowListUpdate(ctx context.Context, req *mcp.CallToolRequest, input AllowListUpdateInput) (*mcp.CallToolResult, AllowListOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, AllowListOutput{}, err
	}

	if cfg.ReadOnly.BlocksAll() {
		return nil, AllowListOutput{}, common.ErrReadOnly
	}

	if input.Description == nil && input.CidrBlocks == nil {
		return nil, AllowListOutput{}, errors.New("at least one of description or cidr_blocks is required")
	}

	s.logger.Info("MCP: Updating IP allow list",
		slog.String("project_id", projectID),
		slog.String("allow_list_id", input.AllowListID),
	)

	resp, err := client.UpdateAllowListWithResponse(ctx, projectID, input.AllowListID, api.AllowListUpdate{
		Description: input.Description,
		CidrBlocks:  input.CidrBlocks,
	})
	if err != nil {
		return nil, AllowListOutput{}, fmt.Errorf("failed to update IP allow list: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, AllowListOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, AllowListOutput{}, fmt.Errorf("empty response from API")
	}

	return nil, allowListOutputFor(*resp.JSON200), nil
}
