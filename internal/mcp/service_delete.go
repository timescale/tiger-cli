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

// deleteConfirmationKey is the InputRequests/InputResponses key of the PROD
// deletion prompt, and deleteConfirmationField the field in it where the user
// types the service ID back.
const (
	deleteConfirmationKey   = "confirm_delete"
	deleteConfirmationField = "service_id"
)

// ServiceDeleteInput represents input for service_delete
type ServiceDeleteInput struct {
	ServiceID string `json:"service_id"`
}

func (ServiceDeleteInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceDeleteInput](nil))

	setServiceIDSchemaProperties(schema)

	return schema
}

// ServiceDeleteOutput represents output for service_delete
type ServiceDeleteOutput struct {
	ServiceID string `json:"service_id"`
	Deleted   bool   `json:"deleted"`
	Message   string `json:"message"`
}

func (ServiceDeleteOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceDeleteOutput](nil))

	schema.Properties["service_id"].Description = "Identifier of the service the deletion targeted"
	schema.Properties["deleted"].Description = "Whether the service was deleted. False when the user declined the confirmation prompt for a PROD service; do not retry unless the user asks again."
	schema.Properties["message"].Description = "Human-readable outcome of the operation"

	return schema
}

func newServiceDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceDelete,
		Title: "Delete Database Service",
		Description: `Delete a database service permanently.

This operation is irreversible: the service and all of its data are destroyed. Deleting a service tagged PROD automatically prompts the user, via an elicitation request through the MCP client, to confirm the deletion before it proceeds, so agents don't need to ask the user for confirmation themselves; if the client cannot prompt, the deletion is refused and the user must run 'tiger service delete' from the CLI instead.`,
		InputSchema:  ServiceDeleteInput{}.Schema(),
		OutputSchema: ServiceDeleteOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // Permanently destroys the service and its data
			IdempotentHint:  true,      // Deleting an already-deleted service has no further effect (but returns an error)
			OpenWorldHint:   new(false),
			Title:           "Delete Database Service",
		},
	}
}

// handleServiceDelete handles the service_delete MCP tool.
//
// Deleting a PROD service is a multi round-trip call: the first invocation
// returns an elicitation asking the user to confirm, and the SDK re-invokes the
// handler with the answer in InputResponses — client-side on protocol 2026-07-28
// and later, server-side (via ServerSession.Elicit) for older clients — so the
// handler never calls Elicit itself.
func (s *Server) handleServiceDelete(ctx context.Context, req *mcp.CallToolRequest, input ServiceDeleteInput) (*mcp.CallToolResult, ServiceDeleteOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceDeleteOutput{}, err
	}

	// Refuse without an API call under read_only=all. prod needs the tag, so the
	// real gate waits for the fetch below.
	if cfg.ReadOnly.BlocksAll() {
		return nil, ServiceDeleteOutput{}, common.ErrReadOnly
	}

	// The service's tag decides both the prod half of the read-only gate and
	// whether the user has to confirm, so fetch it unconditionally.
	service, err := common.GetService(ctx, client, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceDeleteOutput{}, err
	}

	tag := common.ServiceEnvironmentTag(*service)
	if err := common.CheckReadOnly(cfg, tag); err != nil {
		return nil, ServiceDeleteOutput{}, err
	}

	if tag == api.EnvironmentTagPROD {
		answer, answered := req.Params.InputResponses[deleteConfirmationKey]
		if !answered {
			result, err := promptProdDelete(req, *service)
			return result, ServiceDeleteOutput{}, err
		}
		if !deleteConfirmed(answer, input.ServiceID) {
			return nil, ServiceDeleteOutput{
				ServiceID: input.ServiceID,
				Deleted:   false,
				Message:   fmt.Sprintf("Deletion cancelled: the user did not confirm deleting PROD service %q by typing its ID.", input.ServiceID),
			}, nil
		}
	}

	s.logger.Info("MCP: Deleting service",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID))

	resp, err := client.DeleteServiceWithResponse(ctx, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceDeleteOutput{}, fmt.Errorf("failed to delete service: %w", err)
	}
	if resp.StatusCode() != http.StatusAccepted {
		return nil, ServiceDeleteOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	return nil, ServiceDeleteOutput{
		ServiceID: input.ServiceID,
		Deleted:   true,
		Message:   fmt.Sprintf("Service %q has been deleted.", input.ServiceID),
	}, nil
}

// promptProdDelete returns the input-required result that asks the user to
// confirm deleting a PROD service. It refuses outright when the client can't
// show the prompt: silently deleting a production service without a human in
// the loop is the one outcome this tool must never produce.
func promptProdDelete(req *mcp.CallToolRequest, service api.Service) (*mcp.CallToolResult, error) {
	if !clientSupportsFormElicitation(req) {
		return nil, fmt.Errorf("deleting service %s requires the user's confirmation because it is tagged PROD, but this MCP client does not support elicitation; run 'tiger service delete %s' from the CLI instead", service.ServiceID, service.ServiceID)
	}

	// The user types the ID back, as `tiger service delete` requires. Clients
	// focus their accept control by default, so a bare accept/decline form
	// would let a stray Enter delete the service; a mistyped or empty ID
	// can't. The SDK validates the answer against this schema, and a violation
	// fails the whole call instead of reaching deleteConfirmed, so the field
	// carries only what clients enforce in their own form: required (Claude
	// Code refuses to accept an empty field) but no pattern (Claude Code
	// submits a non-matching value, which would then fail SDK validation
	// instead of returning a clean cancellation).
	return &mcp.CallToolResult{
		InputRequests: mcp.InputRequestMap{
			deleteConfirmationKey: &mcp.ElicitParams{
				Message: fmt.Sprintf("Delete PRODUCTION service %q (%s)? This permanently destroys the service and all of its data, and cannot be undone.", service.Name, service.ServiceID),
				RequestedSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						deleteConfirmationField: {
							Type:        "string",
							Title:       "Service ID",
							Description: fmt.Sprintf("Type the service ID %s to confirm", service.ServiceID),
						},
					},
					Required: []string{deleteConfirmationField},
				},
			},
		},
	}, nil
}

// clientSupportsFormElicitation reports whether the client advertised form
// elicitation. The capabilities come from the request's _meta on the newest
// protocol and from the initialize handshake before that, which is also why a
// stateless HTTP session on an older protocol reports none.
func clientSupportsFormElicitation(req *mcp.CallToolRequest) bool {
	caps := req.ClientCapabilities()
	if caps == nil || caps.Elicitation == nil {
		return false
	}
	// A client that declares elicitation without naming a mode supports form
	// (the SDK assumes the same, for backward compatibility); one that names
	// only url does not.
	return caps.Elicitation.Form != nil || caps.Elicitation.URL == nil
}

// deleteConfirmed reports whether the user's answer to promptProdDelete's
// prompt approves deleting the service with the given ID: only an accepted form
// with that exact ID typed in does. A decline, a dismissal, or a mismatched ID
// is a no.
func deleteConfirmed(answer mcp.InputResponse, serviceID string) bool {
	result, ok := answer.(*mcp.ElicitResult)
	if !ok || result.Action != "accept" {
		return false
	}
	typed, ok := result.Content[deleteConfirmationField].(string)
	return ok && typed == serviceID
}
