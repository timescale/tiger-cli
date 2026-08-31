package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// withExecuteQuery stubs common.ExecuteQuery, which is otherwise the point
// where the command would open a real database connection. The stub records
// what the command asked for so a case can assert on it, and hands back result.
func withExecuteQuery(result *common.QueryResult, err error, record ...*common.ExecuteQueryArgs) runOption {
	return withSetup(func(t *testing.T) {
		original := common.ExecuteQuery
		common.ExecuteQuery = func(_ context.Context, _ *config.Config, _ *common.ConnectionTarget, args common.ExecuteQueryArgs) (*common.QueryResult, error) {
			for _, into := range record {
				*into = args
			}
			return result, err
		}
		t.Cleanup(func() { common.ExecuteQuery = original })
	})
}

// queryRows builds the row list of a result set from plain strings, with "" for
// a SQL NULL.
func queryRows(rows ...[]string) *[][]*string {
	out := make([][]*string, 0, len(rows))
	for _, row := range rows {
		values := make([]*string, len(row))
		for i, val := range row {
			if val != "" {
				values[i] = new(val)
			}
		}
		out = append(out, values)
	}
	return &out
}

// selectResult is a two-column, two-row SELECT result, the second row's name
// being NULL.
func selectResult() *common.QueryResult {
	return &common.QueryResult{
		ResultSets: []common.ResultSet{{
			CommandTag: "SELECT 2",
			Columns: []common.Column{
				{Name: "id", Type: "int4"},
				{Name: "name", Type: "text"},
			},
			Rows:         queryRows([]string{"1", "alice"}, []string{"2", ""}),
			RowsAffected: 2,
		}},
		ExecutionTime: 12 * time.Millisecond,
	}
}

func TestDbQueryCmd(t *testing.T) {
	setupGetService := func(m *mocks.MockClientWithResponsesInterface) {
		expectGetService(m, "svc-12345", sampleService())
	}

	sqlDir := t.TempDir()
	sqlFile := filepath.Join(sqlDir, "query.sql")
	if err := os.WriteFile(sqlFile, []byte("SELECT * FROM users"), 0o600); err != nil {
		t.Fatalf("failed to write SQL file: %v", err)
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// An explicit empty --command is an empty query, not a signal to
			// fall back to stdin.
			name:    "empty command flag",
			args:    []string{"db", "query", "svc-12345", "-c", ""},
			setup:   setupGetService,
			opts:    []runOption{withStdin("SELECT 'from stdin'")},
			wantErr: "query cannot be empty",
		},
		{
			name:    "empty stdin",
			args:    []string{"db", "query", "svc-12345"},
			setup:   setupGetService,
			opts:    []runOption{withStdin("")},
			wantErr: "query cannot be empty",
		},
		{
			name:    "negative timeout",
			args:    []string{"db", "query", "svc-12345", "-c", "SELECT 1", "--timeout", "-5s"},
			wantErr: "timeout must be positive or zero, got -5s",
		},
		{
			name:    "command and file are mutually exclusive",
			args:    []string{"db", "query", "svc-12345", "-c", "SELECT 1", "-f", sqlFile},
			wantErr: "if any flags in the group [command file] are set none of the others can be; [command file] were all set",
		},
		{
			name:    "missing file",
			args:    []string{"db", "query", "svc-12345", "-f", filepath.Join(sqlDir, "nope.sql")},
			setup:   setupGetService,
			wantErr: "failed to read SQL file: open " + filepath.Join(sqlDir, "nope.sql") + ": no such file or directory",
		},
		{
			// No mock and no stdin: the service ID is required before anything
			// blocks on reading a query.
			name:    "missing service id",
			args:    []string{"db", "query"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "too many args",
			args:    []string{"db", "query", "svc-12345", "extra"},
			wantErr: "accepts at most 1 arg(s), received 2",
		},
		{
			name: "network error",
			args: []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "API error",
			args: []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name:    "service paused",
			args:    []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			setup:   setupGetService,
			opts:    []runOption{withExecuteQuery(nil, common.ErrPaused)},
			wantErr: pausedMsg("svc-12345"),
		},
		{
			name:    "query error",
			args:    []string{"db", "query", "svc-12345", "-c", "SELCT 1"},
			setup:   setupGetService,
			opts:    []runOption{withExecuteQuery(nil, errors.New(`ERROR: syntax error at or near "SELCT" (SQLSTATE 42601)`))},
			wantErr: `ERROR: syntax error at or near "SELCT" (SQLSTATE 42601)`,
		},
		{
			name:  "select results as a table",
			args:  []string{"db", "query", "svc-12345", "-c", "SELECT id, name FROM users"},
			setup: setupGetService,
			opts:  []runOption{withExecuteQuery(selectResult(), nil)},
			wantStdout: " id │ name  \n" +
				"────┼───────\n" +
				" 1  │ alice \n" +
				" 2  │ NULL  \n" +
				"(2 rows)\n\n",
		},
		{
			name:  "single row is reported in the singular",
			args:  []string{"db", "query", "svc-12345", "-c", "SELECT id, name FROM users LIMIT 1"},
			setup: setupGetService,
			opts: []runOption{withExecuteQuery(&common.QueryResult{
				ResultSets: []common.ResultSet{{
					CommandTag:   "SELECT 1",
					Columns:      []common.Column{{Name: "id", Type: "int4"}},
					Rows:         queryRows([]string{"1"}),
					RowsAffected: 1,
				}},
			}, nil)},
			wantStdout: " id \n" +
				"────\n" +
				" 1  \n" +
				"(1 row)\n\n",
		},
		{
			name:  "empty select still renders its columns",
			args:  []string{"db", "query", "svc-12345", "-c", "SELECT id FROM users WHERE false"},
			setup: setupGetService,
			opts: []runOption{withExecuteQuery(&common.QueryResult{
				ResultSets: []common.ResultSet{{
					CommandTag: "SELECT 0",
					Columns:    []common.Column{{Name: "id", Type: "int4"}},
					Rows:       queryRows(),
				}},
			}, nil)},
			// tablewriter draws no header rule for a table with no rows.
			wantStdout: " id \n(0 rows)\n\n",
		},
		{
			// A statement that returns no columns has nothing to tabulate, so
			// its command tag stands in for the result.
			name:  "multi-statement mixes command tags and tables",
			args:  []string{"db", "query", "svc-12345", "-c", "CREATE TABLE t (id int); INSERT INTO t VALUES (1); SELECT id FROM t"},
			setup: setupGetService,
			opts: []runOption{withExecuteQuery(&common.QueryResult{
				ResultSets: []common.ResultSet{
					{CommandTag: "CREATE TABLE"},
					{CommandTag: "INSERT 0 1", RowsAffected: 1},
					{
						CommandTag:   "SELECT 1",
						Columns:      []common.Column{{Name: "id", Type: "int4"}},
						Rows:         queryRows([]string{"1"}),
						RowsAffected: 1,
					},
				},
			}, nil)},
			wantStdout: "CREATE TABLE\n" +
				"INSERT 0 1\n" +
				" id \n" +
				"────\n" +
				" 1  \n" +
				"(1 row)\n\n",
		},
		{
			name:  "json output",
			args:  []string{"db", "query", "svc-12345", "-c", "SELECT id, name FROM users", "-o", "json"},
			setup: setupGetService,
			opts:  []runOption{withExecuteQuery(selectResult(), nil)},
			wantStdout: `{
  "result_sets": [
    {
      "command_tag": "SELECT 2",
      "columns": [
        {
          "name": "id",
          "type": "int4"
        },
        {
          "name": "name",
          "type": "text"
        }
      ],
      "rows": [
        [
          "1",
          "alice"
        ],
        [
          "2",
          null
        ]
      ],
      "rows_affected": 2
    }
  ],
  "execution_time": "12ms"
}
`,
		},
		{
			name:  "yaml output",
			args:  []string{"db", "query", "svc-12345", "-c", "SELECT id, name FROM users", "-o", "yaml"},
			setup: setupGetService,
			opts:  []runOption{withExecuteQuery(selectResult(), nil)},
			// Keys come out alphabetically: SerializeToYAML round-trips
			// through JSON, so the struct's field order is lost.
			wantStdout: `execution_time: 12ms
result_sets:
  - columns:
      - name: id
        type: int4
      - name: name
        type: text
    command_tag: SELECT 2
    rows:
      - - "1"
        - alice
      - - "2"
        - null
    rows_affected: 2
`,
		},
	})
}

// TestDbQueryCmdArgs covers what the command passes down to common.ExecuteQuery:
// the query it resolved from its various sources, and the connection options.
func TestDbQueryCmdArgs(t *testing.T) {
	var got common.ExecuteQueryArgs

	tests := []struct {
		name string
		args []string
		opts []runOption
		tag  string // environment tag on the service, for the read_only=prod cases
		want common.ExecuteQueryArgs
	}{
		{
			name: "defaults",
			args: []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			want: common.ExecuteQueryArgs{Query: "SELECT 1", Role: "tsdbadmin"},
		},
		{
			name: "query read from stdin",
			args: []string{"db", "query", "svc-12345"},
			opts: []runOption{withStdin("SELECT 1;\nSELECT 2;\n")},
			want: common.ExecuteQueryArgs{Query: "SELECT 1;\nSELECT 2;", Role: "tsdbadmin"},
		},
		{
			name: "role, pooled and read-only flags",
			args: []string{"db", "query", "svc-12345", "-c", "SELECT 1", "--role", "readonly", "--pooled", "--read-only"},
			want: common.ExecuteQueryArgs{Query: "SELECT 1", Role: "readonly", Pooled: true, ReadOnly: true},
		},
		{
			// Whitespace-only SQL is passed through for the database to report
			// on, rather than being second-guessed here.
			name: "whitespace-only query reaches the database",
			args: []string{"db", "query", "svc-12345", "-c", "   "},
			want: common.ExecuteQueryArgs{Query: "   ", Role: "tsdbadmin"},
		},
		{
			// read_only=all forces a read-only session without the flag.
			name: "read_only all",
			args: []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			opts: []runOption{withConfig(map[string]any{"read_only": "all"})},
			want: common.ExecuteQueryArgs{Query: "SELECT 1", Role: "tsdbadmin", ReadOnly: true},
		},
		{
			// Legacy boolean value, still accepted as an alias for "all".
			name: "read_only true",
			args: []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			opts: []runOption{withConfig(map[string]any{"read_only": true})},
			want: common.ExecuteQueryArgs{Query: "SELECT 1", Role: "tsdbadmin", ReadOnly: true},
		},
		{
			name: "read_only prod, PROD service",
			args: []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			opts: []runOption{withConfig(map[string]any{"read_only": "prod"})},
			tag:  "PROD",
			want: common.ExecuteQueryArgs{Query: "SELECT 1", Role: "tsdbadmin", ReadOnly: true},
		},
		{
			// prod mode leaves DEV services writable.
			name: "read_only prod, DEV service",
			args: []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			opts: []runOption{withConfig(map[string]any{"read_only": "prod"})},
			tag:  "DEV",
			want: common.ExecuteQueryArgs{Query: "SELECT 1", Role: "tsdbadmin"},
		},
		{
			// Row caps protect an agent's context, so they're MCP-only: the CLI
			// asks for everything the query produced.
			name: "no row or byte caps",
			args: []string{"db", "query", "svc-12345", "-c", "SELECT 1"},
			opts: []runOption{withConfig(map[string]any{"mcp_max_rows": 10})},
			want: common.ExecuteQueryArgs{Query: "SELECT 1", Role: "tsdbadmin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got = common.ExecuteQueryArgs{}
			opts := append(tt.opts, withExecuteQuery(&common.QueryResult{}, nil, &got))

			setup := func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService())
			}
			if tt.tag != "" {
				setup = expectTaggedService(tt.tag)
			}

			result := runCommand(t, tt.args, setup, opts...)
			if result.err != nil {
				t.Fatalf("unexpected error: %v", result.err)
			}

			if got.Query != tt.want.Query {
				t.Errorf("query = %q, want %q", got.Query, tt.want.Query)
			}
			if got.Role != tt.want.Role {
				t.Errorf("role = %q, want %q", got.Role, tt.want.Role)
			}
			if got.Pooled != tt.want.Pooled {
				t.Errorf("pooled = %t, want %t", got.Pooled, tt.want.Pooled)
			}
			if got.ReadOnly != tt.want.ReadOnly {
				t.Errorf("read-only = %t, want %t", got.ReadOnly, tt.want.ReadOnly)
			}
			if got.MaxRows != tt.want.MaxRows {
				t.Errorf("max rows = %d, want %d", got.MaxRows, tt.want.MaxRows)
			}
			if got.MaxBytes != tt.want.MaxBytes {
				t.Errorf("max bytes = %d, want %d", got.MaxBytes, tt.want.MaxBytes)
			}
		})
	}
}

// TestDbQueryCmdFileInput checks the --file path end to end, including the
// alias the command is also reachable by.
func TestDbQueryCmdFileInput(t *testing.T) {
	sqlFile := filepath.Join(t.TempDir(), "query.sql")
	if err := os.WriteFile(sqlFile, []byte("SELECT * FROM users;\n"), 0o600); err != nil {
		t.Fatalf("failed to write SQL file: %v", err)
	}

	var got common.ExecuteQueryArgs
	result := runCommand(t, []string{"db", "sql", "svc-12345", "-f", sqlFile},
		func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "svc-12345", sampleService())
		},
		withExecuteQuery(&common.QueryResult{}, nil, &got))
	if result.err != nil {
		t.Fatalf("unexpected error: %v", result.err)
	}

	if want := "SELECT * FROM users;\n"; got.Query != want {
		t.Errorf("query = %q, want %q", got.Query, want)
	}
}
