package mcp

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestFeedback(t *testing.T) {
	args := map[string]any{"message": "Great tool!"}

	expectSubmit := func(message string, resp *api.SubmitFeedbackResponse, err error) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().SubmitFeedbackWithResponse(validCtx, api.SubmitFeedbackJSONRequestBody{
				Message: message,
				Source:  new(api.SubmitFeedbackJSONBodySourceMCP),
			}).Return(resp, err)
		}
	}
	submitted := &api.SubmitFeedbackResponse{HTTPResponse: httpResponse(http.StatusNoContent)}

	// The support link is built from the console URL and the caller's project.
	sent := map[string]any{
		"success":     true,
		"support_url": "https://console.cloud.tigerdata.com/projects/" + testProjectID + "/support/main",
	}

	// The limit counts runes, not bytes: 3000 two-byte characters pass.
	atLimit := strings.Repeat("é", maxFeedbackMessageLength)

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolFeedback,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "missing message",
			tool:    toolFeedback,
			args:    map[string]any{},
			wantErr: `validating "arguments": validating root: required: missing properties: ["message"]`,
		},
		{
			// The schema's MinLength rejects the empty string before the handler
			// runs, so the error is the validator's.
			name:    "empty message",
			tool:    toolFeedback,
			args:    map[string]any{"message": ""},
			wantErr: `validating "arguments": validating root: validating /properties/message: minLength: "" contains 0 Unicode code points, fewer than 1`,
		},
		{
			// Whitespace passes MinLength but trims to nothing, and is caught
			// before the round trip.
			name:    "blank message",
			tool:    toolFeedback,
			args:    map[string]any{"message": "  \n\t"},
			wantErr: "feedback message cannot be empty",
		},
		{
			// The limit is enforced by the handler, not the schema, so the error
			// reports the length without echoing the message.
			name:    "message too long",
			tool:    toolFeedback,
			args:    map[string]any{"message": strings.Repeat("a", maxFeedbackMessageLength+1)},
			wantErr: "feedback message is too long: 3001 characters, limit is 3000",
		},
		{
			name:    "network error",
			tool:    toolFeedback,
			args:    args,
			mock:    expectSubmit("Great tool!", nil, errors.New("connection refused")),
			wantErr: "failed to submit feedback: connection refused",
		},
		{
			name: "API error",
			tool: toolFeedback,
			args: args,
			mock: expectSubmit("Great tool!", &api.SubmitFeedbackResponse{
				HTTPResponse: httpResponse(http.StatusBadRequest),
				JSON4XX:      &api.Error{Message: new("message must not be blank")},
			}, nil),
			wantErr: "message must not be blank",
		},
		{
			// A 5XX carries no typed body, so the error is the generic one.
			name: "server error",
			tool: toolFeedback,
			args: args,
			mock: expectSubmit("Great tool!", &api.SubmitFeedbackResponse{
				HTTPResponse: httpResponse(http.StatusInternalServerError),
			}, nil),
			wantErr: "unknown error",
		},
		{
			name:       "submits feedback",
			tool:       toolFeedback,
			args:       args,
			mock:       expectSubmit("Great tool!", submitted, nil),
			wantOutput: sent,
		},
		{
			// Trimmed like the CLI, so both surfaces send the same text.
			name:       "trims surrounding whitespace",
			tool:       toolFeedback,
			args:       map[string]any{"message": "  Great tool!\n"},
			mock:       expectSubmit("Great tool!", submitted, nil),
			wantOutput: sent,
		},
		{
			name:       "message at the limit",
			tool:       toolFeedback,
			args:       map[string]any{"message": atLimit},
			mock:       expectSubmit(atLimit, submitted, nil),
			wantOutput: sent,
		},
		{
			// Feedback mutates no service, so the tool isn't read-only gated: it
			// stays registered and works under read_only=all.
			name:       "read-only all still submits",
			tool:       toolFeedback,
			args:       args,
			opts:       []runOption{withConfig(map[string]any{"read_only": "all"})},
			mock:       expectSubmit("Great tool!", submitted, nil),
			wantOutput: sent,
		},
	})
}
