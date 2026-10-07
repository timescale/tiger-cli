package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// AllowListCreateInput represents input for allowlist_create
type AllowListCreateInput struct {
	Description string   `json:"description"`
	CidrBlocks  []string `json:"cidr_blocks"`
}

func (AllowListCreateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[AllowListCreateInput](nil))
	schema.Properties["description"].Description = "Human-readable label for the IP allow list."
	schema.Properties["description"].Examples = []any{"Office network"}
	schema.Properties["cidr_blocks"].Description = "The IP ranges to permit. Each block must be a public range no smaller than a /17; the count is capped by your plan."
	schema.Properties["cidr_blocks"].Examples = []any{[]string{"203.0.113.0/24"}}
	return schema
}

func newAllowListCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolAllowListCreate,
		Title: "Create IP Allow List",
		Description: `Create an IP allow list that restricts which IP ranges can reach a service.

An IP allow list carries no effect until it is attached to a service with service_allowlist_attach. The number of CIDR blocks per list and the number of lists per project are plan-dependent.`,
		InputSchema:  AllowListCreateInput{}.Schema(),
		OutputSchema: AllowListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false,
			OpenWorldHint:   new(false),
			Title:           "Create IP Allow List",
		},
	}
}

// handleAllowListCreate handles the allowlist_create MCP tool
func (s *Server) handleAllowListCreate(ctx context.Context, req *mcp.CallToolRequest, input AllowListCreateInput) (*mcp.CallToolResult, AllowListOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, AllowListOutput{}, err
	}

	if cfg.ReadOnly.BlocksAll() {
		return nil, AllowListOutput{}, common.ErrReadOnly
	}

	s.logger.Info("MCP: Creating IP allow list", slog.String("project_id", projectID))

	resp, err := client.CreateAllowListWithResponse(ctx, projectID, api.AllowListCreate{
		Description: input.Description,
		CidrBlocks:  input.CidrBlocks,
	})
	if err != nil {
		return nil, AllowListOutput{}, fmt.Errorf("failed to create IP allow list: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, AllowListOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON201 == nil {
		return nil, AllowListOutput{}, fmt.Errorf("empty response from API")
	}

	return nil, allowListOutputFor(*resp.JSON201), nil
}
