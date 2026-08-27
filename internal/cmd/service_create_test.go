package cmd

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestServiceCreateCmd(t *testing.T) {
	// The default request sent when only --name is provided.
	baseReq := api.ServiceCreate{
		Name:           "test-service",
		ReplicaCount:   new(0),
		EnvironmentTag: new(api.EnvironmentTagDEV),
	}

	svc := sampleService()

	runCmdTests(t, []cmdTest{
		{
			name:    "invalid addon",
			args:    []string{"service", "create", "--name", "test-service", "--addons", "invalid-addon"},
			wantErr: "invalid add-on 'invalid-addon'. Valid add-ons: time-series, ai, or 'none' for PostgreSQL-only",
		},
		{
			name:    "negative replica count",
			args:    []string{"service", "create", "--name", "test-service", "--replicas=-1"},
			wantErr: "replica count must be non-negative (--replicas)",
		},
		{
			name:    "invalid environment",
			args:    []string{"service", "create", "--name", "test-service", "--environment", "staging"},
			wantErr: "environment must be one of: DEV, PROD (got 'STAGING')",
		},
		{
			name:    "invalid cpu/memory combination",
			args:    []string{"service", "create", "--name", "test-service", "--cpu", "3000", "--memory", "8"},
			wantErr: "invalid CPU/Memory combination. Allowed combinations: shared/shared, 0.5 CPU/2 GB, 1 CPU/4 GB, 2 CPU/8 GB, 4 CPU/16 GB, 8 CPU/32 GB, 16 CPU/64 GB, 32 CPU/128 GB",
		},
		{
			name:    "unparseable wait timeout",
			args:    []string{"service", "create", "--name", "test-service", "--wait-timeout", "invalid"},
			wantErr: `invalid argument "invalid" for "--wait-timeout" flag: time: invalid duration "invalid"`,
		},
		{
			name:    "zero wait timeout",
			args:    []string{"service", "create", "--name", "test-service", "--wait-timeout", "0s"},
			wantErr: "wait timeout must be positive, got 0s",
		},
		{
			name:    "negative wait timeout",
			args:    []string{"service", "create", "--name", "test-service", "--wait-timeout=-30m"},
			wantErr: "wait timeout must be positive, got -30m0s",
		},
		{
			name:    "not logged in",
			args:    []string{"service", "create", "--name", "test-service"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "read-only mode",
			args:    []string{"service", "create", "--name", "test-service"},
			opts:    []runOption{withConfig(map[string]any{"read_only": true})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name: "network error",
			args: []string{"service", "create", "--name", "test-service"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(nil, errors.New("connection refused"))
			},
			wantErr:    "failed to create Service: connection refused",
			wantStderr: "🚀 Creating service 'test-service'...\nError: failed to create Service: connection refused\n",
		},
		{
			name: "API error",
			args: []string{"service", "create", "--name", "test-service"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.Error{Message: new("service limit reached")},
					}, nil)
			},
			wantErr:    "service limit reached",
			wantStderr: "🚀 Creating service 'test-service'...\nError: service limit reached\n",
			checks:     []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "nil response body",
			args: []string{"service", "create", "--name", "test-service"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantErr:    "empty response from API",
			wantStderr: "🚀 Creating service 'test-service'...\nError: empty response from API\n",
		},
		{
			name: "success with wait, service immediately ready",
			args: []string{"service", "create", "--name", "test-service"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStdout: sampleServiceCreateTable,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🎯 Set service 'svc-12345' as default service.
⏳ Waiting for service to be ready (wait timeout: 30m0s)...
🎉 Service is ready and running!
`,
			checks: []checkFunc{checkDefaultService("svc-12345")},
		},
		{
			name: "initial password saved to keyring",
			args: []string{"service", "create", "--name", "test-service"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				pwSvc := sampleService(func(s *api.Service) {
					s.InitialPassword = new("init-pass-123")
				})
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &pwSvc,
					}, nil)
			},
			wantStdout: sampleServiceCreateTable,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🔐 Password saved to system keyring for automatic authentication
🎯 Set service 'svc-12345' as default service.
⏳ Waiting for service to be ready (wait timeout: 30m0s)...
🎉 Service is ready and running!
🔌 Run 'tiger db connect' to connect to your new service
`,
			checks: []checkFunc{checkStoredPassword("svc-12345", "init-pass-123")},
		},
		{
			name: "no set default",
			args: []string{"service", "create", "--name", "test-service", "--no-set-default"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				pwSvc := sampleService(func(s *api.Service) {
					s.InitialPassword = new("init-pass-123")
				})
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &pwSvc,
					}, nil)
			},
			wantStdout: sampleServiceCreateTable,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🔐 Password saved to system keyring for automatic authentication
⏳ Waiting for service to be ready (wait timeout: 30m0s)...
🎉 Service is ready and running!
🔌 Run 'tiger db connect svc-12345' to connect to your new service
`,
			checks: []checkFunc{checkDefaultService("")},
		},
		{
			name: "no wait",
			args: []string{"service", "create", "--name", "test-service", "--no-wait"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStdout: sampleServiceCreateTable,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🎯 Set service 'svc-12345' as default service.
⏳ Service is being created. Use 'tiger service list' to check status.
`,
			checks: []checkFunc{checkDefaultService("svc-12345")},
		},
		{
			name:     "wait polls until ready",
			synctest: true,
			args:     []string{"service", "create", "--name", "test-service"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				queued := sampleService(func(s *api.Service) {
					s.Status = api.DeployStatusQUEUED
				})
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &queued,
					}, nil)
				ready := sampleService()
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &ready,
					}, nil)
			},
			wantStdout: sampleServiceCreateTable,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🎯 Set service 'svc-12345' as default service.
⏳ Waiting for service to be ready (wait timeout: 30m0s)...
⢎  Service status: QUEUED
🎉 Service is ready and running!
`,
		},
		{
			name:     "wait timeout",
			synctest: true,
			args:     []string{"service", "create", "--name", "test-service"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				queued := sampleService(func(s *api.Service) {
					s.Status = api.DeployStatusQUEUED
				})
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &queued,
					}, nil)
				// The service never reaches the target state, so the wait
				// polls until the (virtual) deadline. The non-TTY spinner
				// dedupes repeated messages, so those polls add no stderr lines.
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &queued,
					}, nil).AnyTimes()
			},
			wantErr:    "wait timeout reached after 30m0s - service may still be provisioning",
			wantStdout: sampleServiceCreateQueuedTable,
			// SilenceErrors is set after the wait fails, so Cobra doesn't
			// print the usual "Error:" line.
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🎯 Set service 'svc-12345' as default service.
⏳ Waiting for service to be ready (wait timeout: 30m0s)...
⢎  Service status: QUEUED
❌ Error: wait timeout reached after 30m0s - service may still be provisioning
`,
			checks: []checkFunc{checkExitCode(common.ExitTimeout)},
		},
		{
			name: "json output",
			args: []string{"service", "create", "--name", "test-service", "--no-wait", "-o", "json"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStdout: sampleServiceCreateJSON,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🎯 Set service 'svc-12345' as default service.
⏳ Service is being created. Use 'tiger service list' to check status.
`,
		},
		{
			name: "yaml output",
			args: []string{"service", "create", "--name", "test-service", "--no-wait", "-o", "yaml"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStdout: sampleServiceCreateYAML,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🎯 Set service 'svc-12345' as default service.
⏳ Service is being created. Use 'tiger service list' to check status.
`,
		},
		{
			name: "env output",
			args: []string{"service", "create", "--name", "test-service", "--no-wait", "-o", "env"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStdout: `PGHOST=svc-12345.project.tsdb.cloud.timescale.com
PGPORT=5432
PGDATABASE=tsdb
PGUSER=tsdbadmin
`,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🎯 Set service 'svc-12345' as default service.
⏳ Service is being created. Use 'tiger service list' to check status.
`,
		},
		{
			name: "with-password includes initial password in output",
			args: []string{"service", "create", "--name", "test-service", "--no-wait", "--with-password", "-o", "env"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				pwSvc := sampleService(func(s *api.Service) {
					s.InitialPassword = new("init-pass-123")
				})
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &pwSvc,
					}, nil)
			},
			wantStdout: `PGHOST=svc-12345.project.tsdb.cloud.timescale.com
PGPORT=5432
PGDATABASE=tsdb
PGUSER=tsdbadmin
PGPASSWORD=init-pass-123
`,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🔐 Password saved to system keyring for automatic authentication
🎯 Set service 'svc-12345' as default service.
⏳ Service is being created. Use 'tiger service list' to check status.
`,
		},
		{
			name: "all creation flags mapped to request",
			args: []string{
				"service", "create", "--name", "test-service",
				"--addons", "time-series,ai", "--region", "eu-central-1",
				"--replicas", "2", "--cpu", "1000", "--memory", "4",
				"--environment", "prod", "--no-wait", "--no-set-default", "-o", "env",
			},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, api.ServiceCreate{
					Name:           "test-service",
					Addons:         &[]api.ServiceCreateAddons{"time-series", "ai"},
					RegionCode:     new("eu-central-1"),
					ReplicaCount:   new(2),
					CPUMillis:      new("1000"),
					MemoryGbs:      new("4"),
					EnvironmentTag: new(api.EnvironmentTagPROD),
				}).Return(&api.CreateServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
			},
			wantStdout: `PGHOST=svc-12345.project.tsdb.cloud.timescale.com
PGPORT=5432
PGDATABASE=tsdb
PGUSER=tsdbadmin
`,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
⏳ Service is being created. Use 'tiger service list' to check status.
`,
		},
		{
			name: "addons none sends empty list",
			args: []string{"service", "create", "--name", "test-service", "--addons", "none", "--no-wait", "--no-set-default", "-o", "env"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, api.ServiceCreate{
					Name:           "test-service",
					Addons:         &[]api.ServiceCreateAddons{},
					ReplicaCount:   new(0),
					EnvironmentTag: new(api.EnvironmentTagDEV),
				}).Return(&api.CreateServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
			},
			wantStdout: `PGHOST=svc-12345.project.tsdb.cloud.timescale.com
PGPORT=5432
PGDATABASE=tsdb
PGUSER=tsdbadmin
`,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
⏳ Service is being created. Use 'tiger service list' to check status.
`,
		},
		{
			name: "output flag does not persist to config file",
			args: []string{"service", "create", "--name", "test-service", "--no-wait", "-o", "json"},
			opts: []runOption{withConfig(map[string]any{"output": "yaml"})},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &svc,
					}, nil)
			},
			wantStdout: sampleServiceCreateJSON,
			wantStderr: `🚀 Creating service 'test-service'...
✅ Service creation request accepted!
📋 Service ID: svc-12345
🎯 Set service 'svc-12345' as default service.
⏳ Service is being created. Use 'tiger service list' to check status.
`,
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				// setDefaultService rewrites the config file; the -o flag must
				// not leak into it.
				cfg, err := config.Load(testFlags(t, result.configDir))
				if err != nil {
					t.Fatalf("failed to load config: %v", err)
				}
				if cfg.Output != "yaml" {
					t.Errorf("config output = %q, want %q", cfg.Output, "yaml")
				}
			}},
		},
		{
			// The generated name is random, so the request is matched on its
			// "db-" prefix and stderr on the auto-generated-name notice.
			name: "auto-generated name",
			args: []string{"service", "create", "--no-wait", "--no-set-default", "-o", "env"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, gomock.Cond(func(x any) bool {
					req, ok := x.(api.CreateServiceJSONRequestBody)
					return ok && strings.HasPrefix(req.Name, "db-")
				})).Return(&api.CreateServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
			},
			wantStdout: `PGHOST=svc-12345.project.tsdb.cloud.timescale.com
PGPORT=5432
PGDATABASE=tsdb
PGUSER=tsdbadmin
`,
			wantStderr: matchFunc(func(t *testing.T, got string) {
				if !strings.Contains(got, "(auto-generated name)...") {
					t.Errorf("expected stderr to mention the auto-generated name, got: %s", got)
				}
			}),
		},
	})
}

// sampleServiceCreateTable is the table rendering of sampleService() as
// printed by service create.
const sampleServiceCreateTable = `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                            VALUE                                            │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────┤
│ Service ID        │ svc-12345                                                                                   │
│ Name              │ test-service                                                                                │
│ Status            │ READY                                                                                       │
│ Type              │ TIMESCALEDB                                                                                 │
│ Region            │ us-east-1                                                                                   │
│ CPU               │ 1 cores (1000m)                                                                             │
│ Memory            │ 4 GB                                                                                        │
│ Direct Endpoint   │ svc-12345.project.tsdb.cloud.timescale.com:5432                                             │
│ Created           │ 2025-01-15 10:30:00 UTC                                                                     │
│ Connection String │ postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345                            │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────┘
`

// sampleServiceCreateQueuedTable is the same table with a QUEUED status.
const sampleServiceCreateQueuedTable = `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                            VALUE                                            │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────┤
│ Service ID        │ svc-12345                                                                                   │
│ Name              │ test-service                                                                                │
│ Status            │ QUEUED                                                                                      │
│ Type              │ TIMESCALEDB                                                                                 │
│ Region            │ us-east-1                                                                                   │
│ CPU               │ 1 cores (1000m)                                                                             │
│ Memory            │ 4 GB                                                                                        │
│ Direct Endpoint   │ svc-12345.project.tsdb.cloud.timescale.com:5432                                             │
│ Created           │ 2025-01-15 10:30:00 UTC                                                                     │
│ Connection String │ postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-12345                            │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────┘
`

const sampleServiceCreateJSON = `{
  "created": "2025-01-15T10:30:00Z",
  "endpoint": {
    "host": "svc-12345.project.tsdb.cloud.timescale.com",
    "port": 5432
  },
  "metrics": null,
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
`

const sampleServiceCreateYAML = `connection_string: postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require
console_url: https://console.cloud.tigerdata.com/dashboard/services/svc-12345
created: "2025-01-15T10:30:00Z"
database: tsdb
endpoint:
  host: svc-12345.project.tsdb.cloud.timescale.com
  port: 5432
host: svc-12345.project.tsdb.cloud.timescale.com
metrics: null
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
`
