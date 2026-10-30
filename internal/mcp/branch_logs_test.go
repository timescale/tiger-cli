package mcp

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestBranchLogsTool(t *testing.T) {
	experimental := withExperimental()
	args := map[string]any{"branch_id": "br4nch1d9x"}

	// FetchBranchLogs pins an absent upper bound to time.Now(). Cases that
	// omit until run under synctest, whose clock always starts at this instant,
	// so the params it sends can be spelled out exactly.
	now := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	defaultParams := &api.GetBranchLogsParams{Until: &now}

	logsResponse := func(logs *api.BranchLogs) *api.GetBranchLogsResponse {
		return &api.GetBranchLogsResponse{
			HTTPResponse: httpResponse(http.StatusOK),
			JSON200:      logs,
		}
	}
	expectLogs := func(logs api.BranchLogs) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveBranch(m, sampleBranch())
			m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", defaultParams).
				Return(logsResponse(&logs), nil)
		}
	}

	// The API returns entries newest-first; the tool returns them oldest-first.
	entries := []api.ServiceLogEntry{
		{Message: "LOG: checkpoint complete", Severity: "LOG", Timestamp: time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)},
		{Message: `ERROR: relation "missing" does not exist`, Severity: "ERROR", Timestamp: time.Date(2025, 1, 15, 10, 30, 30, 0, time.UTC)},
		{Message: "LOG: database system is ready to accept connections", Severity: "LOG", Timestamp: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)},
	}
	entriesOutput := map[string]any{"logs": []any{
		"2025-01-15 10:30:00 UTC LOG: database system is ready to accept connections",
		`2025-01-15 10:30:30 UTC ERROR: relation "missing" does not exist`,
		"2025-01-15 10:31:00 UTC LOG: checkpoint complete",
	}}

	runToolTests(t, []toolTest{
		{
			name:        "not registered without the experimental gate",
			tool:        toolBranchLogs,
			args:        args,
			wantCallErr: `calling "tools/call": unknown tool "branch_logs"`,
		},
		{
			name:    "not logged in",
			tool:    toolBranchLogs,
			args:    args,
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "rejects a malformed branch ID",
			tool:    toolBranchLogs,
			args:    map[string]any{"branch_id": "nope"},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/branch_id: pattern: "nope" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "zero tail",
			tool:    toolBranchLogs,
			args:    map[string]any{"branch_id": "br4nch1d9x", "tail": 0},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/tail: minimum: 0/1 is less than 1.000000`,
		},
		{
			// The time parameters are optional strings, not nullable ones.
			name:    "null since",
			tool:    toolBranchLogs,
			args:    map[string]any{"branch_id": "br4nch1d9x", "since": nil},
			opts:    []runOption{experimental},
			wantErr: `validating "arguments": validating root: validating /properties/since: type: <invalid reflect.Value> has type "null", want "string"`,
		},
		{
			name: "branch not found",
			tool: toolBranchLogs,
			args: args,
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveStatus(m, "br4nch1d9x", branchType, http.StatusNotFound, &api.ClientError{Message: new("resource not found")})
			},
			wantErr: "branch 'br4nch1d9x' not found",
		},
		{
			name:     "network error",
			synctest: true,
			tool:     toolBranchLogs,
			args:     args,
			opts:     []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", defaultParams).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch logs: connection refused",
		},
		{
			name:     "API error",
			synctest: true,
			tool:     toolBranchLogs,
			args:     args,
			opts:     []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", defaultParams).
					Return(&api.GetBranchLogsResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSON4XX:      &api.ClientError{Message: new("forbidden")},
					}, nil)
			},
			wantErr: "forbidden",
		},
		{
			name:     "nil response body",
			synctest: true,
			tool:     toolBranchLogs,
			args:     args,
			opts:     []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", defaultParams).
					Return(logsResponse(nil), nil)
			},
			wantErr: "unexpected empty response",
		},
		{
			name:       "no log entries",
			synctest:   true,
			tool:       toolBranchLogs,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectLogs(api.BranchLogs{Entries: []api.ServiceLogEntry{}}),
			wantOutput: map[string]any{"logs": []any{}},
		},
		{
			name:       "returns entries oldest first",
			synctest:   true,
			tool:       toolBranchLogs,
			args:       args,
			opts:       []runOption{experimental},
			mock:       expectLogs(api.BranchLogs{Entries: entries}),
			wantOutput: entriesOutput,
		},
		{
			// An entry the API sent without a timestamp renders bare.
			name:     "entry with a zero timestamp has no prefix",
			synctest: true,
			tool:     toolBranchLogs,
			args:     args,
			opts:     []runOption{experimental},
			mock: expectLogs(api.BranchLogs{Entries: []api.ServiceLogEntry{
				{Message: "LOG: checkpoint complete", Severity: "LOG", Timestamp: time.Date(2025, 1, 15, 10, 31, 0, 0, time.UTC)},
				{Message: "LOG: no timestamp", Severity: "LOG"},
			}}),
			wantOutput: map[string]any{"logs": []any{
				"LOG: no timestamp",
				"2025-01-15 10:31:00 UTC LOG: checkpoint complete",
			}},
		},
		{
			// A non-UTC timestamp is converted before formatting.
			name:     "non-UTC timestamp is rendered in UTC",
			synctest: true,
			tool:     toolBranchLogs,
			args:     args,
			opts:     []runOption{experimental},
			mock: expectLogs(api.BranchLogs{Entries: []api.ServiceLogEntry{
				{Message: "LOG: ready", Severity: "LOG", Timestamp: time.Date(2025, 1, 15, 10, 31, 0, 0, time.FixedZone("UTC+2", 2*60*60))},
			}}),
			wantOutput: map[string]any{"logs": []any{"2025-01-15 08:31:00 UTC LOG: ready"}},
		},
		{
			// Two pages: the first returns a cursor, the second must be
			// requested with it. Entries beyond tail are trimmed.
			name:     "pagination trimmed to tail",
			synctest: true,
			tool:     toolBranchLogs,
			args:     map[string]any{"branch_id": "br4nch1d9x", "tail": 3},
			opts:     []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				page1 := []api.ServiceLogEntry{
					{Message: "entry 5", Severity: "LOG"},
					{Message: "entry 4", Severity: "LOG"},
				}
				page2 := []api.ServiceLogEntry{
					{Message: "entry 3", Severity: "LOG"},
					{Message: "entry 2", Severity: "LOG"},
				}
				gomock.InOrder(
					m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", defaultParams).
						Return(logsResponse(&api.BranchLogs{Entries: page1, LastCursor: new("cursor-1")}), nil),
					m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", &api.GetBranchLogsParams{
						Until:  &now,
						Cursor: new("cursor-1"),
					}).
						Return(logsResponse(&api.BranchLogs{Entries: page2, LastCursor: new("cursor-2")}), nil),
				)
			},
			wantOutput: map[string]any{"logs": []any{"entry 3", "entry 4", "entry 5"}},
		},
		{
			// An explicit until leaves nothing time-dependent, so no bubble is
			// needed to match the params exactly.
			name: "since and until reach the request",
			tool: toolBranchLogs,
			args: map[string]any{
				"branch_id": "br4nch1d9x",
				"since":     "2024-01-15T09:00:00Z",
				"until":     "2024-01-15T10:00:00Z",
			},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				since := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
				until := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", &api.GetBranchLogsParams{
					Since: &since,
					Until: &until,
				}).
					Return(logsResponse(&api.BranchLogs{Entries: entries}), nil)
			},
			wantOutput: entriesOutput,
		},
		{
			// Without until, the upper bound is pinned to the current time.
			name:     "since alone reaches the request",
			synctest: true,
			tool:     toolBranchLogs,
			args:     map[string]any{"branch_id": "br4nch1d9x", "since": "2024-01-15T09:00:00Z"},
			opts:     []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				since := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", &api.GetBranchLogsParams{
					Since: &since,
					Until: &now,
				}).
					Return(logsResponse(&api.BranchLogs{Entries: entries}), nil)
			},
			wantOutput: entriesOutput,
		},
		{
			// Without since, no lower bound is sent.
			name: "until alone reaches the request",
			tool: toolBranchLogs,
			args: map[string]any{"branch_id": "br4nch1d9x", "until": "2024-01-15T10:00:00Z"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveBranch(m, sampleBranch())
				until := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
				m.EXPECT().GetBranchLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", "br4nch1d9x", &api.GetBranchLogsParams{
					Until: &until,
				}).
					Return(logsResponse(&api.BranchLogs{Entries: entries}), nil)
			},
			wantOutput: entriesOutput,
		},
	})
}
