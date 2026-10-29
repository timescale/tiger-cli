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

	fluidService := sampleService(fluidPlatform)
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	withInitialPassword := sampleService(func(s *api.Service) {
		s.InitialPassword = new("super-secret-pw")
	})

	setupGet := func(service api.Service) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, "svc-12345", service)
		}
	}

	// setupGetFluid also lists the fluid service's two branches, which only
	// service get does, behind the experimental gate.
	setupGetFluid := func(m *mocks.MockClientWithResponsesInterface) {
		expectResolveRef(m, "svc-12345", fluidService)
		branches := []api.Branch{sampleBranch(), sampleBranch()}
		m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "svc-12345", nil).
			Return(&api.GetBranchesResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &branches,
			}, nil)
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
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error",
			args: []string{"service", "get", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefError(m, "svc-12345", errors.New("connection refused"))
			},
			wantErr: `failed to resolve service 'svc-12345': connection refused`,
		},
		{
			name: "service not found",
			args: []string{"service", "get", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefNotFound(m, "svc-12345")
			},
			wantErr: "service 'svc-12345' not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			// A ref matching more than one service is refused by the API; the
			// CLI surfaces that message and points at the command that lists
			// the candidates, which the message itself doesn't name.
			name: "ambiguous name refused",
			args: []string{"service", "get", "my-api-db"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "nil response body",
			args: []string{"service", "get", "svc-12345"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "svc-12345", http.StatusOK, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// An ambiguous ref is refused with a 400, so more than one match
			// means the filter was ignored; the CLI refuses rather than pick.
			name: "more than one match refused",
			args: []string{"service", "get", "my-api-db"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServicesWithResponse(validCtx, testProjectID, &api.GetServicesParams{Ref: new("my-api-db")}).
					Return(&api.GetServicesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &[]api.Service{sampleService(), sampleService()},
					}, nil)
			},
			wantErr: "expected one service for 'my-api-db', got 2",
		},
		{
			// A default must be an ID. The refusal comes after the lookup, so
			// it can name the ID to store instead of just saying "not found".
			// Tested here for all three layers; every command that reads the
			// default shares this path through getServiceRef.
			name: "name in TIGER_SERVICE_ID is refused",
			args: []string{"service", "get"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			opts: []runOption{withEnv("TIGER_SERVICE_ID", "test-service")},
			wantErr: "the default service must be an ID, but TIGER_SERVICE_ID is set to the name 'test-service'. " +
				"Set it to svc-12345, or pass the name as an argument",
			checks: []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "name in --service-id is refused",
			args: []string{"service", "get", "--service-id", "test-service"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			wantErr: "the default service must be an ID, but --service-id is set to the name 'test-service'. " +
				"Set it to svc-12345, or pass the name as an argument",
			checks: []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "name in the config file is refused",
			args: []string{"service", "get"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			opts: []runOption{withConfig(map[string]any{"service_id": "test-service"})},
			wantErr: "the default service must be an ID, but the service_id config value is set to the name 'test-service'. " +
				"Set it to svc-12345, or pass the name as an argument",
			checks: []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "table output",
			args: []string{"service", "get", "svc-12345"},
			mock: setupGet(fullService),
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
			name: "free tier table output",
			args: []string{"service", "get", "svc-12345"},
			mock: setupGet(freeTierService),
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
			// Fractional cores print with as many decimal places as they need.
			name: "fractional cpu table output",
			args: []string{"service", "get", "svc-12345"},
			mock: setupGet(sampleService(func(s *api.Service) {
				s.Resources[0].Spec.CPUMillis = new(250)
				s.Resources[0].Spec.MemoryGbs = new(2)
			})),
			wantStdout: `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                            VALUE                                            │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────┤
│ Service ID        │ svc-12345                                                                                   │
│ Name              │ test-service                                                                                │
│ Status            │ READY                                                                                       │
│ Type              │ TIMESCALEDB                                                                                 │
│ Region            │ us-east-1                                                                                   │
│ CPU               │ 0.25 cores (250m)                                                                           │
│ Memory            │ 2 GB                                                                                        │
│ Direct Endpoint   │ svc-12345.project.tsdb.cloud.timescale.com:5432                                             │
│ Created           │ 2025-01-15 10:30:00 UTC                                                                     │
│ Connection String │ postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345                            │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────┘
`,
		},
		{
			name: "default service id from config",
			args: []string{"service", "get"},
			mock: setupGet(freeTierService),
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
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
			// The API's initial_password must never appear in output without
			// --with-password.
			name: "json output omits password",
			args: []string{"service", "get", "svc-12345", "-o", "json"},
			mock: setupGet(withInitialPassword),
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
			name: "yaml output",
			args: []string{"service", "get", "svc-12345", "-o", "yaml"},
			mock: setupGet(sampleService()),
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
			// The platform, IOPS, and branch count are preview fields: the
			// output cases above carry a platform and IOPS from sampleService
			// and drop them, while these show all three with the gate.
			name: "experimental table output shows platform, IOPS, and branches",
			args: []string{"service", "get", "svc-12345"},
			mock: setupGetFluid,
			opts: []runOption{experimental},
			wantStdout: `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                            VALUE                                            │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────┤
│ Service ID        │ svc-12345                                                                                   │
│ Name              │ test-service                                                                                │
│ Status            │ READY                                                                                       │
│ Platform          │ FLUID                                                                                       │
│ Type              │ TIMESCALEDB                                                                                 │
│ Region            │ us-east-1                                                                                   │
│ CPU               │ 1 cores (1000m)                                                                             │
│ Memory            │ 4 GB                                                                                        │
│ IOPS              │ 6000                                                                                        │
│ Branches          │ 2                                                                                           │
│ Direct Endpoint   │ svc-12345.project.tsdb.cloud.timescale.com:5432                                             │
│ Created           │ 2025-01-15 10:30:00 UTC                                                                     │
│ Connection String │ postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345                            │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────┘
`,
		},
		{
			name: "experimental json output includes platform, IOPS, and branches",
			args: []string{"service", "get", "svc-12345", "-o", "json"},
			mock: setupGetFluid,
			opts: []runOption{experimental},
			wantStdout: `{
  "created": "2025-01-15T10:30:00Z",
  "endpoint": {
    "host": "svc-12345.project.tsdb.cloud.timescale.com",
    "port": 5432
  },
  "name": "test-service",
  "platform": "FLUID",
  "project_id": "test-project-123",
  "region_code": "us-east-1",
  "resources": [
    {
      "id": "resource-1",
      "spec": {
        "cpu_millis": 1000,
        "iops": 6000,
        "memory_gbs": 4
      }
    }
  ],
  "service_id": "svc-12345",
  "service_type": "TIMESCALEDB",
  "status": "READY",
  "branch_count": 2,
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
			name: "experimental yaml output includes platform, IOPS, and branches",
			args: []string{"service", "get", "svc-12345", "-o", "yaml"},
			mock: setupGetFluid,
			opts: []runOption{experimental},
			wantStdout: `branch_count: 2
connection_string: postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require
console_url: https://console.cloud.tigerdata.com/dashboard/services/svc-12345
created: "2025-01-15T10:30:00Z"
database: tsdb
endpoint:
  host: svc-12345.project.tsdb.cloud.timescale.com
  port: 5432
host: svc-12345.project.tsdb.cloud.timescale.com
name: test-service
platform: FLUID
port: 5432
project_id: test-project-123
region_code: us-east-1
resources:
  - id: resource-1
    spec:
      cpu_millis: 1000
      iops: 6000
      memory_gbs: 4
role: tsdbadmin
service_id: svc-12345
service_type: TIMESCALEDB
status: READY
`,
		},
		{
			// Only fluid services can be branched, so a standard one isn't
			// counted. The mock fails the test on any branch listing.
			name:       "experimental standard service lists no branches",
			args:       []string{"service", "get", "svc-12345", "-o", "env"},
			mock:       setupGet(sampleService()),
			opts:       []runOption{experimental},
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
		{
			name:       "fluid service lists no branches without the gate",
			args:       []string{"service", "get", "svc-12345", "-o", "env"},
			mock:       setupGet(fluidService),
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
		{
			// The count is the only thing the branch listing adds, so the
			// service is still shown without it.
			name: "branch count fails",
			args: []string{"service", "get", "svc-12345", "-o", "env"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "svc-12345", fluidService)
				m.EXPECT().GetBranchesWithResponse(validCtx, testProjectID, "svc-12345", nil).
					Return(nil, errors.New("connection refused"))
			},
			opts:       []runOption{experimental},
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
			wantStderr: "Warning: failed to count branches: failed to list branches: connection refused\n",
		},
		{
			name:       "env output",
			args:       []string{"service", "get", "svc-12345", "-o", "env"},
			mock:       setupGet(sampleService()),
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
		{
			name:       "env output with password",
			args:       []string{"service", "get", "svc-12345", "-o", "env", "--with-password"},
			mock:       setupGet(withInitialPassword),
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\nPGPASSWORD=super-secret-pw\n",
		},
		{
			name: "table output with password",
			args: []string{"service", "get", "svc-12345", "--with-password"},
			mock: setupGet(withInitialPassword),
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
			mock:    setupGet(sampleService()),
			wantErr: "password requested but not available for service svc-12345",
		},
		{
			name: "no endpoint warning",
			args: []string{"service", "get", "svc-12345"},
			mock: setupGet(sampleService(func(s *api.Service) {
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
			wantStderr: "Warning: Failed to get connection details: endpoint not available\n",
		},
		{
			// No port is guessed, for the direct endpoint or the pooler, so
			// an endpoint without one isn't shown or connected to.
			name: "endpoints without a port",
			args: []string{"service", "get", "svc-12345"},
			mock: setupGet(sampleService(func(s *api.Service) {
				s.Endpoint.Port = nil
				s.ConnectionPooler = &api.ConnectionPooler{
					Endpoint: &api.Endpoint{Host: new("svc-12345.project.pooler.tsdb.cloud.timescale.com")},
				}
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
			wantStderr: "Warning: Failed to get connection details: endpoint port not available\n",
		},
		{
			// The ref reaches the API as typed: no client-side classification
			// stands between a name and the service it names.
			name: "name resolves to the service",
			args: []string{"service", "get", "test-service", "-o", "env"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRef(m, "test-service", sampleService())
			},
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
		{
			name:       "describe alias",
			args:       []string{"service", "describe", "svc-12345", "-o", "env"},
			mock:       setupGet(sampleService()),
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
		{
			name:       "show alias",
			args:       []string{"service", "show", "svc-12345", "-o", "env"},
			mock:       setupGet(sampleService()),
			wantStdout: "PGHOST=svc-12345.project.tsdb.cloud.timescale.com\nPGPORT=5432\nPGDATABASE=tsdb\nPGUSER=tsdbadmin\n",
		},
	})
}
