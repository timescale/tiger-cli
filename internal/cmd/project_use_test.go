package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestProjectUseCmd(t *testing.T) {
	// An OAuth login for the harness's current project (testProjectID), which
	// switching away from requires.
	oauthToken := &oauth2.Token{
		AccessToken:  "valid-access-token",
		RefreshToken: "valid-refresh-token",
		Expiry:       time.Now().Add(time.Hour),
	}
	oauthLogin := withStoredCredentials(config.Credentials{
		OAuth:     oauthToken,
		ProjectID: testProjectID,
	})

	setupProjects := func(projects []api.Project) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetProjectsWithResponse(validCtx).
				Return(&api.GetProjectsResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &projects,
				}, nil)
		}
	}
	bothProjects := setupProjects([]api.Project{
		{ID: testProjectID, Name: "Old Project"},
		{ID: "project-new", Name: "New Project"},
	})

	// checkStoredProject asserts which project the stored credentials point at
	// and that the OAuth token survived.
	checkStoredProject := func(want string) func(t *testing.T, result cmdResult) {
		return func(t *testing.T, result cmdResult) {
			t.Helper()
			stored, err := readStoredCredentials(t, result.configDir)
			if err != nil {
				t.Fatalf("failed to get stored credentials: %v", err)
			}
			if stored.ProjectID != want {
				t.Errorf("stored project ID = %q, want %q", stored.ProjectID, want)
			}
			if stored.OAuth == nil || stored.OAuth.AccessToken != "valid-access-token" {
				t.Errorf("expected OAuth token to be preserved, got: %+v", stored.OAuth)
			}
		}
	}

	tests := []cmdTest{
		{
			name:    "rejects missing argument",
			args:    []string{"project", "use"},
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name: "rejects env API keys",
			args: []string{"project", "use", "project-new"},
			opts: []runOption{
				withEnv("TIGER_PUBLIC_KEY", "env-public"),
				withEnv("TIGER_SECRET_KEY", "env-secret"),
			},
			wantErr: "cannot switch projects while TIGER_PUBLIC_KEY/TIGER_SECRET_KEY are set: an API key is scoped to a single project",
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "not logged in",
			args:    []string{"project", "use", "project-new"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			// The client is available (e.g. still cached) but nothing is stored:
			// the command needs the stored OAuth token to rewrite.
			name:    "no stored credentials",
			args:    []string{"project", "use", "project-new"},
			wantErr: "not logged in",
		},
		{
			name: "rejects API key login",
			args: []string{"project", "use", "project-new"},
			opts: []runOption{withStoredCredentials(config.Credentials{
				APIKey:    "pub:sec",
				ProjectID: testProjectID,
			})},
			wantErr: "an API key is scoped to a single project. Run 'tiger auth login' without --public-key/--secret-key",
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name: "already using project",
			args: []string{"project", "use", testProjectID},
			opts: []runOption{
				oauthLogin,
				withConfig(map[string]any{"service_id": "svc-123"}),
			},
			wantStdout: "Already using project " + testProjectID + "\n",
			// A no-op switch keeps the default service.
			checks: []checkFunc{checkDefaultService("svc-123")},
		},
		{
			name: "network error listing projects",
			args: []string{"project", "use", "project-new"},
			opts: []runOption{oauthLogin},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetProjectsWithResponse(validCtx).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list projects: connection refused",
		},
		{
			name: "API error listing projects",
			args: []string{"project", "use", "project-new"},
			opts: []runOption{oauthLogin},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetProjectsWithResponse(validCtx).
					Return(&api.GetProjectsResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
						JSON4XX:      &api.ClientError{Message: new("internal error")},
					}, nil)
			},
			wantErr: "internal error",
			checks:  []checkFunc{checkExitCode(common.ExitGeneralError)},
		},
		{
			name:    "no access to requested project",
			args:    []string{"project", "use", "project-unknown"},
			opts:    []runOption{oauthLogin},
			setup:   setupProjects([]api.Project{{ID: testProjectID, Name: "Old Project"}}),
			wantErr: "no access to the requested project",
			wantStderr: "Project project-unknown is not among your accessible projects\n" +
				"Error: no access to the requested project\n",
			checks: []checkFunc{
				checkExitCode(common.ExitInvalidParameters),
				checkStoredProject(testProjectID),
			},
		},
		{
			name: "switch clears default service",
			args: []string{"project", "use", "project-new"},
			opts: []runOption{
				oauthLogin,
				withConfig(map[string]any{"service_id": "svc-123"}),
			},
			setup:      bothProjects,
			wantStdout: "Switched to project project-new\n",
			wantStderr: "Cleared default service (config key service_id): it belonged to the previous project\n",
			checks: []checkFunc{
				checkStoredProject("project-new"),
				checkDefaultService(""),
			},
		},
		{
			name:       "switch without default service",
			args:       []string{"project", "use", "project-new"},
			opts:       []runOption{oauthLogin},
			setup:      bothProjects,
			wantStdout: "Switched to project project-new\n",
			checks:     []checkFunc{checkStoredProject("project-new")},
		},
		{
			// A default service from the environment can't be cleared, so the
			// command warns that it still points at the previous project.
			name: "switch warns about env default service",
			args: []string{"project", "use", "project-new"},
			opts: []runOption{
				oauthLogin,
				withEnv("TIGER_SERVICE_ID", "svc-env"),
			},
			setup:      bothProjects,
			wantStdout: "Switched to project project-new\n",
			wantStderr: "Warning: the default service from --service-id/TIGER_SERVICE_ID belongs to the previous project and is still in effect\n",
			checks:     []checkFunc{checkStoredProject("project-new")},
		},
		{
			name:       "switch alias",
			args:       []string{"project", "switch", "project-new"},
			opts:       []runOption{oauthLogin},
			setup:      bothProjects,
			wantStdout: "Switched to project project-new\n",
			checks:     []checkFunc{checkStoredProject("project-new")},
		},
	}

	runCmdTests(t, tests)
}
