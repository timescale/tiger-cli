package cmd

import (
	"context"
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
	// feedbackRequest is the body the command sends for message: the source is
	// always CLI, and the client version and OS travel in the User-Agent rather
	// than the body.
	feedbackRequest := func(message string) api.SubmitFeedbackJSONRequestBody {
		return api.SubmitFeedbackJSONRequestBody{
			Message: message,
			Source:  new(api.SubmitFeedbackJSONBodySourceCLI),
		}
	}
	expectSubmit := func(message string, resp *api.SubmitFeedbackResponse, err error) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().SubmitFeedbackWithResponse(validCtx, feedbackRequest(message)).Return(resp, err)
		}
	}
	submitted := &api.SubmitFeedbackResponse{HTTPResponse: httpResponse(http.StatusNoContent)}

	// secretMessage is the kind of text that would leak if the message ever
	// reached analytics.
	const secretMessage = "cannot connect with postgres://tsdbadmin:hunter2@svc.tsdb.cloud/tsdb"

	// trackedEvent captures the analytics event wrapCommands defers. Cases run
	// sequentially, so one var serves them all.
	var trackedEvent api.TrackEventJSONRequestBody
	expectTrack := func(m *mocks.MockClientWithResponsesInterface) {
		// elapsed_seconds and the temp config dir make the body
		// nondeterministic, so capture it and assert in checkTrackedArgs.
		m.EXPECT().TrackEventWithResponse(validCtx, gomock.Any()).
			DoAndReturn(func(_ context.Context, body api.TrackEventJSONRequestBody, _ ...api.RequestEditorFn) (*api.TrackEventResponse, error) {
				trackedEvent = body
				return &api.TrackEventResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil
			})
	}

	// checkTrackedArgs asserts the tracked event carried wantArgs and the
	// user's flags, and mentions message nowhere in its properties.
	checkTrackedArgs := func(wantArgs []string, message string) checkFunc {
		return func(t *testing.T, _ cmdResult) {
			t.Helper()
			if want := "Run tiger feedback"; trackedEvent.Event != want {
				t.Errorf("tracked event = %q, want %q", trackedEvent.Event, want)
			}
			if trackedEvent.Properties == nil {
				t.Fatal("tracked event carried no properties")
			}
			props := *trackedEvent.Properties
			if diff := cmp.Diff(wantArgs, props["args"]); diff != "" {
				t.Errorf("tracked args property mismatch (-want +got):\n%s", diff)
			}
			// Only argument values are replaced; --analytics is a flag each
			// case sets itself.
			if _, ok := props["analytics"]; !ok {
				t.Errorf("tracked event carries no flags, want the flags the user set: %#v", props)
			}
			// The message must not appear under any key, not just "args".
			encoded, err := json.Marshal(props)
			if err != nil {
				t.Fatalf("failed to encode tracked properties: %v", err)
			}
			if strings.Contains(string(encoded), message) {
				t.Errorf("tracked event contains the feedback message: %s", encoded)
			}
		}
	}

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
			// Whitespace-only input is empty too: the API would reject it, and
			// the message is more useful before a round trip.
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
			// A 5XX carries no typed body, so the error falls back to the
			// generic message.
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
			wantStdout: "Feedback submitted! Thank you.\n",
		},
		{
			// Surrounding whitespace, such as the newline echo appends, is
			// trimmed before sending.
			name:       "message from stdin",
			args:       []string{"feedback"},
			opts:       []runOption{withStdin("Great tool!\n")},
			setup:      expectSubmit("Great tool!", submitted, nil),
			wantStdout: "Feedback submitted! Thank you.\n",
		},
		{
			// On a terminal the command says what it is waiting for; the hint
			// goes to stderr so a redirected stdout stays clean.
			name:       "prompts on a terminal",
			args:       []string{"feedback"},
			opts:       []runOption{withIsTerminal(true), withStdin("Great tool!")},
			setup:      expectSubmit("Great tool!", submitted, nil),
			wantStdout: "Feedback submitted! Thank you.\n",
			wantStderr: "Enter your feedback (press Ctrl+D when done):\n",
		},
		{
			// The message may quote a connection string or a query, so the
			// annotation replaces it before the event is sent. The event has to
			// actually be sent for this to test anything, hence --analytics=true
			// over the harness default and the neutralized opt-outs.
			name: "feedback message never reaches analytics",
			args: []string{"feedback", secretMessage, "--analytics=true"},
			opts: []runOption{
				withEnv("DO_NOT_TRACK", ""),
				withEnv("NO_TELEMETRY", ""),
				withEnv("DISABLE_TELEMETRY", ""),
			},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectSubmit(secretMessage, submitted, nil)(m)
				expectTrack(m)
			},
			wantStdout: "Feedback submitted! Thank you.\n",
			checks:     []checkFunc{checkTrackedArgs([]string{redactedArgValue}, secretMessage)},
		},
		{
			// Redacting values rather than dropping the property preserves
			// this: a piped message leaves no argument, so the two entry points
			// stay distinguishable.
			name: "analytics distinguishes a piped message from an argument",
			args: []string{"feedback", "--analytics=true"},
			opts: []runOption{
				withStdin(secretMessage),
				withEnv("DO_NOT_TRACK", ""),
				withEnv("NO_TELEMETRY", ""),
				withEnv("DISABLE_TELEMETRY", ""),
			},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectSubmit(secretMessage, submitted, nil)(m)
				expectTrack(m)
			},
			wantStdout: "Feedback submitted! Thank you.\n",
			checks:     []checkFunc{checkTrackedArgs([]string{}, secretMessage)},
		},
	})
}
