package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

// defaultBranchLogsParams are the params of a `tiger branch logs` call with no
// flags, under synctest (see logsNow).
func defaultBranchLogsParams() *api.GetBranchLogsParams {
	return &api.GetBranchLogsParams{Until: &logsNow}
}

func branchLogsResponse(logs *api.BranchLogs) *api.GetBranchLogsResponse {
	return &api.GetBranchLogsResponse{
		HTTPResponse: httpResponse(http.StatusOK),
		JSON200:      logs,
	}
}

func TestBranchLogsCmd(t *testing.T) {
	// The API returns entries newest-first; the command prints them
	// oldest-first. Zero timestamps render as the bare message.
	entries := []api.ServiceLogEntry{
		{Message: "ERROR: relation \"missing\" does not exist", Severity: "ERROR"},
		{Message: "LOG: database system is ready to accept connections", Severity: "LOG"},
	}

	ts1 := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	ts2 := time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)
	timestampedEntries := []api.ServiceLogEntry{
		{Message: "LOG: checkpoint complete", Severity: "LOG", Timestamp: ts2},
		{Message: "LOG: checkpoint starting", Severity: "LOG", Timestamp: ts1},
	}

	setupLogs := func(logs api.BranchLogs) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveBranch(m, "br-12345", sampleBranch())
			m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", defaultBranchLogsParams()).
				Return(branchLogsResponse(&logs), nil)
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"branch", "logs", "br-12345"},
			opts:    []runOption{withExperimental(), withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			// No default fallback: service_id only ever holds a service ID.
			name:    "missing branch id",
			args:    []string{"branch", "logs"},
			opts:    []runOption{withExperimental(), withConfig(map[string]any{"service_id": "svc-12345"})},
			wantErr: "branch name or ID is required",
		},
		{
			name:    "invalid since flag",
			args:    []string{"branch", "logs", "br-12345", "--since", "bogus"},
			opts:    []runOption{withExperimental()},
			wantErr: "invalid argument \"bogus\" for \"--since\" flag: invalid time format `bogus` must be one of: `2006-01-02T15:04:05Z07:00`",
		},
		{
			name: "branch not found",
			args: []string{"branch", "logs", "br-12345"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranchStatus(m, "br-12345", http.StatusNotFound, &api.ClientError{Message: new("branch not found")})
			},
			wantErr: "branch 'br-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "ambiguous name refused",
			args: []string{"branch", "logs", "ci-4821"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranchStatus(m, "ci-4821", http.StatusBadRequest, &api.ClientError{Message: new("ref matches multiple branches")})
			},
			wantErr: "ref matches multiple branches\nRun 'tiger branch list' on the branch's service to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name:     "network error",
			args:     []string{"branch", "logs", "br-12345"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", defaultBranchLogsParams()).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch logs: connection refused",
		},
		{
			name:     "API error",
			args:     []string{"branch", "logs", "br-12345"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", defaultBranchLogsParams()).
					Return(&api.GetBranchLogsResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSON4XX:      &api.ClientError{Message: new("forbidden")},
					}, nil)
			},
			wantErr: "forbidden",
			checks:  []checkFunc{checkExitCode(common.ExitPermissionDenied)},
		},
		{
			name:     "nil response body",
			args:     []string{"branch", "logs", "br-12345"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", defaultBranchLogsParams()).
					Return(branchLogsResponse(nil), nil)
			},
			wantErr: "unexpected empty response",
		},
		{
			name:     "empty logs",
			args:     []string{"branch", "logs", "br-12345"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock:     setupLogs(api.BranchLogs{Entries: []api.ServiceLogEntry{}}),
		},
		{
			name:       "empty logs json output",
			args:       []string{"branch", "logs", "br-12345", "-o", "json"},
			opts:       []runOption{withExperimental()},
			synctest:   true,
			mock:       setupLogs(api.BranchLogs{}),
			wantStdout: "[]\n",
		},
		{
			name:       "empty logs yaml output",
			args:       []string{"branch", "logs", "br-12345", "-o", "yaml"},
			opts:       []runOption{withExperimental()},
			synctest:   true,
			mock:       setupLogs(api.BranchLogs{}),
			wantStdout: "[]\n",
		},
		{
			name:     "text output",
			args:     []string{"branch", "logs", "br-12345"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock:     setupLogs(api.BranchLogs{Entries: entries}),
			wantStdout: "LOG: database system is ready to accept connections\n" +
				"ERROR: relation \"missing\" does not exist\n",
		},
		{
			// Timestamps are rendered in the local timezone, pinned to UTC in
			// TestMain so the expected output stays literal.
			name:     "text output with timestamps",
			args:     []string{"branch", "logs", "br-12345"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock:     setupLogs(api.BranchLogs{Entries: timestampedEntries}),
			wantStdout: "2025-01-15 10:30:00 UTC LOG: checkpoint starting\n" +
				"2025-01-15 10:31:00 UTC LOG: checkpoint complete\n",
		},
		{
			name:     "json output",
			args:     []string{"branch", "logs", "br-12345", "-o", "json"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock:     setupLogs(api.BranchLogs{Entries: timestampedEntries}),
			wantStdout: `[
  {
    "message": "LOG: checkpoint starting",
    "severity": "LOG",
    "timestamp": "2025-01-15T10:30:00Z"
  },
  {
    "message": "LOG: checkpoint complete",
    "severity": "LOG",
    "timestamp": "2025-01-15T10:31:00Z"
  }
]
`,
		},
		{
			name:     "yaml output",
			args:     []string{"branch", "logs", "br-12345", "-o", "yaml"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock:     setupLogs(api.BranchLogs{Entries: timestampedEntries}),
			wantStdout: `- message: 'LOG: checkpoint starting'
  severity: LOG
  timestamp: "2025-01-15T10:30:00Z"
- message: 'LOG: checkpoint complete'
  severity: LOG
  timestamp: "2025-01-15T10:31:00Z"
`,
		},
		{
			// Two pages: the first returns a cursor, the second must be
			// requested with it. Entries beyond --tail are trimmed.
			name:     "pagination with tail",
			args:     []string{"branch", "logs", "br-12345", "--tail", "3"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				page1 := []api.ServiceLogEntry{
					{Message: "entry 5", Severity: "LOG"},
					{Message: "entry 4", Severity: "LOG"},
				}
				page2 := []api.ServiceLogEntry{
					{Message: "entry 3", Severity: "LOG"},
					{Message: "entry 2", Severity: "LOG"},
				}
				gomock.InOrder(
					m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", defaultBranchLogsParams()).
						Return(branchLogsResponse(&api.BranchLogs{Entries: page1, LastCursor: new("cursor-1")}), nil),
					m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", &api.GetBranchLogsParams{
						Until:  &logsNow,
						Cursor: new("cursor-1"),
					}).
						Return(branchLogsResponse(&api.BranchLogs{Entries: page2, LastCursor: new("cursor-2")}), nil),
				)
			},
			wantStdout: "entry 3\nentry 4\nentry 5\n",
		},
		{
			name: "since and until params",
			args: []string{"branch", "logs", "br-12345", "--since", "2024-01-15T09:00:00Z", "--until", "2024-01-15T10:00:00Z"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, "br-12345", sampleBranch())
				since := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
				until := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "svc-12345", "br-12345", &api.GetBranchLogsParams{
					Since: &since,
					Until: &until,
				}).
					Return(branchLogsResponse(&api.BranchLogs{Entries: entries}), nil)
			},
			wantStdout: "LOG: database system is ready to accept connections\n" +
				"ERROR: relation \"missing\" does not exist\n",
		},
		{
			name:     "log alias",
			args:     []string{"branch", "log", "br-12345"},
			opts:     []runOption{withExperimental()},
			synctest: true,
			mock: setupLogs(api.BranchLogs{Entries: []api.ServiceLogEntry{
				{Message: "LOG: ready", Severity: "LOG"},
			}}),
			wantStdout: "LOG: ready\n",
		},
	})
}
