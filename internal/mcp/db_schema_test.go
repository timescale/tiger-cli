package mcp

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
// where the tool would open a real database connection — asserting the
// arguments the handler passed down and returning schema and err in their
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

func TestDBSchemaTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	setupGetWithStatus := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) { s.Status = status }))
		}
	}

	// The fetch is stubbed for the success cases below, so they reach the
	// handler's own output without a live database.
	schema := &common.DatabaseSchema{
		ID:   "e6ue9697jf",
		Name: "tsdb",
		Schemas: []common.NamespacedSchema{{
			Name: "public",
			Tables: []common.TableSchema{{
				Name:    "metrics",
				Columns: []common.TableColumnSchema{{Name: "time", Type: "timestamptz", NotNull: true}},
			}},
		}},
	}
	const schemaText = "DATABASE: tsdb (e6ue9697jf)\n\nSCHEMA: public\n\nTABLE: metrics\n  time  TIMESTAMPTZ NOT NULL\n"

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolDBSchema,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The service_id pattern is enforced by the SDK's schema validation,
			// before the handler runs, so no API call is registered.
			name:    "service id fails schema validation",
			tool:    toolDBSchema,
			args:    map[string]any{"service_id": "not-an-id"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "not-an-id" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name: "service lookup network error",
			tool: toolDBSchema,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "service lookup API error",
			tool: toolDBSchema,
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
			tool: toolDBSchema,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:    "service paused",
			tool:    toolDBSchema,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusPAUSED),
			wantErr: pausedMsg,
		},
		{
			name:    "service pausing",
			tool:    toolDBSchema,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusPAUSING),
			wantErr: pausedMsg,
		},
		{
			name:    "service not ready",
			tool:    toolDBSchema,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusQUEUED),
			wantErr: notReadyMsg,
		},
		{
			// Both services are fetched: the replica for its endpoint, the
			// parent for its credentials. The replica's own status then stops it.
			name: "read replica resolves its parent",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "u8me885b93"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "u8me885b93", sampleReplica())
				expectGetService(m, "e6ue9697jf", sampleService())
			},
			wantErr: pausedMsg,
		},
		{
			name: "read replica parent lookup fails",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "u8me885b93"},
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
			name:    "ready service without an endpoint",
			tool:    toolDBSchema,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			wantErr: noEndpointMsg,
		},
		{
			name: "pooled without a pooler",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "e6ue9697jf", "pooled": true},
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
			// A fetch failure passes through handleDatabaseError unchanged.
			name:    "fetch fails",
			tool:    toolDBSchema,
			args:    args,
			opts:    []runOption{withFetchServiceSchema(common.FetchServiceSchemaArgs{Role: "tsdbadmin"}, nil, errors.New("failed to connect to database: no route to host"))},
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			wantErr: "failed to connect to database: no route to host",
		},
		{
			name:       "returns the schema with the parameter defaults",
			tool:       toolDBSchema,
			args:       args,
			opts:       []runOption{withFetchServiceSchema(common.FetchServiceSchemaArgs{Role: "tsdbadmin"}, schema, nil)},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: map[string]any{"schema": schemaText},
		},
		{
			name: "passes every introspection option down to the fetch",
			tool: toolDBSchema,
			args: map[string]any{
				"service_id":  "e6ue9697jf",
				"schema":      "public",
				"internal":    true,
				"definitions": true,
				"comments":    true,
				"role":        "readonly",
			},
			opts: []runOption{withFetchServiceSchema(common.FetchServiceSchemaArgs{
				Role:               "readonly",
				Schema:             "public",
				IncludeInternal:    true,
				IncludeDefinitions: true,
				IncludeComments:    true,
			}, schema, nil)},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: map[string]any{"schema": schemaText},
		},
		{
			// The replica has no pooler, so the warning rides along with a
			// successful result — the one path it is ever visible on — and
			// naming the replica proves it is the connection target.
			name: "read replica without a pooler warns alongside the schema",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "u8me885b93", "pooled": true},
			opts: []runOption{withFetchServiceSchema(common.FetchServiceSchemaArgs{Role: "tsdbadmin", Pooled: true}, schema, nil)},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "u8me885b93", sampleReplica(func(s *api.Service) { s.Status = api.DeployStatusREADY }))
				expectGetService(m, "e6ue9697jf", sampleService())
			},
			wantOutput: map[string]any{
				"schema":  schemaText,
				"warning": `read replica "replica-service" has no connection pooler; connecting directly instead`,
			},
		},
	})
}
