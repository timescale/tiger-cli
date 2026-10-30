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
// target and arguments the handler passed down and returning schema and err in
// their place. Expectation and return value are configured together, the way an API
// client mock's are, so nothing about a case leaks into the next one.
func withFetchServiceSchema(wantTarget *common.ConnectionTarget, want common.FetchServiceSchemaArgs, schema *common.DatabaseSchema, err error) runOption {
	return withSetup(func(t *testing.T) {
		original := common.FetchServiceSchema
		common.FetchServiceSchema = func(_ context.Context, _ *config.Config, gotTarget *common.ConnectionTarget, got common.FetchServiceSchemaArgs) (*common.DatabaseSchema, error) {
			t.Helper()
			if diff := cmp.Diff(wantTarget, gotTarget); diff != "" {
				t.Errorf("FetchServiceSchema target mismatch (-want +got):\n%s", diff)
			}
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
			expectResolve(m, "e6ue9697jf", nil, sampleService(func(s *api.Service) { s.Status = status }))
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

	primaryTarget := common.NewServiceConnectionTarget(sampleService())
	replica := sampleReplica(func(r *api.ReadReplicaSet) { r.Status = api.ReadReplicaSetStatusActive })
	// A replica connects to its own endpoint with its primary's credentials.
	replicaTarget := common.NewReplicaConnectionTarget(replica, common.ServiceCredentials(sampleService()))
	branch := sampleBranch()

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
				m.EXPECT().ResolveRefWithResponse(validCtx, testProjectID, &api.ResolveRefParams{Ref: "e6ue9697jf"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to resolve service or read replica set 'e6ue9697jf': connection refused",
		},
		{
			name: "service lookup API error",
			tool: toolDBSchema,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveStatus(m, "e6ue9697jf", nil, http.StatusNotFound, &api.ClientError{Message: new("resource not found")})
			},
			wantErr: "service or read replica set 'e6ue9697jf' not found",
		},
		{
			name: "nil response body",
			tool: toolDBSchema,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveStatus(m, "e6ue9697jf", nil, http.StatusOK, nil)
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
			// The parent is fetched for its credentials, then the replica's own
			// status stops it: a replica set that isn't active isn't ready.
			name: "read replica resolves its parent",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "u8me885b93"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolve(m, "u8me885b93", nil, sampleReplica())
				expectGetService(m, "e6ue9697jf", sampleService())
			},
			wantErr: notReadyMsg,
		},
		{
			name: "read replica parent lookup fails",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "u8me885b93"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolve(m, "u8me885b93", nil, sampleReplica())
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: `failed to fetch parent service "e6ue9697jf" for read replica: service not found`,
		},
		{
			// The resolve endpoint also matches names, but the tools take IDs
			// only, so a service whose name looks like an ID isn't found by it.
			name: "a name match is not found",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "mydatabase"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolve(m, "mydatabase", nil, sampleService())
			},
			wantErr: "service or read replica set 'mydatabase' not found",
		},
		{
			// Branches are a preview, so without the gate a branch ID is
			// reported as not found, without mentioning branches.
			name: "branch not found without the experimental gate",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "br4nch1d9x"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolve(m, "br4nch1d9x", nil, branch)
			},
			wantErr: "service or read replica set 'br4nch1d9x' not found",
		},
		{
			// A branch is started with branch_start, not service_start.
			name: "paused branch points at branch_start",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "br4nch1d9x"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolve(m, "br4nch1d9x", nil, sampleBranch(branchStatus(api.DeployStatusPAUSED)))
			},
			wantErr: "branch is paused — start it with the branch_start tool",
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
				expectResolve(m, "e6ue9697jf", nil, sampleService(func(s *api.Service) {
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
			opts:    []runOption{withFetchServiceSchema(primaryTarget, common.FetchServiceSchemaArgs{Role: "tsdbadmin"}, nil, errors.New("failed to connect to database: no route to host"))},
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			wantErr: "failed to connect to database: no route to host",
		},
		{
			name:       "returns the schema with the parameter defaults",
			tool:       toolDBSchema,
			args:       args,
			opts:       []runOption{withFetchServiceSchema(primaryTarget, common.FetchServiceSchemaArgs{Role: "tsdbadmin"}, schema, nil)},
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
			opts: []runOption{withFetchServiceSchema(primaryTarget, common.FetchServiceSchemaArgs{
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
			opts: []runOption{withFetchServiceSchema(replicaTarget, common.FetchServiceSchemaArgs{Role: "tsdbadmin", Pooled: true}, schema, nil)},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolve(m, "u8me885b93", nil, replica)
				expectGetService(m, "e6ue9697jf", sampleService())
			},
			wantOutput: map[string]any{
				"schema":  schemaText,
				"warning": `read replica "replica-service" has no connection pooler; connecting directly instead`,
			},
		},
		{
			name: "returns the schema of a branch",
			tool: toolDBSchema,
			args: map[string]any{"service_id": "br4nch1d9x"},
			opts: []runOption{withExperimental(), withFetchServiceSchema(common.NewBranchConnectionTarget(branch), common.FetchServiceSchemaArgs{Role: "tsdbadmin"}, schema, nil)},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolve(m, "br4nch1d9x", nil, branch)
			},
			wantOutput: map[string]any{"schema": schemaText},
		},
	})
}
