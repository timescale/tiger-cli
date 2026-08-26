package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestDbConnectionStringCmd(t *testing.T) {
	const (
		directURI   = "postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n"
		readOnlyURI = "postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require&options=-c%20tsdb_admin.read_only_connection%3Dtrue\n"
		replicaURI  = "postgresql://tsdbadmin@rep-67890.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n"
	)

	setupGet := func(m *mocks.MockClientWithResponsesInterface) {
		expectGetService(m, "svc-12345", sampleService())
	}
	setupGetReplica := func(m *mocks.MockClientWithResponsesInterface) {
		expectGetService(m, "rep-67890", sampleReplica())
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

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "connection-string", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: "authentication required: not logged in. Please run 'tiger auth login'",
			check:   checkExitCode(common.ExitAuthenticationError),
		},
		{
			name:    "missing service id",
			args:    []string{"db", "connection-string"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"db", "connection-string", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "API error",
			args: []string{"db", "connection-string", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			check:   checkExitCode(common.ExitServiceNotFound),
		},
		{
			name: "nil response body",
			args: []string{"db", "connection-string", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "positional service id",
			args:       []string{"db", "connection-string", "svc-12345"},
			setup:      setupGet,
			wantStdout: directURI,
		},
		{
			name:       "default service id from config",
			args:       []string{"db", "connection-string"},
			setup:      setupGet,
			opts:       []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			wantStdout: directURI,
		},
		{
			name:       "uri alias",
			args:       []string{"db", "uri", "svc-12345"},
			setup:      setupGet,
			wantStdout: directURI,
		},
		{
			name:       "custom role",
			args:       []string{"db", "connection-string", "svc-12345", "--role", "readonly"},
			setup:      setupGet,
			wantStdout: "postgresql://readonly@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n",
		},
		{
			name: "pooled with pooler available",
			args: []string{"db", "connection-string", "svc-12345", "--pooled"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "svc-12345", sampleService(withPooler))
			},
			wantStdout: "postgresql://tsdbadmin@pooler.svc-12345.project.tsdb.cloud.timescale.com:6432/tsdb?sslmode=require\n",
		},
		{
			name:    "pooled without pooler",
			args:    []string{"db", "connection-string", "svc-12345", "--pooled"},
			setup:   setupGet,
			wantErr: "connection pooler not available for this service",
		},
		{
			name:    "with-password without stored password",
			args:    []string{"db", "connection-string", "svc-12345", "--with-password"},
			setup:   setupGet,
			wantErr: "password not available to include in connection string",
		},
		{
			name:       "read-only flag",
			args:       []string{"db", "connection-string", "svc-12345", "--read-only"},
			setup:      setupGet,
			wantStdout: readOnlyURI,
		},
		{
			name:       "read-only from config",
			args:       []string{"db", "connection-string", "svc-12345"},
			setup:      setupGet,
			opts:       []runOption{withConfig(map[string]any{"read_only": true})},
			wantStdout: readOnlyURI,
		},
		{
			name:       "read-only flag and config",
			args:       []string{"db", "connection-string", "svc-12345", "--read-only"},
			setup:      setupGet,
			opts:       []runOption{withConfig(map[string]any{"read_only": true})},
			wantStdout: readOnlyURI,
		},
		{
			name:       "replica target uses replica endpoint",
			args:       []string{"db", "connection-string", "rep-67890"},
			setup:      setupGetReplica,
			wantStdout: replicaURI,
		},
		{
			name: "replica pooled with pooler uses replica pooler",
			args: []string{"db", "connection-string", "rep-67890", "--pooled"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "rep-67890", sampleReplica(func(s *api.Service) {
					s.ConnectionPooler = &api.ConnectionPooler{
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
			args:       []string{"db", "connection-string", "rep-67890", "--pooled"},
			setup:      setupGetReplica,
			wantStdout: replicaURI,
			wantStderr: "⚠️  Warning: read replica \"replica-service\" has no connection pooler; connecting directly instead\n",
		},
		{
			name: "replica parent fetch error",
			args: []string{"db", "connection-string", "rep-67890"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "rep-67890", sampleReplica())
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch parent service \"svc-12345\" for read replica: failed to fetch service details: connection refused",
		},
	}

	runCmdTests(t, tests)

	// These need a password seeded in the (mock) keyring before the command
	// runs, which the table's setup hook can't do, so they run as standalone
	// subtests.
	t.Run("with-password includes stored password", func(t *testing.T) {
		config.SetTestServiceName(t)
		storage := &common.KeyringStorage{}
		if err := storage.Save(sampleService(), "secret-pw", "tsdbadmin"); err != nil {
			t.Fatalf("failed to seed password: %v", err)
		}

		result := runCommand(t, []string{"db", "connection-string", "svc-12345", "--with-password"}, setupGet)
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "postgresql://tsdbadmin:secret-pw@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n")
		assertOutput(t, result.stderr, "")
	})

	t.Run("replica uses primary credentials", func(t *testing.T) {
		config.SetTestServiceName(t)
		storage := &common.KeyringStorage{}
		if err := storage.Save(sampleService(), "primary-pw", "tsdbadmin"); err != nil {
			t.Fatalf("failed to seed password: %v", err)
		}

		result := runCommand(t, []string{"db", "connection-string", "rep-67890", "--with-password"}, setupGetReplica)
		if result.err != nil {
			t.Fatalf("unexpected error: %v", result.err)
		}
		assertOutput(t, result.stdout, "postgresql://tsdbadmin:primary-pw@rep-67890.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require\n")
		assertOutput(t, result.stderr, "")
	})
}
