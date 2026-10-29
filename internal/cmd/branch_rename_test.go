package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestBranchRenameCmd(t *testing.T) {
	setupRename := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveBranch(m, "br-12345", sampleBranch())
		renamed := sampleBranch(func(b *api.Branch) { b.Name = "feature/rbac" })
		m.EXPECT().RenameBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", api.BranchRename{Name: "feature/rbac"}).
			Return(&api.RenameBranchResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &renamed,
			}, nil)
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "missing new name",
			args:    []string{"branch", "rename", "br-12345"},
			opts:    []runOption{withExperimental()},
			wantErr: "accepts 2 arg(s), received 1",
		},
		{
			name:    "empty new name",
			args:    []string{"branch", "rename", "br-12345", "  "},
			opts:    []runOption{withExperimental()},
			wantErr: "new name cannot be empty",
		},
		{
			name:    "not logged in",
			args:    []string{"branch", "rename", "br-12345", "feature/rbac"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only all refuses",
			args:    []string{"branch", "rename", "br-12345", "feature/rbac"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:       "read-only prod allows",
			args:       []string{"branch", "rename", "br-12345", "feature/rbac"},
			opts:       []runOption{withExperimental(), withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupRename,
			wantStdout: "Renamed branch 'test-branch' (br-12345) to 'feature/rbac'.\n",
		},
		{
			name: "network error",
			args: []string{"branch", "rename", "br-12345", "feature/rbac"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().RenameBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", api.BranchRename{Name: "feature/rbac"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to call API: connection refused",
		},
		{
			name: "duplicate name",
			args: []string{"branch", "rename", "br-12345", "feature/rbac"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().RenameBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", api.BranchRename{Name: "feature/rbac"}).
					Return(&api.RenameBranchResponse{
						HTTPResponse: httpResponse(http.StatusConflict),
						JSON4XX:      &api.ClientError{Message: new("a branch named feature/rbac already exists")},
					}, nil)
			},
			wantErr: "a branch named feature/rbac already exists",
		},
		{
			name: "nil response body",
			args: []string{"branch", "rename", "br-12345", "feature/rbac"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().RenameBranchWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", api.BranchRename{Name: "feature/rbac"}).
					Return(&api.RenameBranchResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "success",
			args:       []string{"branch", "rename", "br-12345", "feature/rbac"},
			opts:       []runOption{withExperimental()},
			mock:       setupRename,
			wantStdout: "Renamed branch 'test-branch' (br-12345) to 'feature/rbac'.\n",
		},
	})
}
