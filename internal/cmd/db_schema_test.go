package cmd

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// withFetchServiceSchema stubs common.FetchServiceSchema — otherwise the point
// where the command would open a real database connection — asserting the
// arguments the command passed down and returning schema and err in their
// place. Expectation and return value are configured together, the way an API
// client mock's are, so nothing about a case leaks into the next one.
func withFetchServiceSchema(want common.FetchServiceSchemaArgs, schema *common.DatabaseSchema, err error) runOption {
	return withSetup(func(t *testing.T) {
		original := common.FetchServiceSchema
		common.FetchServiceSchema = func(_ context.Context, _ *config.Config, _ *common.ConnectionTarget, got common.FetchServiceSchemaArgs) (*common.DatabaseSchema, error) {
			t.Helper()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("FetchServiceSchema args mismatch (-want +got):\n%s", diff)
			}
			return schema, err
		}
		t.Cleanup(func() { common.FetchServiceSchema = original })
	})
}

func TestDbSchemaCmd(t *testing.T) {
	setupGetWithStatus := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "svc-12345", sampleService(func(s *api.Service) {
				s.Status = status
			}))
		}
	}

	// The fetch is stubbed for the success cases below, so they reach the
	// command's own output and flag handling without a live database.
	schema := &common.DatabaseSchema{
		ID:   "svc-12345",
		Name: "tsdb",
		Schemas: []common.NamespacedSchema{{
			Name: "public",
			Tables: []common.TableSchema{{
				Name:    "metrics",
				Columns: []common.TableColumnSchema{{Name: "time", Type: "timestamptz", NotNull: true}},
			}},
		}},
	}
	const schemaText = "DATABASE: tsdb (svc-12345)\n\nSCHEMA: public\n\nTABLE: metrics\n  time  TIMESTAMPTZ NOT NULL\n"

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "schema", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "missing service id",
			args:    []string{"db", "schema"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			// Paused readiness stops the command before any connection attempt,
			// proving the config default reached the service lookup.
			name:    "default service id from config",
			args:    []string{"db", "schema"},
			opts:    []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			mock:    setupGetWithStatus(api.DeployStatusPAUSED),
			wantErr: pausedMsg("svc-12345"),
		},
		{
			name: "network error",
			args: []string{"db", "schema", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "API error",
			args: []string{"db", "schema", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
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
			name: "nil response body",
			args: []string{"db", "schema", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:    "service paused",
			args:    []string{"db", "schema", "svc-12345"},
			mock:    setupGetWithStatus(api.DeployStatusPAUSED),
			wantErr: pausedMsg("svc-12345"),
		},
		{
			name:    "service pausing",
			args:    []string{"db", "schema", "svc-12345"},
			mock:    setupGetWithStatus(api.DeployStatusPAUSING),
			wantErr: pausedMsg("svc-12345"),
		},
		{
			name:    "service not ready",
			args:    []string{"db", "schema", "svc-12345"},
			mock:    setupGetWithStatus(api.DeployStatusQUEUED),
			wantErr: notReadyMsg("svc-12345"),
		},
		{
			name:    "pooled without pooler",
			args:    []string{"db", "schema", "svc-12345", "--pooled"},
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			wantErr: "connection pooler not available for this service",
		},
		{
			// The replica has no pooler, so --pooled warns and falls back; the
			// not-ready status then stops the command before any connection.
			name: "replica pooled without pooler warns before readiness check",
			args: []string{"db", "schema", "rep-67890", "--pooled"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "rep-67890", sampleReplica(func(s *api.Service) {
					s.Status = api.DeployStatusQUEUED
				}))
				expectGetService(m, "svc-12345", sampleService())
			},
			wantErr:    notReadyMsg("rep-67890"),
			wantStderr: "Warning: read replica \"replica-service\" has no connection pooler; connecting directly instead\nError: " + notReadyMsg("rep-67890") + "\n",
		},
		{
			name:       "prints the schema with the flag defaults",
			args:       []string{"db", "schema", "svc-12345"},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			opts:       []runOption{withFetchServiceSchema(common.FetchServiceSchemaArgs{Role: "tsdbadmin"}, schema, nil)},
			wantStdout: schemaText,
		},
		{
			name: "passes every flag down to the fetch",
			args: []string{
				"db", "schema", "svc-12345",
				"--schema", "public", "--internal", "--definitions", "--comments", "--role", "reader",
			},
			mock: setupGetWithStatus(api.DeployStatusREADY),
			opts: []runOption{withFetchServiceSchema(common.FetchServiceSchemaArgs{
				Role:               "reader",
				Schema:             "public",
				IncludeInternal:    true,
				IncludeDefinitions: true,
				IncludeComments:    true,
			}, schema, nil)},
			wantStdout: schemaText,
		},
		{
			// The warning names the replica, proving it is the connection
			// target; --pooled falls back and this time the fetch succeeds.
			name: "prints the schema of a read replica",
			args: []string{"db", "schema", "rep-67890", "--pooled"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "rep-67890", sampleReplica())
				expectGetService(m, "svc-12345", sampleService())
			},
			opts:       []runOption{withFetchServiceSchema(common.FetchServiceSchemaArgs{Role: "tsdbadmin", Pooled: true}, schema, nil)},
			wantStdout: schemaText,
			wantStderr: "Warning: read replica \"replica-service\" has no connection pooler; connecting directly instead\n",
		},
		{
			// A fetch failure goes through handleDatabaseError like any other.
			name:    "fetch fails",
			args:    []string{"db", "schema", "svc-12345"},
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			opts:    []runOption{withFetchServiceSchema(common.FetchServiceSchemaArgs{Role: "tsdbadmin"}, nil, errors.New("failed to connect to database: no route to host"))},
			wantErr: "failed to connect to database: no route to host",
		},
	})
}
