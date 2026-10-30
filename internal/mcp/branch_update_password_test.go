package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchUpdatePasswordTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"branch_id": "br4nch1d9x"}
	generated := withGenerateSecurePassword(32, "generated-pw", nil)
	expectUpdate := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveBranch(m, sampleBranch())
		m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", api.UpdatePasswordInput{Password: "generated-pw"}).
			Return(&api.UpdateBranchPasswordResponse{HTTPResponse: httpResponse(http.StatusNoContent)}, nil)
	}
	storedPassword := map[string]any{
		"success": true,
		"method":  "keyring",
		"message": "Password saved to system keyring",
	}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchUpdatePassword,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_update_password"`,
		},
		{
			name:    "read-only all refuses without an API call",
			tool:    toolBranchUpdatePassword,
			args:    args,
			opts:    []runOption{experimental, withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// A branch always counts as DEV, so prod neither refuses nor
			// prompts.
			name: "read-only prod allows",
			tool: toolBranchUpdatePassword,
			args: args,
			opts: []runOption{experimental, generated, withConfig(map[string]any{"read_only": "prod"})},
			mock: expectUpdate,
			wantOutput: map[string]any{
				"message":          "Password updated for tsdbadmin.",
				"password_storage": storedPassword,
			},
		},
		{
			name: "branch not found",
			tool: toolBranchUpdatePassword,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveStatus(m, "br4nch1d9x", branchType, http.StatusNotFound, &api.ClientError{Message: new("resource not found")})
			},
			wantErr: "branch 'br4nch1d9x' not found",
		},
		{
			name: "network error",
			tool: toolBranchUpdatePassword,
			args: args,
			opts: []runOption{experimental, generated},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", api.UpdatePasswordInput{Password: "generated-pw"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to update branch password: connection refused",
		},
		{
			name: "password generation fails",
			tool: toolBranchUpdatePassword,
			args: args,
			opts: []runOption{experimental, withGenerateSecurePassword(32, "", errors.New("failed to generate random password: entropy unavailable"))},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
			},
			wantErr: "failed to generate random password: entropy unavailable",
		},
		{
			// The endpoint answers success with a 204 and nothing else.
			name: "unexpected 200 is an error",
			tool: toolBranchUpdatePassword,
			args: args,
			opts: []runOption{experimental, generated},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().UpdateBranchPasswordWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", api.UpdatePasswordInput{Password: "generated-pw"}).
					Return(&api.UpdateBranchPasswordResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "unknown error",
		},
		{
			name: "saves the generated password",
			tool: toolBranchUpdatePassword,
			args: args,
			opts: []runOption{experimental, generated},
			mock: expectUpdate,
			wantOutput: map[string]any{
				"message":          "Password updated for tsdbadmin.",
				"password_storage": storedPassword,
			},
			checks: []checkFunc{checkStoredPassword("br4nch1d9x", "generated-pw")},
		},
		{
			name: "with_password returns the password",
			tool: toolBranchUpdatePassword,
			args: map[string]any{"branch_id": "br4nch1d9x", "with_password": true},
			opts: []runOption{experimental, generated},
			mock: expectUpdate,
			wantOutput: map[string]any{
				"message":          "Password updated for tsdbadmin.",
				"password":         "generated-pw",
				"password_storage": storedPassword,
			},
		},
		{
			name: "warns when the password was neither saved nor returned",
			tool: toolBranchUpdatePassword,
			args: args,
			opts: []runOption{experimental, generated, withConfig(map[string]any{"password_storage": "none"})},
			mock: expectUpdate,
			wantOutput: map[string]any{
				"message": "Password updated for tsdbadmin. Warning: the new password was not saved, and was not returned because with_password was false.",
				"password_storage": map[string]any{
					"success": false,
					"method":  "none",
					"message": "Password not saved (--password-storage=none). Make sure to store it securely.",
				},
			},
		},
	})
}
