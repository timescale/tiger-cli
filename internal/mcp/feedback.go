package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// maxFeedbackMessageLength mirrors the API's own limit. It is deliberately not
// a schema MaxLength: the validator's error quotes the whole message, which
// analytics then records.
const maxFeedbackMessageLength = 3000

// FeedbackInput represents input for feedback
type FeedbackInput struct {
	Message string `json:"message"`
}

func (FeedbackInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[FeedbackInput](nil))

	schema.Properties["message"].Description = fmt.Sprintf("The feedback or bug report to send. Describe what the user was trying to do and what happened. At most %d characters.", maxFeedbackMessageLength)
	schema.Properties["message"].MinLength = new(1)
	schema.Properties["message"].Examples = []any{"Creating a service fails with INVALID_REQUEST when the region is us-west-2."}

	return schema
}

// FeedbackOutput represents output for feedback
type FeedbackOutput struct {
	Success bool `json:"success"`
	// SupportURL is project-specific, so it can't live in the tool description.
	SupportURL string `json:"support_url"`
}

func (FeedbackOutput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[FeedbackOutput](nil))

	schema.Properties["success"].Description = "Whether the feedback was submitted."
	schema.Properties["support_url"].Description = "The Tiger Cloud console page where the user opens a tracked support ticket."

	return schema
}

func newFeedbackTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolFeedback,
		Title: "Submit Feedback",
		Description: `Submit feedback or a bug report to the Tiger Data team.

The message reaches a person, so confirm the wording with the user before sending. This opens no support case and returns no ticket to track; for anything that needs a tracked response, point the user at the support URL returned in the output.`,
		InputSchema:  FeedbackInput{}.Schema(),
		OutputSchema: FeedbackOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false, // Sending the same feedback twice delivers it twice
			OpenWorldHint:   new(false),
			Title:           "Submit Feedback",
		},
	}
}

// handleFeedback handles the feedback MCP tool
func (s *Server) handleFeedback(ctx context.Context, req *mcp.CallToolRequest, input FeedbackInput) (*mcp.CallToolResult, FeedbackOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, FeedbackOutput{}, err
	}

	message, err := validateFeedbackMessage(input.Message)
	if err != nil {
		return nil, FeedbackOutput{}, err
	}

	resp, err := client.SubmitFeedbackWithResponse(ctx, api.SubmitFeedbackJSONRequestBody{
		Message: message,
		Source:  new(api.SubmitFeedbackJSONBodySourceMCP),
	})
	if err != nil {
		return nil, FeedbackOutput{}, fmt.Errorf("failed to submit feedback: %w", err)
	}

	if resp.StatusCode() != http.StatusNoContent {
		return nil, FeedbackOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	return nil, FeedbackOutput{
		Success:    true,
		SupportURL: fmt.Sprintf("%s/projects/%s/support/main", cfg.ConsoleURL, projectID),
	}, nil
}

// validateFeedbackMessage trims the message like the CLI does, so both surfaces
// send the same text. Its errors report only the length, never the message,
// which analytics would record.
func validateFeedbackMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("feedback message cannot be empty")
	}
	if n := utf8.RuneCountInString(message); n > maxFeedbackMessageLength {
		return "", fmt.Errorf("feedback message is too long: %d characters, limit is %d", n, maxFeedbackMessageLength)
	}
	return message, nil
}
