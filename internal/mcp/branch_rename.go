package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// BranchRenameInput represents input for branch_rename
type BranchRenameInput struct {
	BranchID string `json:"branch_id"`
	Name     string `json:"name"`
}

func (BranchRenameInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchRenameInput](nil))

	setBranchIDSchemaProperties(schema)

	schema.Properties["name"].Description = "The new name for the branch, unique within its parent service."
	schema.Properties["name"].Examples = []any{"feature/rbac", "ci-4821"}

	return schema
}

// BranchRenameOutput represents output for branch_rename
type BranchRenameOutput struct {
	Message  string `json:"message"`
	BranchID string `json:"branch_id"`
	Name     string `json:"name"`
}

func (BranchRenameOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchRenameOutput](nil))
	schema.Properties["message"].Description = "Human-readable result of the rename operation"
	schema.Properties["branch_id"].Description = "Unique identifier of the renamed branch, unchanged by the rename"
	schema.Properties["name"].Description = "Name of the branch as stored after the rename"
	return schema
}

func newBranchRenameTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchRename,
		Title: "Rename Branch",
		Description: "Rename a branch. " +
			"Only the name changes: the branch ID, endpoint, and data are untouched, so existing connections and connection strings keep working.",
		InputSchema:  BranchRenameInput{}.Schema(),
		OutputSchema: BranchRenameOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // A rename loses nothing and can be reversed by renaming back
			IdempotentHint:  true,       // Renaming to the name a branch already has changes nothing
			OpenWorldHint:   new(false),
			Title:           "Rename Branch",
		},
	}
}

// handleBranchRename handles the branch_rename MCP tool
func (s *Server) handleBranchRename(ctx context.Context, req *mcp.CallToolRequest, input BranchRenameInput) (*mcp.CallToolResult, BranchRenameOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, BranchRenameOutput{}, err
	}

	if err := common.CheckReadOnly(cfg, common.BranchEnvironmentTag); err != nil {
		return nil, BranchRenameOutput{}, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, BranchRenameOutput{}, errors.New("new name cannot be empty")
	}

	s.logger.Info("MCP: Renaming branch",
		slog.String("project_id", projectID),
		slog.String("branch_id", input.BranchID))

	resp, err := client.RenameBranchWithResponse(ctx, projectID, input.BranchID, api.BranchRename{Name: name})
	if err != nil {
		return nil, BranchRenameOutput{}, fmt.Errorf("failed to rename branch: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, BranchRenameOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}
	if resp.JSON200 == nil {
		return nil, BranchRenameOutput{}, errors.New("empty response from API")
	}
	renamed := *resp.JSON200

	return nil, BranchRenameOutput{
		Message:  fmt.Sprintf("Branch %q renamed to %q.", renamed.ID, renamed.Name),
		BranchID: renamed.ID,
		Name:     renamed.Name,
	}, nil
}
