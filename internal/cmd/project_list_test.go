package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestProjectListCmd(t *testing.T) {
	// The harness's client factory reports testProjectID as the active project,
	// so it's the one marked current in the output.
	setupList := func(projects []api.Project) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetProjectsWithResponse(validCtx).
				Return(&api.GetProjectsResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &projects,
				}, nil)
		}
	}
	bothProjects := setupList([]api.Project{
		{ID: "project-other", Name: "Other Project"},
		{ID: testProjectID, Name: "Current Project"},
	})

	runCmdTests(t, []cmdTest{
		{
			name:    "rejects positional args",
			args:    []string{"project", "list", "extra"},
			wantErr: `unknown command "extra" for "tiger project list"`,
		},
		{
			name:    "not logged in",
			args:    []string{"project", "list"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name: "network error",
			args: []string{"project", "list"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetProjectsWithResponse(validCtx).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list projects: connection refused",
		},
		{
			name: "API error",
			args: []string{"project", "list"},
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
			name: "nil response body",
			args: []string{"project", "list"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetProjectsWithResponse(validCtx).
					Return(&api.GetProjectsResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// Only the active project is marked current.
			name:  "table output",
			args:  []string{"project", "list"},
			setup: bothProjects,
			wantStdout: `┌──────────────────┬─────────────────┬─────────┐
│    PROJECT ID    │      NAME       │ CURRENT │
├──────────────────┼─────────────────┼─────────┤
│ project-other    │ Other Project   │         │
│ test-project-123 │ Current Project │ *       │
└──────────────────┴─────────────────┴─────────┘
`,
		},
		{
			name:  "empty list",
			args:  []string{"project", "list"},
			setup: setupList([]api.Project{}),
			wantStdout: `┌────────────┬──────┬─────────┐
│ PROJECT ID │ NAME │ CURRENT │
└────────────┴──────┴─────────┘
`,
		},
		{
			name:  "json output",
			args:  []string{"project", "list", "-o", "json"},
			setup: bothProjects,
			wantStdout: `[
  {
    "id": "project-other",
    "name": "Other Project",
    "current": false
  },
  {
    "id": "test-project-123",
    "name": "Current Project",
    "current": true
  }
]
`,
		},
		{
			name:  "yaml output",
			args:  []string{"project", "list", "-o", "yaml"},
			setup: bothProjects,
			wantStdout: `- current: false
  id: project-other
  name: Other Project
- current: true
  id: test-project-123
  name: Current Project
`,
		},
		{
			name:    "env output rejected by flag",
			args:    []string{"project", "list", "-o", "env"},
			wantErr: `invalid argument "env" for "-o, --output" flag: invalid output format: env (must be one of: json, yaml, table)`,
		},
		{
			// --output rejects env at parse time, but TIGER_OUTPUT isn't
			// validated on load.
			name:    "env output from env var",
			args:    []string{"project", "list"},
			opts:    []runOption{withEnv("TIGER_OUTPUT", "env")},
			setup:   bothProjects,
			wantErr: "environment variable output is not supported for multiple projects",
		},
		{
			name:  "ls alias",
			args:  []string{"project", "ls"},
			setup: bothProjects,
			wantStdout: `┌──────────────────┬─────────────────┬─────────┐
│    PROJECT ID    │      NAME       │ CURRENT │
├──────────────────┼─────────────────┼─────────┤
│ project-other    │ Other Project   │         │
│ test-project-123 │ Current Project │ *       │
└──────────────────┴─────────────────┴─────────┘
`,
		},
	})
}
