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

// updatePasswordConfirmationKey is the InputRequests/InputResponses key of the
// PROD password update prompt.
const updatePasswordConfirmationKey = "confirm_update_password"

// ServiceUpdatePasswordInput represents input for service_update_password.
//
// Unlike `tiger service update-password`, the tool takes no password. One
// passed as an argument is invented by the model or typed into the chat by the
// user, so it lands in the model's context. Asking for it through an
// elicitation form would keep it out of the context, but not out of session
// transcripts or the MCP client, which can't be trusted with it, and the MCP
// spec says servers MUST NOT request passwords through form elicitation. So
// the tool always generates the password, and the CLI remains the way to set
// a specific one.
type ServiceUpdatePasswordInput struct {
	ServiceID    string `json:"service_id"`
	WithPassword bool   `json:"with_password,omitempty"`
}

func (ServiceUpdatePasswordInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceUpdatePasswordInput](nil))

	setServiceIDSchemaProperties(schema)
	setWithPasswordSchemaProperties(schema)

	return schema
}

// ServiceUpdatePasswordOutput represents output for service_update_password
type ServiceUpdatePasswordOutput struct {
	Updated         bool                          `json:"updated"`
	Message         string                        `json:"message"`
	Password        string                        `json:"password,omitempty"`
	PasswordStorage *common.PasswordStorageResult `json:"password_storage,omitempty"`
}

func (ServiceUpdatePasswordOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceUpdatePasswordOutput](nil))

	schema.Properties["updated"].Description = "Whether the password was updated. False when the user declined the confirmation prompt for a PROD service; do not retry unless the user asks again."
	schema.Properties["message"].Description = "Human-readable outcome of the operation"
	schema.Properties["password"].Description = "The new password for the tsdbadmin user (only included if with_password=true)"
	schema.Properties["password_storage"].Description = "Where the new password was saved locally, and whether saving it succeeded"

	return schema
}

func newServiceUpdatePasswordTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceUpdatePassword,
		Title: "Update Service Password",
		Description: `Update the master password for the 'tsdbadmin' user of a database service.

The tool generates a secure random password itself; it does not accept one. The change takes effect immediately and may terminate existing connections, and applications using the old password fail to authenticate until they are updated. The new password is saved to the configured password storage and is included in the result only if with_password is true.

If the user wants to set a specific password, tell them to run 'tiger service update-password' from the CLI instead, which keeps the password out of this conversation.

Updating the password of a service tagged PROD automatically prompts the user, via an elicitation request through the MCP client, to confirm the update before it proceeds, so agents don't need to ask the user for confirmation themselves; if the client cannot prompt, the update is refused and the user must run 'tiger service update-password' from the CLI instead.`,
		InputSchema:  ServiceUpdatePasswordInput{}.Schema(),
		OutputSchema: ServiceUpdatePasswordOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // Replaces the credentials existing clients use
			IdempotentHint:  false,     // Each call generates a different password
			OpenWorldHint:   new(false),
			Title:           "Update Service Password",
		},
	}
}

// handleServiceUpdatePassword handles the service_update_password MCP tool.
//
// Updating the password of a PROD service is a multi round-trip call, like
// service_delete: the first invocation returns an elicitation asking the user
// to confirm, and the SDK re-invokes the handler with the answer in
// InputResponses. The password is generated only on the confirmed run.
func (s *Server) handleServiceUpdatePassword(ctx context.Context, req *mcp.CallToolRequest, input ServiceUpdatePasswordInput) (*mcp.CallToolResult, ServiceUpdatePasswordOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	// Refuse without an API call under read_only=all. prod needs the tag, so the
	// real gate waits for the fetch below.
	if cfg.ReadOnly.BlocksAll() {
		return nil, ServiceUpdatePasswordOutput{}, common.ErrReadOnly
	}

	// The service's tag decides both the prod half of the read-only gate and
	// whether the user has to confirm, and the service is reused for password
	// storage below.
	service, err := common.GetService(ctx, client, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	tag := common.ServiceEnvironmentTag(*service)
	if err := common.CheckReadOnly(cfg, tag); err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	if common.IsReadReplica(*service) {
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("%q is a read replica; update the password on its primary service %q instead",
			input.ServiceID, util.DerefStr(service.ForkedFrom.ServiceID))
	}

	if tag == api.EnvironmentTagPROD {
		answer, answered := req.Params.InputResponses[updatePasswordConfirmationKey]
		if !answered {
			result, err := promptProdUpdatePassword(req, *service)
			return result, ServiceUpdatePasswordOutput{}, err
		}
		if !serviceIDConfirmed(answer, input.ServiceID) {
			return nil, ServiceUpdatePasswordOutput{
				Updated: false,
				Message: fmt.Sprintf("Password update cancelled: the user did not confirm updating the password of PROD service %q by typing its ID.", input.ServiceID),
			}, nil
		}
	}

	password, err := common.GenerateSecurePassword(32)
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	s.logger.Info("MCP: Updating service password",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID))

	resp, err := client.UpdatePasswordWithResponse(ctx, projectID, input.ServiceID, api.UpdatePasswordInput{Password: password})
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("failed to update service password: %w", err)
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
		return nil, ServiceUpdatePasswordOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	storage, err := common.SavePasswordWithResult(cfg, *service, password, "tsdbadmin")
	if err != nil {
		s.logger.Warn("MCP: Password storage failed", slog.Any("error", err))
	} else {
		s.logger.Info("MCP: Password saved successfully", slog.String("method", storage.Method))
	}

	output := ServiceUpdatePasswordOutput{
		Updated:         true,
		Message:         updatePasswordMessage(input, storage),
		PasswordStorage: &storage,
	}
	if input.WithPassword {
		output.Password = password
	}
	return nil, output, nil
}

// promptProdUpdatePassword returns the input-required result that asks the
// user to confirm updating the password of a PROD service, refusing outright
// when the client can't show the prompt.
func promptProdUpdatePassword(req *mcp.CallToolRequest, service api.Service) (*mcp.CallToolResult, error) {
	if !clientSupportsFormElicitation(req) {
		return nil, fmt.Errorf("updating the password of service %s requires the user's confirmation because it is tagged PROD, but this MCP client does not support elicitation; ask the user to run 'tiger service update-password %s' instead", service.ServiceID, service.ServiceID)
	}
	return serviceIDConfirmationRequest(
		updatePasswordConfirmationKey,
		fmt.Sprintf("Update the password of PRODUCTION service %q (%s)? The tsdbadmin password is replaced with a newly generated one: existing connections may be terminated, and applications using the old password will fail to authenticate.", service.Name, service.ServiceID),
		service.ServiceID,
	), nil
}

// updatePasswordMessage returns the result message for an update that went
// through, given the outcome of saving the new password. It warns when the
// password was neither saved (because saving failed or password storage is
// disabled) nor returned, since then nobody knows it.
func updatePasswordMessage(input ServiceUpdatePasswordInput, storage common.PasswordStorageResult) string {
	if !storage.Success && !input.WithPassword {
		return "Password updated for tsdbadmin. Warning: the new password was not saved, and was not returned because with_password was false."
	}
	return "Password updated for tsdbadmin."
}
