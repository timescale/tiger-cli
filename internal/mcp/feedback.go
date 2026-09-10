package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// FeedbackInput represents input for feedback
type FeedbackInput struct {
	Message string `json:"message"`
}

func (FeedbackInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[FeedbackInput](nil))

	schema.Properties["message"].Description = "The feedback, bug report, or support request to send. Describe what the user was trying to do and what happened."
	schema.Properties["message"].MinLength = new(1)
	schema.Properties["message"].MaxLength = new(3000)
	schema.Properties["message"].Examples = []any{"Creating a service fails with INVALID_REQUEST when the region is us-west-2."}

	return schema
}

// FeedbackOutput represents output for feedback
type FeedbackOutput struct {
	Success bool `json:"success" jsonschema:"Whether the feedback was submitted"`
}

func (FeedbackOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[FeedbackOutput](nil))
}

func newFeedbackTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "feedback",
		Title: "Submit Feedback",
		Description: `Submit feedback, a bug report, or a support request to the Tiger Data team.

The message reaches a person, so confirm the wording with the user before sending. This opens no support case and returns no ticket to track.`,
		InputSchema:  FeedbackInput{}.Schema(),
		OutputSchema: FeedbackOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false, // Sending the same feedback twice delivers it twice
			OpenWorldHint:   new(true),
			Title:           "Submit Feedback",
		},
	}
}

// handleFeedback handles the feedback MCP tool
func (s *Server) handleFeedback(ctx context.Context, req *mcp.CallToolRequest, input FeedbackInput) (*mcp.CallToolResult, FeedbackOutput, error) {
	client, _, err := s.app.GetClient()
	if err != nil {
		return nil, FeedbackOutput{}, err
	}

	// Trimmed like the CLI, so both surfaces send the same message.
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return nil, FeedbackOutput{}, errors.New("feedback message cannot be empty")
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

	return nil, FeedbackOutput{Success: true}, nil
}
