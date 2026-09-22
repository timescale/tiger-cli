package mcp

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// withExecuteQuery stubs common.ExecuteQuery — otherwise the point where the
// tool would open a real database connection — asserting the arguments the
// handler passed down and returning result and err in their place. Expectation
// and return value are configured together, the way an API client mock's are,
// so nothing about a case leaks into the next one.
func withExecuteQuery(want common.ExecuteQueryArgs, result *common.QueryResult, err error) runOption {
	return withSetup(func(t *testing.T) {
		original := common.ExecuteQuery
		common.ExecuteQuery = func(_ context.Context, _ *config.Config, _ *common.ConnectionTarget, got common.ExecuteQueryArgs) (*common.QueryResult, error) {
			t.Helper()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("ExecuteQuery args mismatch (-want +got):\n%s", diff)
			}
			return result, err
		}
		t.Cleanup(func() { common.ExecuteQuery = original })
	})
}

func TestDBQuery(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "query": "SELECT 1"}

	setupGetWithStatus := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) { s.Status = status }))
		}
	}

	sqlDir := t.TempDir()
	sqlFile := filepath.Join(sqlDir, "query.sql")
	if err := os.WriteFile(sqlFile, []byte("SELECT * FROM users;\n"), 0o600); err != nil {
		t.Fatalf("failed to write SQL file: %v", err)
	}
	emptyFile := filepath.Join(sqlDir, "empty.sql")
	if err := os.WriteFile(emptyFile, nil, 0o600); err != nil {
		t.Fatalf("failed to write empty SQL file: %v", err)
	}
	missingFile := filepath.Join(sqlDir, "nope.sql")

	const bothOrNeitherMsg = "exactly one of 'query' or 'file' must be provided"

	// The query is stubbed for the success cases below, so they reach the
	// handler's own output without a live database.
	result := &common.QueryResult{
		ResultSets: []common.ResultSet{{
			CommandTag:   "SELECT 1",
			Columns:      []common.Column{{Name: "id", Type: "int4"}},
			Rows:         [][]*string{{new("1")}},
			RowsAffected: 1,
		}},
		ExecutionTime: 12 * time.Millisecond,
	}
	wantResultSets := []any{map[string]any{
		"command_tag":   "SELECT 1",
		"columns":       []any{map[string]any{"name": "id", "type": "int4"}},
		"rows":          []any{[]any{"1"}},
		"rows_affected": float64(1),
	}}
	wantResult := map[string]any{"result_sets": wantResultSets, "execution_time": "12ms"}

	// baseArgs is what the handler builds from a bare query, with the schema
	// defaults the SDK applies and the MCP-only response caps.
	baseArgs := common.ExecuteQueryArgs{
		Query:    "SELECT 1",
		Role:     "tsdbadmin",
		MaxRows:  config.DefaultMCPMaxRows,
		MaxBytes: mcpMaxResponseBytes,
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolDBQuery,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The service_id pattern is enforced by the SDK's schema validation,
			// before the handler runs, so no API call is registered.
			name:    "service id fails schema validation",
			tool:    toolDBQuery,
			args:    map[string]any{"service_id": "not-an-id", "query": "SELECT 1"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "not-an-id" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "negative timeout fails schema validation",
			tool:    toolDBQuery,
			args:    map[string]any{"service_id": "e6ue9697jf", "query": "SELECT 1", "timeout_seconds": -1},
			wantErr: `validating "arguments": validating root: validating /properties/timeout_seconds: minimum: -1/1 is less than 0.000000`,
		},
		{
			name:    "neither query nor file",
			tool:    toolDBQuery,
			args:    map[string]any{"service_id": "e6ue9697jf"},
			wantErr: bothOrNeitherMsg,
		},
		{
			name:    "both query and file",
			tool:    toolDBQuery,
			args:    map[string]any{"service_id": "e6ue9697jf", "query": "SELECT 1", "file": sqlFile},
			wantErr: bothOrNeitherMsg,
		},
		{
			name:    "missing SQL file",
			tool:    toolDBQuery,
			args:    map[string]any{"service_id": "e6ue9697jf", "file": missingFile},
			wantErr: "failed to read SQL file: open " + missingFile + ": no such file or directory",
		},
		{
			name:    "empty SQL file",
			tool:    toolDBQuery,
			args:    map[string]any{"service_id": "e6ue9697jf", "file": emptyFile},
			wantErr: "SQL file " + emptyFile + " is empty",
		},
		{
			// The file is read before the service is looked up, so reaching the
			// connection step proves its contents became the query.
			name:    "SQL file read then stops before connecting",
			tool:    toolDBQuery,
			args:    map[string]any{"service_id": "e6ue9697jf", "file": sqlFile},
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			wantErr: noEndpointMsg,
		},
		{
			name: "service lookup network error",
			tool: toolDBQuery,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "service lookup API error",
			tool: toolDBQuery,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "nil response body",
			tool: toolDBQuery,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// Both services are fetched: the replica for its endpoint, the
			// parent for its credentials. The replica's own status then stops it.
			name: "read replica resolves its parent",
			tool: toolDBQuery,
			args: map[string]any{"service_id": "u8me885b93", "query": "SELECT 1"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "u8me885b93", sampleReplica())
				expectGetService(m, "e6ue9697jf", sampleService())
			},
			wantErr: pausedMsg,
		},
		{
			name: "read replica parent lookup fails",
			tool: toolDBQuery,
			args: map[string]any{"service_id": "u8me885b93", "query": "SELECT 1"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "u8me885b93", sampleReplica())
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: `failed to fetch parent service "e6ue9697jf" for read replica: service not found`,
		},
		{
			name:    "service paused",
			tool:    toolDBQuery,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusPAUSED),
			wantErr: pausedMsg,
		},
		{
			name:    "service pausing",
			tool:    toolDBQuery,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusPAUSING),
			wantErr: pausedMsg,
		},
		{
			name:    "service not ready",
			tool:    toolDBQuery,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusQUEUED),
			wantErr: notReadyMsg,
		},
		{
			name: "pooled without a pooler",
			tool: toolDBQuery,
			args: map[string]any{"service_id": "e6ue9697jf", "query": "SELECT 1", "pooled": true},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) {
					s.Endpoint = &api.Endpoint{
						Host: new("e6ue9697jf.project.tsdb.cloud.timescale.com"),
						Port: new(5432),
					}
				}))
			},
			wantErr: "connection pooler not available for this service",
		},
		{
			// A query failure passes through handleDatabaseError unchanged.
			name:    "query fails",
			tool:    toolDBQuery,
			args:    args,
			opts:    []runOption{withExecuteQuery(baseArgs, nil, errors.New("failed to connect to database: no route to host"))},
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			wantErr: "failed to connect to database: no route to host",
		},
		{
			name:       "returns the result sets",
			tool:       toolDBQuery,
			args:       args,
			opts:       []runOption{withExecuteQuery(baseArgs, result, nil)},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: wantResult,
		},
		{
			name: "passes parameters, role and pooling down to the query",
			tool: toolDBQuery,
			args: map[string]any{
				"service_id":      "e6ue9697jf",
				"query":           "SELECT $1::int",
				"parameters":      []any{"1"},
				"timeout_seconds": 60,
				"role":            "readonly",
				"pooled":          true,
			},
			opts: []runOption{withExecuteQuery(common.ExecuteQueryArgs{
				Query:      "SELECT $1::int",
				Parameters: []string{"1"},
				Role:       "readonly",
				Pooled:     true,
				MaxRows:    config.DefaultMCPMaxRows,
				MaxBytes:   mcpMaxResponseBytes,
			}, result, nil)},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) {
					s.ConnectionPooler = &api.ConnectionPooler{
						Endpoint: &api.Endpoint{
							Host: new("e6ue9697jf.project.tsdb.cloud.timescale.com"),
							Port: new(6432),
						},
					}
				}))
			},
			wantOutput: wantResult,
		},
		{
			// The file's contents become the query text.
			name: "runs the query read from a file",
			tool: toolDBQuery,
			args: map[string]any{"service_id": "e6ue9697jf", "file": sqlFile},
			opts: []runOption{withExecuteQuery(common.ExecuteQueryArgs{
				Query:    "SELECT * FROM users;\n",
				Role:     "tsdbadmin",
				MaxRows:  config.DefaultMCPMaxRows,
				MaxBytes: mcpMaxResponseBytes,
			}, result, nil)},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: wantResult,
		},
		{
			// db_query isn't a read-only gated tool: the mode opens the session
			// read-only rather than refusing the call.
			name: "read-only all runs the query in a read-only session",
			tool: toolDBQuery,
			args: args,
			opts: []runOption{
				withConfig(map[string]any{"read_only": "all"}),
				withExecuteQuery(common.ExecuteQueryArgs{
					Query:    "SELECT 1",
					Role:     "tsdbadmin",
					ReadOnly: true,
					MaxRows:  config.DefaultMCPMaxRows,
					MaxBytes: mcpMaxResponseBytes,
				}, result, nil),
			},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: wantResult,
		},
		{
			name: "mcp_max_rows caps the rows per result set",
			tool: toolDBQuery,
			args: args,
			opts: []runOption{
				withConfig(map[string]any{"mcp_max_rows": 250}),
				withExecuteQuery(common.ExecuteQueryArgs{
					Query:    "SELECT 1",
					Role:     "tsdbadmin",
					MaxRows:  250,
					MaxBytes: mcpMaxResponseBytes,
				}, result, nil),
			},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: wantResult,
		},
		{
			// A config-file or TIGER_MCP_MAX_ROWS value bypasses `tiger config
			// set` validation, so a non-positive one can reach the handler and
			// must fall back to the default rather than meaning "no cap".
			name: "non-positive mcp_max_rows falls back to the default",
			tool: toolDBQuery,
			args: args,
			opts: []runOption{
				withConfig(map[string]any{"mcp_max_rows": 0}),
				withExecuteQuery(baseArgs, result, nil),
			},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: wantResult,
		},
		{
			// A truncated result carries the notice naming the configured cap.
			name: "truncated results carry an actionable notice",
			tool: toolDBQuery,
			args: args,
			opts: []runOption{withExecuteQuery(baseArgs, &common.QueryResult{
				ResultSets: []common.ResultSet{{
					CommandTag:   "SELECT 100",
					Columns:      []common.Column{{Name: "id", Type: "int4"}},
					Rows:         [][]*string{{new("1")}},
					RowsAffected: 5000,
					Truncated:    true,
				}},
				ExecutionTime: 12 * time.Millisecond,
				Truncated:     true,
			}, nil)},
			mock: setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: map[string]any{
				"result_sets": []any{map[string]any{
					"command_tag":   "SELECT 100",
					"columns":       []any{map[string]any{"name": "id", "type": "int4"}},
					"rows":          []any{[]any{"1"}},
					"rows_affected": float64(5000),
					"truncated":     true,
				}},
				"execution_time": "12ms",
				"truncated":      true,
				"notice": "Results were truncated to limit the amount of data returned (the configured mcp_max_rows=100 per result set, " +
					"plus an overall response size cap). More rows exist. Do the work in the database instead of re-running this query: " +
					"aggregate (GROUP BY, COUNT, SUM, AVG), filter (WHERE), or paginate (LIMIT/OFFSET).",
			},
		},
		{
			// The replica has no pooler, so the warning rides along with a
			// successful result — the one path it is ever visible on.
			name: "read replica without a pooler warns alongside the results",
			tool: toolDBQuery,
			args: map[string]any{"service_id": "u8me885b93", "query": "SELECT 1", "pooled": true},
			opts: []runOption{withExecuteQuery(common.ExecuteQueryArgs{
				Query:    "SELECT 1",
				Role:     "tsdbadmin",
				Pooled:   true,
				MaxRows:  config.DefaultMCPMaxRows,
				MaxBytes: mcpMaxResponseBytes,
			}, result, nil)},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "u8me885b93", sampleReplica(func(s *api.Service) { s.Status = api.DeployStatusREADY }))
				expectGetService(m, "e6ue9697jf", sampleService())
			},
			wantOutput: map[string]any{
				"result_sets":    wantResultSets,
				"execution_time": "12ms",
				"warning":        `read replica "replica-service" has no connection pooler; connecting directly instead`,
			},
		},
	})
}
