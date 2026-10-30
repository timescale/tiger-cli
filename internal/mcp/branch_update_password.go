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

// BranchUpdatePasswordInput represents input for branch_update_password. Like
// service_update_password, the tool takes no password and always generates
// one (see ServiceUpdatePasswordInput for why).
type BranchUpdatePasswordInput struct {
	BranchID     string `json:"branch_id"`
	WithPassword bool   `json:"with_password,omitempty"`
}

func (BranchUpdatePasswordInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchUpdatePasswordInput](nil))

	setBranchIDSchemaProperties(schema)
	setWithPasswordSchemaProperties(schema)

	return schema
}

// BranchUpdatePasswordOutput represents output for branch_update_password
type BranchUpdatePasswordOutput struct {
	Message         string                        `json:"message"`
	Password        string                        `json:"password,omitempty"`
	PasswordStorage *common.PasswordStorageResult `json:"password_storage,omitempty"`
}

func (BranchUpdatePasswordOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchUpdatePasswordOutput](nil))

	schema.Properties["message"].Description = "Human-readable outcome of the operation"
	schema.Properties["password"].Description = "The new password for the tsdbadmin user (only included if with_password=true)"
	schema.Properties["password_storage"].Description = "Where the new password was saved locally, and whether saving it succeeded"

	return schema
}

func newBranchUpdatePasswordTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolBranchUpdatePassword,
		Title: "Update Branch Password",
		Description: `Update the master password for the 'tsdbadmin' user of a branch.

Changing it affects only the branch, not its parent service. The tool generates a secure random password itself; it does not accept one. The change takes effect immediately and may terminate existing connections to the branch. The new password is saved to the configured password storage and is included in the result only if with_password is true.

If the user wants to set a specific password, tell them to run 'tiger branch update-password' from the CLI instead, which keeps the password out of this conversation.`,
		InputSchema:  BranchUpdatePasswordInput{}.Schema(),
		OutputSchema: BranchUpdatePasswordOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // Replaces the credentials existing clients use
			IdempotentHint:  false,     // Each call generates a different password
			OpenWorldHint:   new(false),
			Title:           "Update Branch Password",
		},
	}
}

// handleBranchUpdatePassword handles the branch_update_password MCP tool
func (s *Server) handleBranchUpdatePassword(ctx context.Context, req *mcp.CallToolRequest, input BranchUpdatePasswordInput) (*mcp.CallToolResult, BranchUpdatePasswordOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, BranchUpdatePasswordOutput{}, err
	}

	if err := common.CheckReadOnly(cfg, common.BranchEnvironmentTag); err != nil {
		return nil, BranchUpdatePasswordOutput{}, err
	}

	// The branch is needed to save the new password under it, and its parent
	// for the API call.
	branch, err := resolveBranchByID(ctx, client, projectID, input.BranchID)
	if err != nil {
		return nil, BranchUpdatePasswordOutput{}, err
	}

	password, err := common.GenerateSecurePassword(32)
	if err != nil {
		return nil, BranchUpdatePasswordOutput{}, err
	}

	s.logger.Info("MCP: Updating branch password",
		slog.String("project_id", projectID),
		slog.String("branch_id", input.BranchID))

	resp, err := client.UpdateBranchPasswordWithResponse(ctx, projectID, branch.ServiceID, branch.ID, api.UpdatePasswordInput{Password: password})
	if err != nil {
		return nil, BranchUpdatePasswordOutput{}, fmt.Errorf("failed to update branch password: %w", err)
	}
	if resp.StatusCode() != http.StatusNoContent {
		return nil, BranchUpdatePasswordOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	storage, err := common.SavePasswordWithResult(cfg, common.BranchPasswordTarget(*branch), password, "tsdbadmin")
	if err != nil {
		s.logger.Warn("MCP: Password storage failed", slog.Any("error", err))
	}

	output := BranchUpdatePasswordOutput{
		Message:         updatePasswordMessage(input.WithPassword, storage),
		PasswordStorage: &storage,
	}
	if input.WithPassword {
		output.Password = password
	}
	return nil, output, nil
}
