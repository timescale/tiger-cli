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

// detachAllowListConfirmationKey is the InputRequests/InputResponses key of
// the PROD detach prompt.
const detachAllowListConfirmationKey = "confirm_detach_allowlist"

// ServiceAllowListDetachInput represents input for service_allowlist_detach
type ServiceAllowListDetachInput struct {
	ServiceID   string `json:"service_id"`
	AllowListID string `json:"allow_list_id"`
}

func (ServiceAllowListDetachInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceAllowListDetachInput](nil))
	setServiceIDSchemaProperties(schema)
	schema.Properties["allow_list_id"].Description = "The IP allow list to detach the service from. Use allowlist_list to find IP allow list IDs."
	schema.Properties["allow_list_id"].Examples = []any{"1234567890"}
	return schema
}

// ServiceAllowListDetachOutput represents output for service_allowlist_detach
type ServiceAllowListDetachOutput struct {
	Detached bool   `json:"detached"`
	Message  string `json:"message"`
}

func (ServiceAllowListDetachOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceAllowListDetachOutput](nil))

	schema.Properties["detached"].Description = "Whether the service was detached from the IP allow list. False when the user declined the confirmation prompt for a PROD service; do not retry unless the user asks again."
	schema.Properties["message"].Description = "Human-readable outcome of the operation"

	return schema
}

func newServiceAllowListDetachTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceAllowListDetach,
		Title: "Detach Service from IP Allow List",
		Description: `Remove the IP restriction an IP allow list placed on a service.

The IP allow list itself is not deleted. Detaching a PROD-tagged service automatically prompts the user, via an elicitation request through the MCP client, to confirm before it proceeds, so agents don't need to ask the user for confirmation themselves; if the client cannot prompt, the detach is refused and the user must run 'tiger service allowlist detach' from the CLI instead.`,
		InputSchema:  ServiceAllowListDetachInput{}.Schema(),
		OutputSchema: ServiceAllowListDetachOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // removes an existing IP restriction
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
			Title:           "Detach Service from IP Allow List",
		},
	}
}

// handleServiceAllowListDetach handles the service_allowlist_detach MCP
// tool.
//
// Detaching a PROD service from its IP allow list is a multi round-trip
// call: the first invocation returns an elicitation asking the user to
// confirm, and the SDK re-invokes the handler with the answer in
// InputResponses — client-side on protocol 2026-07-28 and later,
// server-side (via ServerSession.Elicit) for older clients — so the handler
// never calls Elicit itself.
func (s *Server) handleServiceAllowListDetach(ctx context.Context, req *mcp.CallToolRequest, input ServiceAllowListDetachInput) (*mcp.CallToolResult, ServiceAllowListDetachOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceAllowListDetachOutput{}, err
	}

	// Refuse without an API call under read_only=all. prod needs the tag, so the
	// real gate waits for the fetch below.
	if cfg.ReadOnly.BlocksAll() {
		return nil, ServiceAllowListDetachOutput{}, common.ErrReadOnly
	}

	// The service's tag decides both the prod half of the read-only gate and
	// whether the user has to confirm, so fetch it unconditionally.
	service, err := common.GetService(ctx, client, projectID, input.ServiceID)
	if err != nil {
		return nil, ServiceAllowListDetachOutput{}, err
	}

	tag := common.ServiceEnvironmentTag(*service)
	if err := common.CheckReadOnly(cfg, tag); err != nil {
		return nil, ServiceAllowListDetachOutput{}, err
	}

	if tag == api.EnvironmentTagPROD {
		answer, answered := req.Params.InputResponses[detachAllowListConfirmationKey]
		if !answered {
			result, err := promptProdAllowListDetach(req, *service, input.AllowListID)
			return result, ServiceAllowListDetachOutput{}, err
		}
		if !serviceIDConfirmed(answer, input.ServiceID) {
			return nil, ServiceAllowListDetachOutput{
				Detached: false,
				Message:  fmt.Sprintf("Detach cancelled: the user did not confirm detaching PROD service %q from IP allow list %q by typing its ID.", input.ServiceID, input.AllowListID),
			}, nil
		}
	}

	s.logger.Info("MCP: Detaching service from IP allow list",
		slog.String("project_id", projectID),
		slog.String("service_id", input.ServiceID),
		slog.String("allow_list_id", input.AllowListID),
	)

	resp, err := client.DetachServiceFromAllowListWithResponse(ctx, projectID, input.ServiceID, api.ServiceAllowListInput{
		AllowListID: input.AllowListID,
	})
	if err != nil {
		return nil, ServiceAllowListDetachOutput{}, fmt.Errorf("failed to detach service from IP allow list: %w", err)
	}

	if resp.StatusCode() != http.StatusAccepted {
		return nil, ServiceAllowListDetachOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON202 == nil {
		return nil, ServiceAllowListDetachOutput{}, fmt.Errorf("empty response from API")
	}

	return nil, ServiceAllowListDetachOutput{
		Detached: true,
		Message:  fmt.Sprintf("Service '%s' detached from IP allow list '%s'.", input.ServiceID, input.AllowListID),
	}, nil
}

// promptProdAllowListDetach returns the input-required result that asks the
// user to confirm detaching a PROD service from its IP allow list. It refuses
// outright when the client can't show the prompt: silently widening a
// production service's network exposure without a human in the loop is the
// one outcome this tool must never produce.
func promptProdAllowListDetach(req *mcp.CallToolRequest, service api.Service, allowListID string) (*mcp.CallToolResult, error) {
	if !clientSupportsFormElicitation(req) {
		return nil, fmt.Errorf("detaching service %s from IP allow list %s requires the user's confirmation because it is tagged PROD, but this MCP client does not support elicitation; ask the user to run 'tiger service allowlist detach %s --allow-list %s' instead", service.ServiceID, allowListID, service.ServiceID, allowListID)
	}
	return serviceIDConfirmationRequest(
		detachAllowListConfirmationKey,
		fmt.Sprintf("Remove the IP restriction from PRODUCTION service %q (%s) by detaching it from IP allow list %q? The service will become reachable from any IP.", service.Name, service.ServiceID, allowListID),
		service.ServiceID,
	), nil
}
