package mcp

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestDBQuery(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "query": "SELECT 1"}

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

	// A read replica connects to its own endpoint but borrows the parent
	// primary's credentials, so resolving one fetches both services.
	replica := sampleService(func(s *api.Service) {
		s.ServiceID = "u8me885b93"
		s.Name = "replica-service"
		s.Status = api.DeployStatusPAUSED
		s.ForkedFrom = &api.ForkSpec{
			IsStandby: new(true),
			ProjectID: new(testProjectID),
			ServiceID: new("e6ue9697jf"),
		}
	})

	expectGet := func(id string, svc api.Service) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, id).
				Return(&api.GetServiceResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &svc,
				}, nil)
		}
	}
	expectStatus := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return expectGet("e6ue9697jf", sampleService(func(s *api.Service) { s.Status = status }))
	}

	const (
		bothOrNeitherErr = "exactly one of 'query' or 'file' must be provided"
		pausedErr        = "service is paused — start it with the service_start tool"
		notReadyErr      = "service is not ready — check its status with service_get and try again"
		// sampleService carries no endpoint, so a READY service fails here
		// rather than opening a connection.
		noEndpointErr = "failed to build connection string: service endpoint not available"
	)

	runToolTests(t, []toolTest{
		{
			name:      "not logged in",
			tool:      toolDBQuery,
			args:      args,
			clientErr: errNotLoggedIn,
			wantErr:   errNotLoggedIn.Error(),
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
			wantErr: bothOrNeitherErr,
		},
		{
			name:    "both query and file",
			tool:    toolDBQuery,
			args:    map[string]any{"service_id": "e6ue9697jf", "query": "SELECT 1", "file": sqlFile},
			wantErr: bothOrNeitherErr,
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
			name:      "SQL file read then stops before connecting",
			tool:      toolDBQuery,
			args:      map[string]any{"service_id": "e6ue9697jf", "file": sqlFile},
			setupMock: expectStatus(api.DeployStatusREADY),
			wantErr:   noEndpointErr,
		},
		{
			name: "service lookup network error",
			tool: toolDBQuery,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "service lookup API error",
			tool: toolDBQuery,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
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
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
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
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGet("u8me885b93", replica)(m)
				expectGet("e6ue9697jf", sampleService())(m)
			},
			wantErr: pausedErr,
		},
		{
			name: "read replica parent lookup fails",
			tool: toolDBQuery,
			args: map[string]any{"service_id": "u8me885b93", "query": "SELECT 1"},
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGet("u8me885b93", replica)(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: `failed to fetch parent service "e6ue9697jf" for read replica: service not found`,
		},
		{
			name:      "service paused",
			tool:      toolDBQuery,
			args:      args,
			setupMock: expectStatus(api.DeployStatusPAUSED),
			wantErr:   pausedErr,
		},
		{
			name:      "service pausing",
			tool:      toolDBQuery,
			args:      args,
			setupMock: expectStatus(api.DeployStatusPAUSING),
			wantErr:   pausedErr,
		},
		{
			name:      "service not ready",
			tool:      toolDBQuery,
			args:      args,
			setupMock: expectStatus(api.DeployStatusQUEUED),
			wantErr:   notReadyErr,
		},
		{
			name: "pooled without a pooler",
			tool: toolDBQuery,
			args: map[string]any{"service_id": "e6ue9697jf", "query": "SELECT 1", "pooled": true},
			setupMock: expectGet("e6ue9697jf", sampleService(func(s *api.Service) {
				s.Endpoint = &api.Endpoint{
					Host: new("e6ue9697jf.project.tsdb.cloud.timescale.com"),
					Port: new(5432),
				}
			})),
			wantErr: "connection pooler not available for this service",
		},
		{
			// db_query isn't a read-only gated tool: the mode makes the session
			// read-only rather than refusing the call, so the query proceeds.
			name:      "read-only all still runs the query",
			tool:      toolDBQuery,
			args:      args,
			config:    map[string]any{"read_only": "all"},
			setupMock: expectStatus(api.DeployStatusREADY),
			wantErr:   noEndpointErr,
		},
		{
			// The remaining parameters only shape the connection and the
			// statement issued after it; all that's observable here is that
			// they're accepted.
			name: "remaining parameters accepted",
			tool: toolDBQuery,
			args: map[string]any{
				"service_id":      "e6ue9697jf",
				"query":           "SELECT $1::int",
				"parameters":      []any{"1"},
				"timeout_seconds": 60,
				"role":            "readonly",
				"pooled":          false,
			},
			setupMock: expectStatus(api.DeployStatusPAUSED),
			wantErr:   pausedErr,
		},
	})
}

// Helper-level: the row cap only takes effect once a query runs, so a tool
// call can't reach it without a live database.
func TestResolveMaxRows(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		want       int
	}{
		{
			name:       "configured value is used",
			configured: 250,
			want:       250,
		},
		{
			// A config-file or TIGER_MCP_MAX_ROWS value bypasses `tiger config
			// set` validation, so a zero (or negative) configured value can
			// reach here and must be sanitized to the default.
			name:       "zero configured (env/file bypass) falls back to default",
			configured: 0,
			want:       config.DefaultMCPMaxRows,
		},
		{
			name:       "negative configured falls back to default",
			configured: -1,
			want:       config.DefaultMCPMaxRows,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveMaxRows(tt.configured); got != tt.want {
				t.Errorf("resolveMaxRows(%d) = %d, want %d", tt.configured, got, tt.want)
			}
		})
	}
}

// Helper-level: the notice is only emitted on a truncated result set, which
// needs a live database to produce.
func TestTruncationNotice(t *testing.T) {
	notice := truncationNotice(100)
	// The notice must mention the actual cap and steer the model toward doing
	// the work in SQL rather than re-running the query.
	for _, want := range []string{"100", "LIMIT", "aggregate"} {
		if !strings.Contains(notice, want) {
			t.Errorf("truncationNotice() = %q, missing %q", notice, want)
		}
	}
}

func TestDBQueryOutputSchemaHasTruncationFields(t *testing.T) {
	schema := DBQueryOutput{}.Schema()
	for _, name := range []string{"truncated", "notice"} {
		prop, ok := schema.Properties[name]
		if !ok {
			t.Fatalf("expected %q property in output schema", name)
		}
		if prop.Description == "" {
			t.Errorf("expected %q to have a description", name)
		}
	}
	resultSet := schema.Properties["result_sets"].Items
	if _, ok := resultSet.Properties["truncated"]; !ok {
		t.Error("expected truncated property on result set schema")
	}
}

func TestResolveQueryInput(t *testing.T) {
	dir := t.TempDir()
	sqlPath := filepath.Join(dir, "schema.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatalf("failed to write SQL file: %v", err)
	}
	emptyPath := filepath.Join(dir, "empty.sql")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatalf("failed to write empty SQL file: %v", err)
	}

	tests := []struct {
		name    string
		query   string
		file    string
		want    string
		wantErr string
	}{
		{
			name:  "inline query",
			query: "SELECT 1",
			want:  "SELECT 1",
		},
		{
			name: "query read from file",
			file: sqlPath,
			want: "SELECT 1;\n",
		},
		{
			name:    "neither provided",
			wantErr: "exactly one of 'query' or 'file' must be provided",
		},
		{
			name:    "both provided",
			query:   "SELECT 1",
			file:    sqlPath,
			wantErr: "exactly one of 'query' or 'file' must be provided",
		},
		{
			name:    "missing file",
			file:    filepath.Join(dir, "nope.sql"),
			wantErr: "failed to read SQL file",
		},
		{
			name:    "empty file",
			file:    emptyPath,
			wantErr: "is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveQueryInput(tt.query, tt.file)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("query = %q, want %q", got, tt.want)
			}
		})
	}
}
