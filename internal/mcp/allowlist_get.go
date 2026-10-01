package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// AllowListGetInput represents input for allowlist_get
type AllowListGetInput struct {
	AllowListID string `json:"allow_list_id"`
}

func (AllowListGetInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[AllowListGetInput](nil))
	schema.Properties["allow_list_id"].Description = "Unique identifier of the IP allow list. Use allowlist_list to find IP allow list IDs."
	schema.Properties["allow_list_id"].Examples = []any{"1234567890"}
	return schema
}

// AllowListOutput represents an IP allow list, shared by allowlist_get and
// allowlist_list.
type AllowListOutput struct {
	AllowListID string    `json:"allow_list_id"`
	ProjectID   string    `json:"project_id"`
	Description string    `json:"description"`
	CidrBlocks  []string  `json:"cidr_blocks"`
	CreatedAt   time.Time `json:"created_at"`
}

func (AllowListOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[AllowListOutput](nil))
}

func allowListOutputFor(allowList api.AllowList) AllowListOutput {
	return AllowListOutput{
		AllowListID: allowList.AllowListID,
		ProjectID:   allowList.ProjectID,
		Description: allowList.Description,
		CidrBlocks:  allowList.CidrBlocks,
		CreatedAt:   allowList.CreatedAt,
	}
}

func newAllowListGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolAllowListGet,
		Title: "Get IP Allow List",
		Description: "Get the details of a single IP allow list. " +
			"Use allowlist_list to find IP allow list IDs.",
		InputSchema:  AllowListGetInput{}.Schema(),
		OutputSchema: AllowListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
			Title:         "Get IP Allow List",
		},
	}
}

// handleAllowListGet handles the allowlist_get MCP tool
func (s *Server) handleAllowListGet(ctx context.Context, req *mcp.CallToolRequest, input AllowListGetInput) (*mcp.CallToolResult, AllowListOutput, error) {
	client, projectID, err := s.app.GetClient()
	if err != nil {
		return nil, AllowListOutput{}, err
	}

	s.logger.Info("MCP: Getting IP allow list",
		slog.String("project_id", projectID),
		slog.String("allow_list_id", input.AllowListID),
	)

	resp, err := client.GetAllowListWithResponse(ctx, projectID, input.AllowListID)
	if err != nil {
		return nil, AllowListOutput{}, fmt.Errorf("failed to get IP allow list: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, AllowListOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON200 == nil {
		return nil, AllowListOutput{}, fmt.Errorf("empty response from API")
	}

	return nil, allowListOutputFor(*resp.JSON200), nil
}
