package mcp

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// confirmServiceIDField is the field of a PROD confirmation prompt where the
// user types the service ID back.
const confirmServiceIDField = "service_id"

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

// serviceIDConfirmationRequest returns the input-required result that asks the
// user to confirm an action on a PROD service by typing its ID back, under
// key in InputRequests. The caller checks clientSupportsFormElicitation first.
//
// The user types the ID back, as the CLI requires. Clients focus their accept
// control by default, so a bare accept/decline form would let a stray Enter
// through; a mistyped or empty ID can't. The SDK validates the answer against
// this schema, and a violation fails the whole call instead of reaching
// serviceIDConfirmed, so the field carries only what clients enforce in their
// own form: required (Claude Code refuses to accept an empty field) but no
// pattern (Claude Code submits a non-matching value, which would then fail SDK
// validation instead of returning a clean cancellation).
func serviceIDConfirmationRequest(key, message, serviceID string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		InputRequests: mcp.InputRequestMap{
			key: &mcp.ElicitParams{
				Message: message,
				RequestedSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						confirmServiceIDField: {
							Type:        "string",
							Title:       "Service ID",
							Description: fmt.Sprintf("Type the service ID %s to confirm", serviceID),
						},
					},
					Required: []string{confirmServiceIDField},
				},
			},
		},
	}
}

// serviceIDConfirmed reports whether the user's answer to a
// serviceIDConfirmationRequest prompt approves the action on the service with
// the given ID: only an accepted form with that exact ID typed in does. A
// decline, a dismissal, or a mismatched ID is a no.
func serviceIDConfirmed(answer mcp.InputResponse, serviceID string) bool {
	result, ok := answer.(*mcp.ElicitResult)
	if !ok || result.Action != "accept" {
		return false
	}
	typed, ok := result.Content[confirmServiceIDField].(string)
	return ok && typed == serviceID
}
