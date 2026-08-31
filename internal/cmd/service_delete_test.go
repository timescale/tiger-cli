package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceDeleteCmd(t *testing.T) {
	setupDelete := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
			Return(&api.DeleteServiceResponse{
				HTTPResponse: httpResponse(http.StatusAccepted),
			}, nil)
	}

	confirmPrompt := "Are you sure you want to delete service 'svc-12345'? This operation cannot be undone.\n" +
		"Type the service ID 'svc-12345' to confirm: "

	runCmdTests(t, []cmdTest{
		{
			// No fallback to the configured default service ID for deletes.
			name:    "missing service id",
			args:    []string{"service", "delete"},
			opts:    []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: "service ID is required",
		},
		{
			name:    "not logged in",
			args:    []string{"service", "delete", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only mode",
			args:    []string{"service", "delete", "svc-12345"},
			opts:    []runOption{withConfig(map[string]any{"read_only": true})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "non-TTY without confirm",
			args:    []string{"service", "delete", "svc-12345"},
			wantErr: "TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt",
		},
		{
			name:       "confirmation mismatch",
			args:       []string{"service", "delete", "svc-12345"},
			opts:       []runOption{withIsTerminal(true), withStdin("svc-other\n")},
			wantStderr: confirmPrompt + "❌ Delete operation cancelled.\n",
		},
		{
			name:  "confirmation match",
			args:  []string{"service", "delete", "svc-12345", "--no-wait"},
			opts:  []runOption{withIsTerminal(true), withStdin("svc-12345\n")},
			setup: setupDelete,
			wantStderr: confirmPrompt +
				"🗑️  Delete request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service list' to check deletion status.\n",
		},
		{
			name:  "confirm flag skips prompt",
			args:  []string{"service", "delete", "svc-12345", "--confirm", "--no-wait"},
			setup: setupDelete,
			wantStderr: "🗑️  Delete request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service list' to check deletion status.\n",
		},
		{
			name: "network error",
			args: []string{"service", "delete", "svc-12345", "--confirm"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to delete Service: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "delete", "svc-12345", "--confirm"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().DeleteServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.DeleteServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name:     "wait until deleted",
			synctest: true,
			args:     []string{"service", "delete", "svc-12345", "--confirm"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupDelete(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
					}, nil)
			},
			wantStderr: "🗑️  Delete request accepted for service 'svc-12345'.\n" +
				"⢎  Waiting for service 'svc-12345' to be deleted\n" +
				"✅ Service 'svc-12345' has been successfully deleted.\n",
		},
		{
			name:     "wait timeout",
			synctest: true,
			args:     []string{"service", "delete", "svc-12345", "--confirm"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupDelete(m)
				// The service is never deleted (a 200 keeps the wait
				// polling), so the wait runs to the (virtual) deadline. The
				// non-TTY spinner dedupes repeated messages, so those polls
				// add no stderr lines.
				svc := sampleService()
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &svc,
					}, nil).AnyTimes()
			},
			wantErr: "wait timeout reached after 30m0s - service may still be deleting",
			wantStderr: "🗑️  Delete request accepted for service 'svc-12345'.\n" +
				"⢎  Waiting for service 'svc-12345' to be deleted\n" +
				"❌ Error: wait timeout reached after 30m0s - service may still be deleting\n",
			checks: []checkFunc{checkExitCode(common.ExitTimeout)},
		},
		{
			name:  "rm alias",
			args:  []string{"service", "rm", "svc-12345", "--confirm", "--no-wait"},
			setup: setupDelete,
			wantStderr: "🗑️  Delete request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service list' to check deletion status.\n",
		},
	})
}
