package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchUpdatePasswordCmd(t *testing.T) {
	setupUpdate := func(password string) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveBranch(m, "br-12345", sampleBranch())
			m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", api.UpdatePasswordInput{Password: password}).
				Return(&api.UpdateBranchPasswordResponse{HTTPResponse: httpResponse(http.StatusNoContent)}, nil)
		}
	}
	successStderr := "Password saved to system keyring\nPassword updated for tsdbadmin\n"

	runCmdTests(t, []cmdTest{
		{
			name:    "auto-generate with new password env var",
			args:    []string{"branch", "update-password", "br-12345", "--auto-generate"},
			opts:    []runOption{withExperimental(), withEnv("TIGER_NEW_PASSWORD", "secret")},
			wantErr: "cannot use --auto-generate and --new-password together",
		},
		{
			name:    "not logged in",
			args:    []string{"branch", "update-password", "br-12345", "--new-password", "secret"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			// No default fallback: service_id only ever holds a service ID.
			name:    "missing branch id",
			args:    []string{"branch", "update-password", "--new-password", "secret"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: "branch name or ID is required",
		},
		{
			name:    "read-only all refuses",
			args:    []string{"branch", "update-password", "br-12345", "--new-password", "secret"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			args:       []string{"branch", "update-password", "br-12345", "--new-password", "secret"},
			opts:       []runOption{withExperimental(), withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupUpdate("secret"),
			wantStderr: successStderr,
		},
		{
			name:    "non-TTY without password",
			args:    []string{"branch", "update-password", "br-12345"},
			opts:    []runOption{withExperimental()},
			mock:    func(m *mocks.MockClientWithResponsesInterface) { expectResolveBranch(m, "br-12345", sampleBranch()) },
			wantErr: "TTY not detected - use --new-password flag, --auto-generate flag, or TIGER_NEW_PASSWORD environment variable",
		},
		{
			name: "network error",
			args: []string{"branch", "update-password", "br-12345", "--new-password", "secret"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", api.UpdatePasswordInput{Password: "secret"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to update password: connection refused",
		},
		{
			name: "API error",
			args: []string{"branch", "update-password", "br-12345", "--new-password", "secret"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", api.UpdatePasswordInput{Password: "secret"}).
					Return(&api.UpdateBranchPasswordResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.ClientError{Message: new("password too short")},
					}, nil)
			},
			wantErr: "password too short",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			// The endpoint answers success with a 204 and nothing else.
			name: "unexpected 200 is an error",
			args: []string{"branch", "update-password", "br-12345", "--new-password", "secret"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", api.UpdatePasswordInput{Password: "secret"}).
					Return(&api.UpdateBranchPasswordResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "unknown error",
		},
		{
			name:       "new password flag",
			args:       []string{"branch", "update-password", "br-12345", "--new-password", "secret"},
			opts:       []runOption{withExperimental()},
			mock:       setupUpdate("secret"),
			wantStderr: successStderr,
			checks:     []checkFunc{checkStoredPassword("br-12345", "secret")},
		},
		{
			name:       "new password env var",
			args:       []string{"branch", "update-password", "br-12345"},
			opts:       []runOption{withExperimental(), withEnv("TIGER_NEW_PASSWORD", "from-env")},
			mock:       setupUpdate("from-env"),
			wantStderr: successStderr,
			checks:     []checkFunc{checkStoredPassword("br-12345", "from-env")},
		},
		{
			name:       "prompt",
			args:       []string{"branch", "update-password", "br-12345"},
			opts:       []runOption{withExperimental(), withIsTerminal(true), withReadPassword("typed")},
			mock:       setupUpdate("typed"),
			wantStderr: "Enter new password (leave empty to generate): \n" + successStderr,
			checks:     []checkFunc{checkStoredPassword("br-12345", "typed")},
		},
		{
			name:       "auto-generate",
			args:       []string{"branch", "update-password", "br-12345", "--auto-generate"},
			opts:       []runOption{withExperimental(), withGenerateSecurePassword(32, "generated", nil)},
			mock:       setupUpdate("generated"),
			wantStderr: successStderr,
			checks:     []checkFunc{checkStoredPassword("br-12345", "generated")},
		},
		{
			name: "password generation fails",
			args: []string{"branch", "update-password", "br-12345", "--auto-generate"},
			opts: []runOption{withExperimental(), withGenerateSecurePassword(32, "", errors.New("entropy unavailable"))},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
			},
			wantErr: "failed to generate new password: entropy unavailable",
		},
	})
}
