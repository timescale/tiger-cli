package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestDbURICmd(t *testing.T) {
	const (
		directURI   = "postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n"
		readOnlyURI = "postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require&options=-c%20tsdb_admin.read_only_connection%3Dtrue\n"
		replicaURI  = "postgresql://tsdbadmin@rep-67890.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n"
		branchURI   = "postgresql://tsdbadmin@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require\n"
	)

	setupGet := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveTarget(m, "svc-12345", sampleService())
	}
	setupGetReplica := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveTarget(m, "rep-67890", sampleReplica())
		expectGetService(m, "svc-12345", sampleService())
	}
	withPooler := func(s *api.Service) {
		s.ConnectionPooler = &api.ConnectionPooler{
			Endpoint: &api.Endpoint{
				Host: new("pooler.svc-12345.project.tsdb.cloud.timescale.com"),
				Port: new(6432),
			},
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "uri", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing service id",
			args:    []string{"db", "uri"},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"db", "uri", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetError(m, "svc-12345", errors.New("connection refused"))
			},
			wantErr: `failed to resolve service or read replica set 'svc-12345': connection refused`,
		},
		{
			name: "service not found",
			args: []string{"db", "uri", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetNotFound(m, "svc-12345")
			},
			wantErr: "service or read replica set 'svc-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "ambiguous name refused",
			args: []string{"db", "uri", "my-api-db"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetStatus(m, "my-api-db", http.StatusBadRequest, &api.ClientError{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			// Branches are a preview, so the hint names `tiger branch list`
			// only behind the gate.
			name: "ambiguous name refused when experimental",
			args: []string{"db", "uri", "my-api-db"},
			opts: []runOption{withExperimental()},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetStatus(m, "my-api-db", http.StatusBadRequest, &api.ClientError{Message: new("ambiguous ref matches multiple resources")})
			},
			wantErr: "ambiguous ref matches multiple resources\nRun 'tiger service list' or 'tiger branch list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			// Branches are a preview, so without the gate a ref naming one is
			// reported as not found, without mentioning branches.
			name: "branch not found without experimental",
			args: []string{"db", "uri", "test-branch"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "test-branch", sampleBranch())
			},
			wantErr: "service or read replica set 'test-branch' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name:       "branch by name when experimental",
			args:       []string{"db", "uri", "test-branch"},
			opts:       []runOption{withExperimental()},
			mock:       func(m *mocks.MockClientWithResponsesInterface) { expectResolveTarget(m, "test-branch", sampleBranch()) },
			wantStdout: branchURI,
		},
		{
			name:    "branch has no pooler",
			args:    []string{"db", "uri", "br-12345", "--pooled"},
			opts:    []runOption{withExperimental()},
			mock:    func(m *mocks.MockClientWithResponsesInterface) { expectResolveTarget(m, "br-12345", sampleBranch()) },
			wantErr: "connection pooler not available for this service",
		},
		{
			name: "nil response body",
			args: []string{"db", "uri", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTargetStatus(m, "svc-12345", http.StatusOK, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "positional service id",
			args:       []string{"db", "uri", "svc-12345"},
			mock:       setupGet,
			wantStdout: directURI,
		},
		{
			name:       "default service id from config",
			args:       []string{"db", "uri"},
			mock:       setupGet,
			opts:       []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			wantStdout: directURI,
		},
		{
			name:       "connection-string alias",
			args:       []string{"db", "connection-string", "svc-12345"},
			mock:       setupGet,
			wantStdout: directURI,
		},
		{
			name:       "custom role",
			args:       []string{"db", "uri", "svc-12345", "--role", "readonly"},
			mock:       setupGet,
			wantStdout: "postgresql://readonly@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n",
		},
		{
			name: "pooled with pooler available",
			args: []string{"db", "uri", "svc-12345", "--pooled"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "svc-12345", sampleService(withPooler))
			},
			wantStdout: "postgresql://tsdbadmin@pooler.svc-12345.project.tsdb.cloud.timescale.com:6432/tsdb?sslmode=require\n",
		},
		{
			name:    "pooled without pooler",
			args:    []string{"db", "uri", "svc-12345", "--pooled"},
			mock:    setupGet,
			wantErr: "connection pooler not available for this service",
		},
		{
			name:    "with-password without stored password",
			args:    []string{"db", "uri", "svc-12345", "--with-password"},
			mock:    setupGet,
			wantErr: "password not available to include in connection string",
		},
		{
			name:       "read-only flag",
			args:       []string{"db", "uri", "svc-12345", "--read-only"},
			mock:       setupGet,
			wantStdout: readOnlyURI,
		},
		{
			name:       "read-only from config all",
			args:       []string{"db", "uri", "svc-12345"},
			mock:       setupGet,
			opts:       []runOption{withConfig(map[string]any{"read_only": "all"})},
			wantStdout: readOnlyURI,
		},
		{
			name:       "read-only flag and config all",
			args:       []string{"db", "uri", "svc-12345", "--read-only"},
			mock:       setupGet,
			opts:       []runOption{withConfig(map[string]any{"read_only": "all"})},
			wantStdout: readOnlyURI,
		},
		{
			name:       "config prod, PROD service",
			args:       []string{"db", "uri", "svc-12345"},
			mock:       expectTaggedTarget("PROD"),
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			wantStdout: readOnlyURI,
		},
		{
			name:       "config prod, DEV service",
			args:       []string{"db", "uri", "svc-12345"},
			mock:       expectTaggedTarget("DEV"),
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			wantStdout: directURI,
		},
		{
			name:       "config prod, untagged service",
			args:       []string{"db", "uri", "svc-12345"},
			mock:       setupGet,
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			wantStdout: directURI,
		},
		{
			name:       "flag on beats config prod",
			args:       []string{"db", "uri", "svc-12345", "--read-only"},
			mock:       expectTaggedTarget("DEV"),
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			wantStdout: readOnlyURI,
		},
		{
			name:       "replica target uses replica endpoint",
			args:       []string{"db", "uri", "rep-67890"},
			mock:       setupGetReplica,
			wantStdout: replicaURI,
		},
		{
			name: "replica pooled with pooler uses replica pooler",
			args: []string{"db", "uri", "rep-67890", "--pooled"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "rep-67890", sampleReplica(func(r *api.ReadReplicaSet) {
					r.ConnectionPooler = &api.ConnectionPooler{
						Endpoint: &api.Endpoint{
							Host: new("pooler.rep-67890.project.tsdb.cloud.timescale.com"),
							Port: new(6432),
						},
					}
				}))
				expectGetService(m, "svc-12345", sampleService())
			},
			wantStdout: "postgresql://tsdbadmin@pooler.rep-67890.project.tsdb.cloud.timescale.com:6432/tsdb?sslmode=require\n",
		},
		{
			name:       "replica pooled without pooler falls back with warning",
			args:       []string{"db", "uri", "rep-67890", "--pooled"},
			mock:       setupGetReplica,
			wantStdout: replicaURI,
			wantStderr: "Warning: read replica \"replica-service\" has no connection pooler; connecting directly instead\n",
		},
		{
			name: "replica parent fetch error",
			args: []string{"db", "uri", "rep-67890"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveTarget(m, "rep-67890", sampleReplica())
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch parent service \"svc-12345\" for read replica: failed to fetch service details: connection refused",
		},
		{
			name:       "with-password includes stored password",
			args:       []string{"db", "uri", "svc-12345", "--with-password"},
			mock:       setupGet,
			opts:       []runOption{withStoredPassword(common.ServicePasswordTarget(sampleService()), "secret-pw")},
			wantStdout: "postgresql://tsdbadmin:secret-pw@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n",
		},
		{
			name:       "replica uses primary credentials",
			args:       []string{"db", "uri", "rep-67890", "--with-password"},
			mock:       setupGetReplica,
			opts:       []runOption{withStoredPassword(common.ServicePasswordTarget(sampleService()), "primary-pw")},
			wantStdout: "postgresql://tsdbadmin:primary-pw@rep-67890.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n",
		},
		{
			// A branch has its own password, saved under its own ID.
			name:       "branch uses its own credentials",
			args:       []string{"db", "uri", "br-12345", "--with-password"},
			opts:       []runOption{withExperimental(), withStoredPassword(common.BranchPasswordTarget(sampleBranch()), "branch-pw"), withStoredPassword(common.ServicePasswordTarget(sampleService()), "parent-pw")},
			mock:       func(m *mocks.MockClientWithResponsesInterface) { expectResolveTarget(m, "br-12345", sampleBranch()) },
			wantStdout: "postgresql://tsdbadmin:branch-pw@br-12345.test-project-123.branch.tsdb.cloud.timescale.com:36956/tsdb?sslmode=require\n",
		},
		{
			// Branches are a preview, so the help mentions them only behind
			// the gate. The generated docs/cli reference covers the ungated
			// help.
			name:       "help mentions branches when experimental",
			args:       []string{"db", "uri", "--help"},
			opts:       []runOption{withExperimental()},
			wantStdout: matchHelp("You can also pass a branch, by ID or name, to get a connection string for that branch."),
		},
	})
}
