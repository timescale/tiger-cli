package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// withFetchExtensions replaces common.FetchExtensions so tests run without a
// real database. It checks what it is called with and returns the given result.
func withFetchExtensions(wantTarget *common.ConnectionTarget, wantRole string, extensions []common.Extension, err error) runOption {
	return withSetup(func(t *testing.T) {
		original := common.FetchExtensions
		common.FetchExtensions = func(_ context.Context, _ *config.Config, gotTarget *common.ConnectionTarget, gotRole string) ([]common.Extension, error) {
			t.Helper()
			if diff := cmp.Diff(wantTarget, gotTarget); diff != "" {
				t.Errorf("FetchExtensions target mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(wantRole, gotRole); diff != "" {
				t.Errorf("FetchExtensions role mismatch (-want +got):\n%s", diff)
			}
			return extensions, err
		}
		t.Cleanup(func() { common.FetchExtensions = original })
	})
}

func TestDbExtensionsListCmd(t *testing.T) {
	setupGetWithStatus := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, "svc-12345", sampleService(func(s *api.Service) {
				s.Status = status
			}))
		}
	}

	extensions := []common.Extension{
		{Name: "plpgsql", Version: "1.0", Schema: "pg_catalog", Description: "PL/pgSQL procedural language"},
		{Name: "timescaledb", Version: "2.21.0", Schema: "public", Description: "Enables scalable inserts and complex queries for time-series data"},
		{Name: "vector", Version: "0.8.0", Schema: "public"},
	}
	primary := sampleService()
	primaryTarget := &common.ConnectionTarget{ConnectionService: primary, CredentialService: primary}
	replica := sampleReplica()
	replicaTarget := &common.ConnectionTarget{ConnectionService: replica, CredentialService: primary, IsReplica: true}
	setupGetReplica := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveRef(m, "rep-67890", replica)
		expectGetService(m, "svc-12345", primary)
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "extensions", "list", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "missing service id",
			args:    []string{"db", "extensions", "list"},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "invalid output format",
			args:    []string{"db", "extensions", "list", "svc-12345", "-o", "csv"},
			wantErr: `invalid argument "csv" for "-o, --output" flag: invalid output format: csv (must be one of: json, yaml, table)`,
		},
		{
			// Stopping at the paused check shows the default service was used.
			name:    "default service id from config",
			args:    []string{"db", "extensions", "list"},
			opts:    []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			mock:    setupGetWithStatus(api.DeployStatusPAUSED),
			wantErr: pausedMsg("svc-12345"),
		},
		{
			name: "network error",
			args: []string{"db", "extensions", "list", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefError(m, "svc-12345", errors.New("connection refused"))
			},
			wantErr: `failed to resolve service 'svc-12345': connection refused`,
		},
		{
			name: "service not found",
			args: []string{"db", "extensions", "list", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefNotFound(m, "svc-12345")
			},
			wantErr: "service 'svc-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"db", "extensions", "list", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "svc-12345", http.StatusOK, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "read replica parent lookup fails",
			args: []string{"db", "extensions", "list", "rep-67890"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "rep-67890", replica)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: `failed to fetch parent service "svc-12345" for read replica: failed to fetch service details: connection refused`,
		},
		{
			name:    "service paused",
			args:    []string{"db", "extensions", "list", "svc-12345"},
			mock:    setupGetWithStatus(api.DeployStatusPAUSED),
			wantErr: pausedMsg("svc-12345"),
		},
		{
			name:    "service not ready",
			args:    []string{"db", "extensions", "list", "svc-12345"},
			mock:    setupGetWithStatus(api.DeployStatusQUEUED),
			wantErr: notReadyMsg("svc-12345"),
		},
		{
			name:    "fetch fails",
			args:    []string{"db", "extensions", "list", "svc-12345"},
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			opts:    []runOption{withFetchExtensions(primaryTarget, "tsdbadmin", nil, errors.New("failed to connect to database: no route to host"))},
			wantErr: "failed to connect to database: no route to host",
		},
		{
			// A replica uses its primary's password, so the hint names the primary.
			name: "authentication failure on a read replica names the primary",
			args: []string{"db", "extensions", "list", "rep-67890"},
			mock: setupGetReplica,
			opts: []runOption{withFetchExtensions(replicaTarget, "tsdbadmin", nil, fmt.Errorf("failed to connect to database: %w", &pgconn.PgError{
				Severity: "FATAL",
				Code:     "28P01",
				Message:  `password authentication failed for user "tsdbadmin"`,
			}))},
			wantErr: "failed to connect to database: FATAL: password authentication failed for user \"tsdbadmin\" (SQLSTATE 28P01)\n\n" +
				"The stored password is missing or invalid. Save the current one with 'tiger db save-password svc-12345', or reset it with 'tiger service update-password svc-12345'",
		},
		{
			name: "prints a table",
			args: []string{"db", "extensions", "list", "svc-12345"},
			mock: setupGetWithStatus(api.DeployStatusREADY),
			opts: []runOption{withFetchExtensions(primaryTarget, "tsdbadmin", extensions, nil)},
			wantStdout: `┌─────────────┬─────────┬────────────┬───────────────────────────────────────────────────────────────────┐
│    NAME     │ VERSION │   SCHEMA   │                            DESCRIPTION                            │
├─────────────┼─────────┼────────────┼───────────────────────────────────────────────────────────────────┤
│ plpgsql     │ 1.0     │ pg_catalog │ PL/pgSQL procedural language                                      │
│ timescaledb │ 2.21.0  │ public     │ Enables scalable inserts and complex queries for time-series data │
│ vector      │ 0.8.0   │ public     │                                                                   │
└─────────────┴─────────┴────────────┴───────────────────────────────────────────────────────────────────┘
`,
		},
		{
			name: "prints json",
			args: []string{"db", "extensions", "list", "svc-12345", "-o", "json"},
			mock: setupGetWithStatus(api.DeployStatusREADY),
			opts: []runOption{withFetchExtensions(primaryTarget, "tsdbadmin", extensions, nil)},
			wantStdout: `[
  {
    "name": "plpgsql",
    "version": "1.0",
    "schema": "pg_catalog",
    "description": "PL/pgSQL procedural language"
  },
  {
    "name": "timescaledb",
    "version": "2.21.0",
    "schema": "public",
    "description": "Enables scalable inserts and complex queries for time-series data"
  },
  {
    "name": "vector",
    "version": "0.8.0",
    "schema": "public"
  }
]
`,
		},
		{
			name: "prints yaml",
			args: []string{"db", "extensions", "list", "svc-12345", "-o", "yaml"},
			mock: setupGetWithStatus(api.DeployStatusREADY),
			opts: []runOption{withFetchExtensions(primaryTarget, "tsdbadmin", extensions, nil)},
			wantStdout: `- description: PL/pgSQL procedural language
  name: plpgsql
  schema: pg_catalog
  version: "1.0"
- description: Enables scalable inserts and complex queries for time-series data
  name: timescaledb
  schema: public
  version: 2.21.0
- name: vector
  schema: public
  version: 0.8.0
`,
		},
		{
			name: "prints an empty table",
			args: []string{"db", "extensions", "list", "svc-12345"},
			mock: setupGetWithStatus(api.DeployStatusREADY),
			opts: []runOption{withFetchExtensions(primaryTarget, "tsdbadmin", []common.Extension{}, nil)},
			wantStdout: `┌──────┬─────────┬────────┬─────────────┐
│ NAME │ VERSION │ SCHEMA │ DESCRIPTION │
└──────┴─────────┴────────┴─────────────┘
`,
		},
		{
			name:       "lists the extensions of a read replica",
			args:       []string{"db", "extensions", "list", "rep-67890", "-o", "json"},
			mock:       setupGetReplica,
			opts:       []runOption{withFetchExtensions(replicaTarget, "tsdbadmin", []common.Extension{}, nil)},
			wantStdout: "[]\n",
		},
		{
			name:       "passes the role down to the fetch",
			args:       []string{"db", "extensions", "list", "svc-12345", "--role", "reader", "-o", "json"},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			opts:       []runOption{withFetchExtensions(primaryTarget, "reader", []common.Extension{}, nil)},
			wantStdout: "[]\n",
		},
	})
}
