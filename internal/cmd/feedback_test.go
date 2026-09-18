package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestFeedbackCmd(t *testing.T) {
	expectSubmit := func(message string, resp *api.SubmitFeedbackResponse, err error) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().SubmitFeedbackWithResponse(validCtx, api.SubmitFeedbackJSONRequestBody{
				Message: message,
				Source:  new(api.SubmitFeedbackJSONBodySourceCLI),
			}).Return(resp, err)
		}
	}
	submitted := &api.SubmitFeedbackResponse{HTTPResponse: httpResponse(http.StatusNoContent)}

	const secretMessage = "cannot connect with postgres://tsdbadmin:hunter2@svc.tsdb.cloud/tsdb"

	// trackedEvent matches the analytics event a feedback invocation sends,
	// asserting the tracked args, that the user's flags survived, and that
	// message appears nowhere. The body can't be matched exactly — it carries
	// elapsed_seconds and the temp config dir — hence a matcher on the
	// properties that matter, checked at the moment the event is sent.
	trackedEvent := func(wantArgs []string, message string) gomock.Matcher {
		return gomock.Cond(func(body api.TrackEventJSONRequestBody) bool {
			if body.Properties == nil {
				return false
			}
			props := *body.Properties
			if !cmp.Equal(wantArgs, props["args"]) {
				return false
			}
			// Only argument values are replaced; --analytics is a flag each case sets.
			if _, ok := props["analytics"]; !ok {
				return false
			}
			// The message must not appear under any key, not just "args".
			encoded, err := json.Marshal(props)
			return err == nil && !strings.Contains(string(encoded), message)
		})
	}
	expectSubmitAndTrack := func(message string, wantArgs []string) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectSubmit(message, submitted, nil)(m)
			m.EXPECT().TrackEventWithResponse(validCtx, trackedEvent(wantArgs, message)).
				Return(&api.TrackEventResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
		}
	}

	// The support link is built from the console URL and the caller's project.
	const wantSubmitted = "Feedback submitted.\n" +
		"For a tracked response, open a support ticket: https://console.cloud.tigerdata.com/projects/" + testProjectID + "/support/main\n"

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"feedback", "hello"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "rejects a second positional argument",
			args:    []string{"feedback", "one", "two"},
			wantErr: "accepts at most 1 arg(s), received 2",
		},
		{
			name:    "empty message from stdin",
			args:    []string{"feedback"},
			opts:    []runOption{withStdin("")},
			wantErr: "feedback message cannot be empty",
		},
		{
			// Whitespace-only counts as empty, and is caught before the round trip.
			name:    "blank message from argument",
			args:    []string{"feedback", "  \n\t"},
			wantErr: "feedback message cannot be empty",
		},
		{
			name:    "network error",
			args:    []string{"feedback", "it broke"},
			setup:   expectSubmit("it broke", nil, errors.New("connection refused")),
			wantErr: "failed to submit feedback: connection refused",
		},
		{
			name: "API error",
			args: []string{"feedback", "it broke"},
			setup: expectSubmit("it broke", &api.SubmitFeedbackResponse{
				HTTPResponse: httpResponse(http.StatusBadRequest),
				JSON4XX:      &api.Error{Message: new("message must not be blank")},
			}, nil),
			wantErr: "message must not be blank",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			// A 5XX carries no typed body, so the error is the generic one.
			name: "server error",
			args: []string{"feedback", "it broke"},
			setup: expectSubmit("it broke", &api.SubmitFeedbackResponse{
				HTTPResponse: httpResponse(http.StatusInternalServerError),
			}, nil),
			wantErr: "unknown error",
			checks:  []checkFunc{checkExitCode(common.ExitGeneralError)},
		},
		{
			name:       "message from argument",
			args:       []string{"feedback", "Great tool!"},
			setup:      expectSubmit("Great tool!", submitted, nil),
			wantStdout: wantSubmitted,
		},
		{
			// The trailing newline echo appends is trimmed before sending.
			name:       "message from stdin",
			args:       []string{"feedback"},
			opts:       []runOption{withStdin("Great tool!\n")},
			setup:      expectSubmit("Great tool!", submitted, nil),
			wantStdout: wantSubmitted,
		},
		{
			// The hint goes to stderr, so a redirected stdout stays clean.
			name:       "prompts on a terminal",
			args:       []string{"feedback"},
			opts:       []runOption{withIsTerminal(true), withStdin("Great tool!")},
			setup:      expectSubmit("Great tool!", submitted, nil),
			wantStdout: wantSubmitted,
			wantStderr: "Enter your feedback (press Ctrl+D when done):\n",
		},
		{
			// The event has to actually be sent for this to test anything, hence
			// --analytics=true over the harness default and the neutralized opt-outs.
			name: "feedback message never reaches analytics",
			args: []string{"feedback", secretMessage, "--analytics=true"},
			opts: []runOption{
				withEnv("DO_NOT_TRACK", ""),
				withEnv("NO_TELEMETRY", ""),
				withEnv("DISABLE_TELEMETRY", ""),
			},
			setup:      expectSubmitAndTrack(secretMessage, []string{"[REDACTED]"}),
			wantStdout: wantSubmitted,
		},
		{
			// A piped message leaves no argument at all, so the redaction has to
			// leave the empty list alone rather than index into it.
			name: "analytics distinguishes a piped message from an argument",
			args: []string{"feedback", "--analytics=true"},
			opts: []runOption{
				withStdin(secretMessage),
				withEnv("DO_NOT_TRACK", ""),
				withEnv("NO_TELEMETRY", ""),
				withEnv("DISABLE_TELEMETRY", ""),
			},
			setup:      expectSubmitAndTrack(secretMessage, []string{}),
			wantStdout: wantSubmitted,
		},
	})
}
