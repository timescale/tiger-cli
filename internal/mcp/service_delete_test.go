package mcp

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceDeleteTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	expectDelete := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
			Return(&api.DeleteServiceResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
	}
	expectTaggedServiceAndDelete := func(tag string) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectTaggedService(tag, 1)(m)
			expectDelete(m)
		}
	}

	deleted := map[string]any{
		"service_id": "e6ue9697jf",
		"deleted":    true,
		"message":    `Service "e6ue9697jf" has been deleted.`,
	}
	cancelled := map[string]any{
		"service_id": "e6ue9697jf",
		"deleted":    false,
		"message":    `Deletion cancelled: the user did not confirm deleting PROD service "e6ue9697jf" by typing its ID.`,
	}
	const noElicitationMsg = "deleting service e6ue9697jf requires the user's confirmation because it is tagged PROD, " +
		"but this MCP client does not support elicitation; ask the user to run 'tiger service delete e6ue9697jf' instead"

	// The prompt raised for a PROD service, as the client receives it after
	// the JSON round trip (which is where the SDK fills in the mode).
	prompt := &mcp.ElicitParams{
		Mode:    "form",
		Message: `Delete PRODUCTION service "test-service" (e6ue9697jf)? This permanently destroys the service and all of its data, and cannot be undone.`,
		RequestedSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"service_id": map[string]any{
					"type":        "string",
					"title":       "Service ID",
					"description": "Type the service ID e6ue9697jf to confirm",
				},
			},
			"required": []any{"service_id"},
		},
	}
	// promptWith is prompt with extra appended to its message, for a fluid
	// service whose branches the prompt names.
	promptWith := func(extra string) *mcp.ElicitParams {
		p := *prompt
		p.Message += extra
		return &p
	}
	// expectFluidService registers fetching a fluid service tagged tag, the
	// kind that can have branches, on each of calls handler runs.
	expectFluidService := func(tag string, calls int) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) {
				s.Metadata = &api.ServiceMetadata{Environment: &tag}
				s.Platform = new(api.ServicePlatformFLUID)
			})).Times(calls)
		}
	}
	// expectBranches registers listing the service's branches on each of
	// calls handler runs.
	expectBranches := func(m *mocks.MockClientWithResponsesInterface, calls int, branches ...api.Branch) {
		m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "e6ue9697jf", nil).
			Return(&api.GetBranchesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &branches,
			}, nil).Times(calls)
	}
	otherBranch := sampleBranch(func(b *api.Branch) {
		b.ID = "x7k2m9p4qa"
		b.Name = "other-branch"
	})
	// deletedWith is a successful deletion's result, with the given message.
	deletedWith := func(message string) map[string]any {
		return map[string]any{
			"service_id": "e6ue9697jf",
			"deleted":    true,
			"message":    message,
		}
	}

	// confirm answers the prompt by typing id back.
	confirm := func(id string) runOption {
		return withElicitation(&mcp.ElicitResult{Action: "accept", Content: map[string]any{"service_id": id}})
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceDelete,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceDelete,
			args:    args,
			opts:    []runOption{withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name: "service lookup fails",
			tool: toolServiceDelete,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			// Only the tag lookup is registered: an attempted deletion fails as
			// an unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceDelete,
			args:    args,
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectTaggedServiceAndDelete("DEV"),
			wantOutput: deleted,
		},
		{
			// The client here can't prompt, which proves DEV deletions never try.
			name:       "deletes DEV service without prompting",
			tool:       toolServiceDelete,
			args:       args,
			mock:       expectTaggedServiceAndDelete("DEV"),
			wantOutput: deleted,
		},
		{
			// The handler runs twice — once to raise the prompt, once with the
			// answer — and fetches the service on both runs.
			name: "deletes PROD service once the user confirms",
			tool: toolServiceDelete,
			args: args,
			opts: []runOption{confirm("e6ue9697jf")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("PROD", 2)(m)
				expectDelete(m)
			},
			wantPrompt: prompt,
			wantOutput: deleted,
		},
		{
			// The API deletes a service's branches along with it, so the
			// prompt and the result name them even without the experimental
			// gate. Both handler runs list them.
			name: "PROD prompt and result name the branches deleted with the service",
			tool: toolServiceDelete,
			args: args,
			opts: []runOption{confirm("e6ue9697jf")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectFluidService("PROD", 2)(m)
				expectBranches(m, 2, sampleBranch(), otherBranch)
				expectDelete(m)
			},
			wantPrompt: promptWith(` Its 2 branches will also be deleted: "test-branch" (br4nch1d9x), "other-branch" (x7k2m9p4qa).`),
			wantOutput: deletedWith(`Service "e6ue9697jf" has been deleted, along with its 2 branches: "test-branch" (br4nch1d9x), "other-branch" (x7k2m9p4qa).`),
		},
		{
			name: "PROD prompt names a single branch",
			tool: toolServiceDelete,
			args: args,
			opts: []runOption{confirm("e6ue9697jf")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectFluidService("PROD", 2)(m)
				expectBranches(m, 2, sampleBranch())
				expectDelete(m)
			},
			wantPrompt: promptWith(` Its 1 branch will also be deleted: "test-branch" (br4nch1d9x).`),
			wantOutput: deletedWith(`Service "e6ue9697jf" has been deleted, along with its 1 branch: "test-branch" (br4nch1d9x).`),
		},
		{
			name: "PROD prompt for a fluid service without branches names none",
			tool: toolServiceDelete,
			args: args,
			opts: []runOption{confirm("e6ue9697jf")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectFluidService("PROD", 2)(m)
				expectBranches(m, 2)
				expectDelete(m)
			},
			wantPrompt: prompt,
			wantOutput: deleted,
		},
		{
			// The listing only informs the decision, so a failure still
			// prompts, with a warning in place of the names.
			name: "PROD prompt when the branch listing fails",
			tool: toolServiceDelete,
			args: args,
			opts: []runOption{confirm("e6ue9697jf")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectFluidService("PROD", 2)(m)
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "e6ue9697jf", nil).
					Return(nil, errors.New("connection refused")).Times(2)
				expectDelete(m)
			},
			wantPrompt: promptWith(" Any branches of the service will also be deleted."),
			wantOutput: deleted,
		},
		{
			// DEV deletions don't prompt, but the result still names the
			// branches deleted along with the service.
			name: "deletes fluid DEV service and names its branches",
			tool: toolServiceDelete,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectFluidService("DEV", 1)(m)
				expectBranches(m, 1, sampleBranch(), otherBranch)
				expectDelete(m)
			},
			wantOutput: deletedWith(`Service "e6ue9697jf" has been deleted, along with its 2 branches: "test-branch" (br4nch1d9x), "other-branch" (x7k2m9p4qa).`),
		},
		{
			name:       "PROD deletion cancelled when the typed ID does not match",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{confirm("u8me885b93")},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			// A non-ID answer must reach the handler as a cancellation, not
			// fail schema validation in the SDK, so the field has no pattern.
			name:       "PROD deletion cancelled when the typed ID is not an ID at all",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{confirm("fail")},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD deletion cancelled when the user declines",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{withElicitation(&mcp.ElicitResult{Action: "decline"})},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD deletion cancelled when the user dismisses the prompt",
			tool:       toolServiceDelete,
			args:       args,
			opts:       []runOption{withElicitation(&mcp.ElicitResult{Action: "cancel"})},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:    "PROD deletion refused when the client lacks elicitation",
			tool:    toolServiceDelete,
			args:    args,
			mock:    expectTaggedService("PROD", 1),
			wantErr: noElicitationMsg,
		},
		{
			name: "PROD deletion refused when the client supports only url elicitation",
			tool: toolServiceDelete,
			args: args,
			opts: []runOption{withClientCapabilities(&mcp.ClientCapabilities{
				Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}},
			})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: noElicitationMsg,
		},
		{
			name: "delete API error",
			tool: toolServiceDelete,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV", 1)(m)
				m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.DeleteServiceResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
		},
	})
}

// TestServiceDeleteToolDescription covers the description's experimental
// gate at the helper level: the harness calls tools but never lists them, so
// a description isn't reachable through runTool.
func TestServiceDeleteToolDescription(t *testing.T) {
	const branchNote = "Deleting a service also deletes all of its branches."
	if got := newServiceDeleteTool(false).Description; strings.Contains(got, "branch") {
		t.Errorf("description must not mention branches without the gate, got:\n%s", got)
	}
	if got := newServiceDeleteTool(true).Description; !strings.Contains(got, branchNote) {
		t.Errorf("description must contain %q behind the gate, got:\n%s", branchNote, got)
	}
}
