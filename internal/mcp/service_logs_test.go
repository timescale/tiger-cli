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

func TestServiceLogsTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	// FetchServiceLogs pins an absent upper bound to time.Now(). Cases that
	// omit until run under synctest, whose clock always starts at this instant,
	// so the params it sends can be spelled out exactly.
	now := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	defaultParams := &api.GetServiceLogsParams{Until: &now}

	logsResponse := func(logs *api.ServiceLogs) *api.GetServiceLogsResponse {
		return &api.GetServiceLogsResponse{
			HTTPResponse: httpResponse(http.StatusOK),
			JSON200:      logs,
		}
	}
	expectLogs := func(logs api.ServiceLogs) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", defaultParams).
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
			name:    "not logged in",
			tool:    toolServiceLogs,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceLogs,
			args:    map[string]any{"service_id": "not-an-id"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "not-an-id" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "negative node",
			tool:    toolServiceLogs,
			args:    map[string]any{"service_id": "e6ue9697jf", "node": -1},
			wantErr: `validating "arguments": validating root: validating /properties/node: minimum: -1/1 is less than 0.000000`,
		},
		{
			name:    "zero tail",
			tool:    toolServiceLogs,
			args:    map[string]any{"service_id": "e6ue9697jf", "tail": 0},
			wantErr: `validating "arguments": validating root: validating /properties/tail: minimum: 0/1 is less than 1.000000`,
		},
		{
			name:     "network error",
			synctest: true,
			tool:     toolServiceLogs,
			args:     args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", defaultParams).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch logs: connection refused",
		},
		{
			name:     "API error",
			synctest: true,
			tool:     toolServiceLogs,
			args:     args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", defaultParams).
					Return(&api.GetServiceLogsResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name:     "nil response body",
			synctest: true,
			tool:     toolServiceLogs,
			args:     args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", defaultParams).
					Return(logsResponse(nil), nil)
			},
			wantErr: "unexpected empty response",
		},
		{
			name:       "no log entries",
			synctest:   true,
			tool:       toolServiceLogs,
			args:       args,
			mock:       expectLogs(api.ServiceLogs{}),
			wantOutput: map[string]any{"logs": []any{}},
		},
		{
			name:       "returns entries oldest first",
			synctest:   true,
			tool:       toolServiceLogs,
			args:       args,
			mock:       expectLogs(api.ServiceLogs{Entries: &entries}),
			wantOutput: entriesOutput,
		},
		{
			// An entry the API sent without a timestamp renders bare.
			name:     "entry with a zero timestamp has no prefix",
			synctest: true,
			tool:     toolServiceLogs,
			args:     args,
			mock: expectLogs(api.ServiceLogs{Entries: &[]api.ServiceLogEntry{
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
			tool:     toolServiceLogs,
			args:     args,
			mock: expectLogs(api.ServiceLogs{Entries: &[]api.ServiceLogEntry{
				{Message: "LOG: ready", Severity: "LOG", Timestamp: time.Date(2025, 1, 15, 10, 31, 0, 0, time.FixedZone("UTC+2", 2*60*60))},
			}}),
			wantOutput: map[string]any{"logs": []any{"2025-01-15 08:31:00 UTC LOG: ready"}},
		},
		{
			// Two pages: the first returns a cursor, the second must be
			// requested with it. Entries beyond tail are trimmed.
			name:     "pagination trimmed to tail",
			synctest: true,
			tool:     toolServiceLogs,
			args:     map[string]any{"service_id": "e6ue9697jf", "tail": 3},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				page1 := []api.ServiceLogEntry{
					{Message: "entry 5", Severity: "LOG"},
					{Message: "entry 4", Severity: "LOG"},
				}
				page2 := []api.ServiceLogEntry{
					{Message: "entry 3", Severity: "LOG"},
					{Message: "entry 2", Severity: "LOG"},
				}
				gomock.InOrder(
					m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", defaultParams).
						Return(logsResponse(&api.ServiceLogs{Entries: &page1, LastCursor: new("cursor-1")}), nil),
					m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", &api.GetServiceLogsParams{
						Until:  &now,
						Cursor: new("cursor-1"),
					}).
						Return(logsResponse(&api.ServiceLogs{Entries: &page2, LastCursor: new("cursor-2")}), nil),
				)
			},
			wantOutput: map[string]any{"logs": []any{"entry 3", "entry 4", "entry 5"}},
		},
		{
			// An explicit until leaves nothing time-dependent, so no bubble is
			// needed to match the params exactly.
			name: "since and until reach the request",
			tool: toolServiceLogs,
			args: map[string]any{
				"service_id": "e6ue9697jf",
				"since":      "2024-01-15T09:00:00Z",
				"until":      "2024-01-15T10:00:00Z",
			},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				since := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
				until := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", &api.GetServiceLogsParams{
					Since: &since,
					Until: &until,
				}).
					Return(logsResponse(&api.ServiceLogs{Entries: &entries}), nil)
			},
			wantOutput: entriesOutput,
		},
		{
			// node 0 is valid and must be sent, not dropped as a zero value.
			name: "node 0 reaches the request",
			tool: toolServiceLogs,
			args: map[string]any{"service_id": "e6ue9697jf", "node": 0, "until": "2024-01-15T10:00:00Z"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				until := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", &api.GetServiceLogsParams{
					Node:  new(0),
					Until: &until,
				}).
					Return(logsResponse(&api.ServiceLogs{Entries: &entries}), nil)
			},
			wantOutput: entriesOutput,
		},
		{
			name:     "node 2 reaches the request",
			synctest: true,
			tool:     toolServiceLogs,
			args:     map[string]any{"service_id": "e6ue9697jf", "node": 2},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceLogsWithResponse(validCtx, testProjectID, "e6ue9697jf", &api.GetServiceLogsParams{
					Node:  new(2),
					Until: &now,
				}).
					Return(logsResponse(&api.ServiceLogs{Entries: &entries}), nil)
			},
			wantOutput: entriesOutput,
		},
	})
}
