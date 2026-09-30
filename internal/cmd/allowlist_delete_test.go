package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestAllowListDeleteCmd(t *testing.T) {
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	setupDelete := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().DeleteAllowListWithResponse(validCtx, testProjectID, "1234567890").
			Return(&api.DeleteAllowListResponse{
				HTTPResponse: httpResponse(http.StatusNoContent),
			}, nil)
	}

	confirmPrompt := "Are you sure you want to delete IP allow list '1234567890'? This operation cannot be undone.\n" +
		"Type the IP allow list ID '1234567890' to confirm: "

	runCmdTests(t, []cmdTest{
		{
			name:    "missing allow list id",
			args:    []string{"allowlist", "delete"},
			opts:    []runOption{experimental},
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "not logged in",
			args:    []string{"allowlist", "delete", "1234567890"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only all refuses",
			args:    []string{"allowlist", "delete", "1234567890"},
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "non-TTY without confirm",
			args:    []string{"allowlist", "delete", "1234567890"},
			opts:    []runOption{experimental},
			wantErr: "TTY not detected - cannot prompt for confirmation. Use --confirm to skip the prompt",
		},
		{
			name:       "confirmation mismatch",
			args:       []string{"allowlist", "delete", "1234567890"},
			opts:       []runOption{experimental, withIsTerminal(true), withStdin("9999999999\n")},
			wantStderr: confirmPrompt + "Delete operation cancelled.\n",
		},
		{
			name:       "confirmation match",
			args:       []string{"allowlist", "delete", "1234567890"},
			opts:       []runOption{experimental, withIsTerminal(true), withStdin("1234567890\n")},
			mock:       setupDelete,
			wantStderr: confirmPrompt + "IP allow list '1234567890' deleted.\n",
		},
		{
			name:       "confirm flag skips prompt",
			args:       []string{"allowlist", "delete", "1234567890", "--confirm"},
			opts:       []runOption{experimental},
			mock:       setupDelete,
			wantStderr: "IP allow list '1234567890' deleted.\n",
		},
		{
			name: "network error",
			args: []string{"allowlist", "delete", "1234567890", "--confirm"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().DeleteAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to delete IP allow list: connection refused",
		},
		{
			name: "still attached refused",
			args: []string{"allowlist", "delete", "1234567890", "--confirm"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().DeleteAllowListWithResponse(validCtx, testProjectID, "1234567890").
					Return(&api.DeleteAllowListResponse{
						HTTPResponse: httpResponse(http.StatusConflict),
						JSON4XX:      &api.ClientError{Message: new("IP allow list is still attached to a service")},
					}, nil)
			},
			wantErr: "IP allow list is still attached to a service",
			checks:  []checkFunc{checkExitCode(common.ExitGeneralError)},
		},
		{
			name:       "rm alias",
			args:       []string{"allowlist", "rm", "1234567890", "--confirm"},
			opts:       []runOption{experimental},
			mock:       setupDelete,
			wantStderr: "IP allow list '1234567890' deleted.\n",
		},
	})
}
