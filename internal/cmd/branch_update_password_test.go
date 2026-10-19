package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

// withGenerateSecurePassword pins the otherwise random generated password,
// asserting the length it is asked for, so the request that carries it can
// be asserted exactly.
func withGenerateSecurePassword(password string) runOption {
	return withSetup(func(t *testing.T) {
		original := common.GenerateSecurePassword
		common.GenerateSecurePassword = func(length int) (string, error) {
			if length != 32 {
				t.Errorf("GenerateSecurePassword length = %d, want 32", length)
			}
			return password, nil
		}
		t.Cleanup(func() { common.GenerateSecurePassword = original })
	})
}

func TestBranchUpdatePasswordCmd(t *testing.T) {
	setupUpdate := func(password string) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetBranch(m, sampleBranch())
			m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "br-12345", api.UpdatePasswordInput{Password: password}).
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
			wantErr: "branch ID is required",
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
			mock:    func(m *mocks.MockClientWithResponsesInterface) { expectGetBranch(m, sampleBranch()) },
			wantErr: "TTY not detected - use --new-password flag, --auto-generate flag, or TIGER_NEW_PASSWORD environment variable",
		},
		{
			name: "network error",
			args: []string{"branch", "update-password", "br-12345", "--new-password", "secret"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetBranch(m, sampleBranch())
				m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "br-12345", api.UpdatePasswordInput{Password: "secret"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to update password: connection refused",
		},
		{
			name: "API error",
			args: []string{"branch", "update-password", "br-12345", "--new-password", "secret"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetBranch(m, sampleBranch())
				m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "br-12345", api.UpdatePasswordInput{Password: "secret"}).
					Return(&api.UpdateBranchPasswordResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.Error{Message: new("password too short")},
					}, nil)
			},
			wantErr: "password too short",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
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
			opts:       []runOption{withExperimental(), withGenerateSecurePassword("generated")},
			mock:       setupUpdate("generated"),
			wantStderr: successStderr,
			checks:     []checkFunc{checkStoredPassword("br-12345", "generated")},
		},
	})
}
