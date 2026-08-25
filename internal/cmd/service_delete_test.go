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

	wantExitCode := func(code int) func(t *testing.T, result cmdResult) {
		return func(t *testing.T, result cmdResult) {
			var exitErr common.ExitCodeError
			if !errors.As(result.err, &exitErr) {
				t.Fatalf("expected ExitCodeError, got %T: %v", result.err, result.err)
			}
			if exitErr.ExitCode() != code {
				t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), code)
			}
		}
	}

	confirmPrompt := "Are you sure you want to delete service 'svc-12345'? This operation cannot be undone.\n" +
		"Type the service ID 'svc-12345' to confirm: "

	tests := []cmdTest{
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
			wantErr: "authentication required: not logged in. Please run 'tiger auth login'",
			check:   wantExitCode(common.ExitAuthenticationError),
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
			check:   wantExitCode(common.ExitServiceNotFound),
		},
		{
			name: "wait until deleted",
			args: []string{"service", "delete", "svc-12345", "--confirm"},
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
			name:    "wait timeout",
			args:    []string{"service", "delete", "svc-12345", "--confirm", "--wait-timeout", "1ms"},
			setup:   setupDelete,
			wantErr: "wait timeout reached after 1ms - service may still be deleting",
			wantStderr: "🗑️  Delete request accepted for service 'svc-12345'.\n" +
				"⢎  Waiting for service 'svc-12345' to be deleted\n" +
				"❌ Error: wait timeout reached after 1ms - service may still be deleting\n",
			check: wantExitCode(common.ExitTimeout),
		},
		{
			name:  "rm alias",
			args:  []string{"service", "rm", "svc-12345", "--confirm", "--no-wait"},
			setup: setupDelete,
			wantStderr: "🗑️  Delete request accepted for service 'svc-12345'.\n" +
				"💡 Use 'tiger service list' to check deletion status.\n",
		},
	}

	runCmdTests(t, tests)
}
