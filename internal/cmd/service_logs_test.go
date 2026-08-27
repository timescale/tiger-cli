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

// logsParams matches a *api.GetServiceLogsParams satisfying check.
func logsParams(check func(p *api.GetServiceLogsParams) bool) gomock.Matcher {
	return gomock.Cond(func(x any) bool {
		p, ok := x.(*api.GetServiceLogsParams)
		return ok && p != nil && check(p)
	})
}

// defaultLogsParams matches the params of a `tiger service logs` call with no
// flags: no node, since, or cursor, and an until bound fixed to the current
// time by FetchServiceLogs.
func defaultLogsParams() gomock.Matcher {
	return logsParams(func(p *api.GetServiceLogsParams) bool {
		return p.Node == nil && p.Since == nil && p.Cursor == nil && p.Until != nil
	})
}

func logsResponse(logs *api.ServiceLogs) *api.GetServiceLogsResponse {
	return &api.GetServiceLogsResponse{
		HTTPResponse: httpResponse(http.StatusOK),
		JSON200:      logs,
	}
}

func TestServiceLogsCmd(t *testing.T) {
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

	setupLogs := func(logs api.ServiceLogs) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "svc-12345", defaultLogsParams()).
				Return(logsResponse(&logs), nil)
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "logs", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "no service id",
			args:    []string{"service", "logs"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "invalid since flag",
			args:    []string{"service", "logs", "svc-12345", "--since", "bogus"},
			wantErr: "invalid argument \"bogus\" for \"--since\" flag: invalid time format `bogus` must be one of: `2006-01-02T15:04:05Z07:00`",
		},
		{
			name: "network error",
			args: []string{"service", "logs", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "svc-12345", defaultLogsParams()).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch logs: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "logs", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "svc-12345", defaultLogsParams()).
					Return(&api.GetServiceLogsResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"service", "logs", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "svc-12345", defaultLogsParams()).
					Return(logsResponse(nil), nil)
			},
			wantErr: "unexpected empty response",
		},
		{
			name:  "empty logs",
			args:  []string{"service", "logs", "svc-12345"},
			setup: setupLogs(api.ServiceLogs{}),
		},
		{
			name:  "text output",
			args:  []string{"service", "logs", "svc-12345"},
			setup: setupLogs(api.ServiceLogs{Entries: &entries}),
			wantStdout: "LOG: database system is ready to accept connections\n" +
				"ERROR: relation \"missing\" does not exist\n",
		},
		{
			// Timestamps are rendered in the machine's local timezone, pinned
			// to UTC by withUTC so the expected output stays literal.
			name:  "text output with timestamps",
			args:  []string{"service", "logs", "svc-12345"},
			opts:  []runOption{withUTC()},
			setup: setupLogs(api.ServiceLogs{Entries: &timestampedEntries}),
			wantStdout: "2025-01-15 10:30:00 UTC LOG: checkpoint starting\n" +
				"2025-01-15 10:31:00 UTC LOG: checkpoint complete\n",
		},
		{
			name:  "json output",
			args:  []string{"service", "logs", "svc-12345", "-o", "json"},
			setup: setupLogs(api.ServiceLogs{Entries: &timestampedEntries}),
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
			name:  "yaml output",
			args:  []string{"service", "logs", "svc-12345", "-o", "yaml"},
			setup: setupLogs(api.ServiceLogs{Entries: &timestampedEntries}),
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
			name: "pagination with tail",
			args: []string{"service", "logs", "svc-12345", "--tail", "3"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				page1 := []api.ServiceLogEntry{
					{Message: "entry 5", Severity: "LOG"},
					{Message: "entry 4", Severity: "LOG"},
				}
				page2 := []api.ServiceLogEntry{
					{Message: "entry 3", Severity: "LOG"},
					{Message: "entry 2", Severity: "LOG"},
				}
				gomock.InOrder(
					m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "svc-12345", defaultLogsParams()).
						Return(logsResponse(&api.ServiceLogs{Entries: &page1, LastCursor: new("cursor-1")}), nil),
					m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "svc-12345", logsParams(func(p *api.GetServiceLogsParams) bool {
						return p.Cursor != nil && *p.Cursor == "cursor-1"
					})).
						Return(logsResponse(&api.ServiceLogs{Entries: &page2, LastCursor: new("cursor-2")}), nil),
				)
			},
			wantStdout: "entry 3\nentry 4\nentry 5\n",
		},
		{
			name: "since and until params",
			args: []string{"service", "logs", "svc-12345", "--since", "2024-01-15T09:00:00Z", "--until", "2024-01-15T10:00:00Z"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				since := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
				until := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "svc-12345", logsParams(func(p *api.GetServiceLogsParams) bool {
					return p.Since != nil && p.Since.Equal(since) &&
						p.Until != nil && p.Until.Equal(until) &&
						p.Cursor == nil && p.Node == nil
				})).
					Return(logsResponse(&api.ServiceLogs{Entries: &entries}), nil)
			},
			wantStdout: "LOG: database system is ready to accept connections\n" +
				"ERROR: relation \"missing\" does not exist\n",
		},
		{
			// --node 0 is valid and must be sent explicitly (the parameter is
			// only omitted when the flag isn't set).
			name: "node param",
			args: []string{"service", "logs", "svc-12345", "--node", "0"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "svc-12345", logsParams(func(p *api.GetServiceLogsParams) bool {
					return p.Node != nil && *p.Node == 0
				})).
					Return(logsResponse(&api.ServiceLogs{Entries: &entries}), nil)
			},
			wantStdout: "LOG: database system is ready to accept connections\n" +
				"ERROR: relation \"missing\" does not exist\n",
		},
		{
			name: "default service id from config",
			args: []string{"service", "logs"},
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup: setupLogs(api.ServiceLogs{Entries: &[]api.ServiceLogEntry{
				{Message: "LOG: ready", Severity: "LOG"},
			}}),
			wantStdout: "LOG: ready\n",
		},
		{
			name: "log alias",
			args: []string{"service", "log", "svc-12345"},
			setup: setupLogs(api.ServiceLogs{Entries: &[]api.ServiceLogEntry{
				{Message: "LOG: ready", Severity: "LOG"},
			}}),
			wantStdout: "LOG: ready\n",
		},
	})
}
