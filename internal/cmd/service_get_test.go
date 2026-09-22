package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceGetCmd(t *testing.T) {
	// A fully populated service, exercising every optional table row: an
	// environment tag, HA replicas, and a connection pooler endpoint.
	fullService := sampleService(func(s *api.Service) {
		s.Metadata = &api.ServiceMetadata{Environment: new("PROD")}
		s.Resources[0].Spec.CPUMillis = new(2000)
		s.Resources[0].Spec.MemoryGbs = new(8)
		s.HaReplicas = &api.HAReplica{ReplicaCount: new(2)}
		s.ConnectionPooler = &api.ConnectionPooler{
			Endpoint: &api.Endpoint{
				Host: new("svc-12345.project.pooler.tsdb.cloud.timescale.com"),
				Port: new(6432),
			},
		}
	})

	// Free tier services report null CPU and memory, rendered as "shared".
	freeTierService := sampleService(func(s *api.Service) {
		s.Resources[0].Spec.CPUMillis = nil
		s.Resources[0].Spec.MemoryGbs = nil
	})

	withInitialPassword := sampleService(func(s *api.Service) {
		s.InitialPassword = new("super-secret-pw")
	})

	setupGet := func(service api.Service) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().ResolveServiceRefWithResponse(validCtx, testProjectID, api.ServiceRefRequest{Ref: "svc-12345"}).
				Return(&api.ResolveServiceRefResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &service,
				}, nil)
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "get", "svc-12345"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "no service id",
			args:    []string{"service", "get"},
			wantErr: "service is required. Provide it as an argument or set a default with 'tiger config set service_id <name-or-id>'",
		},
		{
			name: "network error",
			args: []string{"service", "get", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResolveServiceRefWithResponse(validCtx, testProjectID, api.ServiceRefRequest{Ref: "svc-12345"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: `failed to resolve service "svc-12345": connection refused`,
		},
		{
			name: "not found",
			args: []string{"service", "get", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResolveServiceRefWithResponse(validCtx, testProjectID, api.ServiceRefRequest{Ref: "svc-12345"}).
					Return(&api.ResolveServiceRefResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			// A ref matching more than one service is refused by the API; the
			// CLI surfaces that message and points at the command that lists
			// the candidates, which the message itself doesn't name.
			name: "ambiguous ref refused",
			args: []string{"service", "get", "my-api-db"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResolveServiceRefWithResponse(validCtx, testProjectID, api.ServiceRefRequest{Ref: "my-api-db"}).
					Return(&api.ResolveServiceRefResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.ClientError{Message: new("ambiguous service name matches multiple services")},
					}, nil)
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "nil response body",
			args: []string{"service", "get", "svc-12345"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ResolveServiceRefWithResponse(validCtx, testProjectID, api.ServiceRefRequest{Ref: "svc-12345"}).
					Return(&api.ResolveServiceRefResponse{
						HTTPResponse: httpResponse(http.StatusOK),
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:  "table output",
			args:  []string{"service", "get", "svc-12345"},
			setup: setupGet(fullService),
			wantStdout: `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                            VALUE                                            │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────┤
│ Service ID        │ svc-12345                                                                                   │
│ Name              │ test-service                                                                                │
│ Status            │ READY                                                                                       │
│ Type              │ TIMESCALEDB                                                                                 │
│ Region            │ us-east-1                                                                                   │
│ Environment       │ PROD                                                                                        │
│ CPU               │ 2 cores (2000m)                                                                             │
│ Memory            │ 8 GB                                                                                        │
│ Replicas          │ 2                                                                                           │
│ Direct Endpoint   │ svc-12345.project.tsdb.cloud.timescale.com:5432                                             │
│ Pooler Endpoint   │ svc-12345.project.pooler.tsdb.cloud.timescale.com:6432                                      │
│ Created           │ 2025-01-15 10:30:00 UTC                                                                     │
│ Connection String │ postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345                            │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────┘
`,
		},
		{
			// The ref reaches the API as typed: no client-side classification
			// stands between a name and the service it names.
			name: "name resolves to the service",
			args: []string{"service", "get", "test-service", "-o", "env"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				svc := sampleService()
				m.EXPECT().ResolveServiceRefWithResponse(validCtx, testProjectID, api.ServiceRefRequest{Ref: "test-service"}).
					Return(&api.ResolveServiceRefResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &svc,
					}, nil)
			},
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
		{
			name:  "free tier table output",
			args:  []string{"service", "get", "svc-12345"},
			setup: setupGet(freeTierService),
			wantStdout: `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                            VALUE                                            │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────┤
│ Service ID        │ svc-12345                                                                                   │
│ Name              │ test-service                                                                                │
│ Status            │ READY                                                                                       │
│ Type              │ TIMESCALEDB                                                                                 │
│ Region            │ us-east-1                                                                                   │
│ CPU               │ shared                                                                                      │
│ Memory            │ shared                                                                                      │
│ Direct Endpoint   │ svc-12345.project.tsdb.cloud.timescale.com:5432                                             │
│ Created           │ 2025-01-15 10:30:00 UTC                                                                     │
│ Connection String │ postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345                            │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────┘
`,
		},
		{
			name:  "default service id from config",
			args:  []string{"service", "get"},
			setup: setupGet(freeTierService),
			opts:  []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			wantStdout: `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                            VALUE                                            │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────┤
│ Service ID        │ svc-12345                                                                                   │
│ Name              │ test-service                                                                                │
│ Status            │ READY                                                                                       │
│ Type              │ TIMESCALEDB                                                                                 │
│ Region            │ us-east-1                                                                                   │
│ CPU               │ shared                                                                                      │
│ Memory            │ shared                                                                                      │
│ Direct Endpoint   │ svc-12345.project.tsdb.cloud.timescale.com:5432                                             │
│ Created           │ 2025-01-15 10:30:00 UTC                                                                     │
│ Connection String │ postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345                            │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────┘
`,
		},
		{
			// A default must be an ID. The refusal comes after the lookup, so
			// it can name the ID to store instead of just saying "not found".
			// Tested here for all three layers; every command that reads the
			// default shares this path through getServiceRef.
			name: "name in TIGER_SERVICE_ID is refused",
			args: []string{"service", "get"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			opts: []runOption{withEnv("TIGER_SERVICE_ID", "test-service")},
			wantErr: `TIGER_SERVICE_ID is set to "test-service", the name of service svc-12345. ` +
				"A name here breaks as soon as the service is renamed.\n" +
				"Use svc-12345, pass the name as an argument, or run 'tiger config set service_id test-service' to store its ID",
			checks: []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "name in --service-id is refused",
			args: []string{"service", "get", "--service-id", "test-service"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			wantErr: `--service-id is set to "test-service", the name of service svc-12345. ` +
				"A name here breaks as soon as the service is renamed.\n" +
				"Use svc-12345, pass the name as an argument, or run 'tiger config set service_id test-service' to store its ID",
			checks: []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			// A hand-edited config file is the one way a name gets stored,
			// since 'tiger config set service_id' resolves before writing.
			name: "name in the config file is refused",
			args: []string{"service", "get"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			opts: []runOption{withConfig(map[string]any{"service_id": "test-service"})},
			wantErr: `the service_id config value is set to "test-service", the name of service svc-12345. ` +
				"A name here breaks as soon as the service is renamed.\n" +
				"Use svc-12345, pass the name as an argument, or run 'tiger config set service_id test-service' to store its ID",
			checks: []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			// The API's initial_password must never appear in output without
			// --with-password.
			name:  "json output omits password",
			args:  []string{"service", "get", "svc-12345", "-o", "json"},
			setup: setupGet(withInitialPassword),
			wantStdout: `{
  "created": "2025-01-15T10:30:00Z",
  "endpoint": {
    "host": "svc-12345.project.tsdb.cloud.timescale.com",
    "port": 5432
  },
  "name": "test-service",
  "project_id": "test-project-123",
  "region_code": "us-east-1",
  "resources": [
    {
      "id": "resource-1",
      "spec": {
        "cpu_millis": 1000,
        "memory_gbs": 4
      }
    }
  ],
  "service_id": "svc-12345",
  "service_type": "TIMESCALEDB",
  "status": "READY",
  "role": "tsdbadmin",
  "host": "svc-12345.project.tsdb.cloud.timescale.com",
  "port": 5432,
  "database": "tsdb",
  "connection_string": "postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require",
  "console_url": "https://console.cloud.tigerdata.com/dashboard/services/svc-12345"
}
`,
		},
		{
			name:  "yaml output",
			args:  []string{"service", "get", "svc-12345", "-o", "yaml"},
			setup: setupGet(sampleService()),
			wantStdout: `connection_string: postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require
console_url: https://console.cloud.tigerdata.com/dashboard/services/svc-12345
created: "2025-01-15T10:30:00Z"
database: tsdb
endpoint:
  host: svc-12345.project.tsdb.cloud.timescale.com
  port: 5432
host: svc-12345.project.tsdb.cloud.timescale.com
name: test-service
port: 5432
project_id: test-project-123
region_code: us-east-1
resources:
  - id: resource-1
    spec:
      cpu_millis: 1000
      memory_gbs: 4
role: tsdbadmin
service_id: svc-12345
service_type: TIMESCALEDB
status: READY
`,
		},
		{
			name:       "env output",
			args:       []string{"service", "get", "svc-12345", "-o", "env"},
			setup:      setupGet(sampleService()),
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
		{
			name:       "env output with password",
			args:       []string{"service", "get", "svc-12345", "-o", "env", "--with-password"},
			setup:      setupGet(withInitialPassword),
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\nPGPASSWORD=super-secret-pw\n",
		},
		{
			name:  "table output with password",
			args:  []string{"service", "get", "svc-12345", "--with-password"},
			setup: setupGet(withInitialPassword),
			wantStdout: `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                                    VALUE                                                    │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ Service ID        │ svc-12345                                                                                                   │
│ Name              │ test-service                                                                                                │
│ Status            │ READY                                                                                                       │
│ Type              │ TIMESCALEDB                                                                                                 │
│ Region            │ us-east-1                                                                                                   │
│ CPU               │ 1 cores (1000m)                                                                                             │
│ Memory            │ 4 GB                                                                                                        │
│ Direct Endpoint   │ svc-12345.project.tsdb.cloud.timescale.com:5432                                                             │
│ Created           │ 2025-01-15 10:30:00 UTC                                                                                     │
│ Password          │ super-secret-pw                                                                                             │
│ Connection String │ postgresql://tsdbadmin:super-secret-pw@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345                                            │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
`,
		},
		{
			name:    "with password unavailable",
			args:    []string{"service", "get", "svc-12345", "--with-password"},
			setup:   setupGet(sampleService()),
			wantErr: "password requested but not available for service svc-12345",
		},
		{
			name: "no endpoint warning",
			args: []string{"service", "get", "svc-12345"},
			setup: setupGet(sampleService(func(s *api.Service) {
				s.Endpoint = nil
			})),
			wantStdout: `┌─────────────┬──────────────────────────────────────────────────────────────────┐
│  PROPERTY   │                              VALUE                               │
├─────────────┼──────────────────────────────────────────────────────────────────┤
│ Service ID  │ svc-12345                                                        │
│ Name        │ test-service                                                     │
│ Status      │ READY                                                            │
│ Type        │ TIMESCALEDB                                                      │
│ Region      │ us-east-1                                                        │
│ CPU         │ 1 cores (1000m)                                                  │
│ Memory      │ 4 GB                                                             │
│ Created     │ 2025-01-15 10:30:00 UTC                                          │
│ Console URL │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345 │
└─────────────┴──────────────────────────────────────────────────────────────────┘
`,
			wantStderr: "Warning: Failed to get connection details: service endpoint not available\n",
		},
		{
			name:       "describe alias",
			args:       []string{"service", "describe", "svc-12345", "-o", "env"},
			setup:      setupGet(sampleService()),
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
		{
			name:       "show alias",
			args:       []string{"service", "show", "svc-12345", "-o", "env"},
			setup:      setupGet(sampleService()),
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
	})
}
