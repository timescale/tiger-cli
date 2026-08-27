package cmd

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

const noServicesStderr = "🏜️  No services found! Your project is looking a bit empty.\n" +
	"🚀 Ready to get started? Create your first service with: tiger service create\n"

func TestServiceListCmd(t *testing.T) {
	services := []api.Service{
		// An initial password returned by the API must never appear in list
		// output — the json/yaml expectations below have no password fields.
		sampleService(func(s *api.Service) {
			s.InitialPassword = new("super-secret-pw")
		}),
		sampleService(func(s *api.Service) {
			s.ServiceID = "svc-67890"
			s.Name = "analytics-db"
			s.ServiceType = api.ServiceTypePOSTGRES
			s.RegionCode = "eu-west-1"
			s.Status = api.DeployStatusPAUSED
			s.Created = time.Date(2025, 2, 1, 8, 0, 0, 0, time.UTC)
			s.Endpoint = &api.Endpoint{
				Host: new("svc-67890.project.tsdb.cloud.timescale.com"),
				Port: new(5432),
			}
		}),
	}

	setupList := func(services []api.Service) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
				Return(&api.GetServicesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &services,
				}, nil)
		}
	}

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "list"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name: "network error",
			args: []string{"service", "list"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to list services: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "list"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
					Return(&api.GetServicesResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
			checks:  []checkFunc{checkExitCode(common.ExitGeneralError)},
		},
		{
			name: "nil response body",
			args: []string{"service", "list"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
					Return(&api.GetServicesResponse{
						HTTPResponse: httpResponse(http.StatusOK),
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "empty list",
			args:       []string{"service", "list"},
			setup:      setupList(nil),
			wantStderr: noServicesStderr,
		},
		{
			name:  "table output",
			args:  []string{"service", "list"},
			setup: setupList(services),
			wantStdout: `┌────────────┬──────────────┬────────┬─────────────┬───────────┬──────────────────┐
│ SERVICE ID │     NAME     │ STATUS │    TYPE     │  REGION   │     CREATED      │
├────────────┼──────────────┼────────┼─────────────┼───────────┼──────────────────┤
│ svc-12345  │ test-service │ READY  │ TIMESCALEDB │ us-east-1 │ 2025-01-15 10:30 │
│ svc-67890  │ analytics-db │ PAUSED │ POSTGRES    │ eu-west-1 │ 2025-02-01 08:00 │
└────────────┴──────────────┴────────┴─────────────┴───────────┴──────────────────┘
`,
		},
		{
			// The -o flag overrides the configured format for this run only:
			// the config file must not be rewritten.
			name:       "json output via flag overriding config",
			args:       []string{"service", "list", "-o", "json"},
			setup:      setupList(services),
			opts:       []runOption{withConfig(map[string]any{"output": "table"})},
			wantStdout: serviceListJSON,
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				configMap := readConfigFile(t, result.configDir)
				if got := configMap["output"]; got != "table" {
					t.Errorf("config output = %v, want table (config file must not be modified by -o)", got)
				}
			}},
		},
		{
			name:  "yaml output",
			args:  []string{"service", "list", "-o", "yaml"},
			setup: setupList(services),
			wantStdout: `- connection_string: postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require
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
- connection_string: postgresql://tsdbadmin@svc-67890.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require
  console_url: https://console.cloud.tigerdata.com/dashboard/services/svc-67890
  created: "2025-02-01T08:00:00Z"
  database: tsdb
  endpoint:
    host: svc-67890.project.tsdb.cloud.timescale.com
    port: 5432
  host: svc-67890.project.tsdb.cloud.timescale.com
  metrics: null
  name: analytics-db
  port: 5432
  project_id: test-project-123
  region_code: eu-west-1
  resources:
    - id: resource-1
      spec:
        cpu_millis: 1000
        memory_gbs: 4
  role: tsdbadmin
  service_id: svc-67890
  service_type: POSTGRES
  status: PAUSED
`,
		},
		{
			name:       "output format from config",
			args:       []string{"service", "list"},
			setup:      setupList(services),
			opts:       []runOption{withConfig(map[string]any{"output": "json"})},
			wantStdout: serviceListJSON,
		},
		{
			// "env" is rejected at flag-parse time for list, but can still
			// reach the output switch via a hand-edited config file.
			name:    "env output format from config",
			args:    []string{"service", "list"},
			setup:   setupList(services),
			opts:    []runOption{withConfig(map[string]any{"output": "env"})},
			wantErr: "environment variable output is not supported for multiple services",
		},
		{
			name:    "env output flag rejected",
			args:    []string{"service", "list", "-o", "env"},
			wantErr: `invalid argument "env" for "-o, --output" flag: invalid output format: env (must be one of: json, yaml, table)`,
		},
		{
			name:       "ls alias",
			args:       []string{"service", "ls"},
			setup:      setupList(nil),
			wantStderr: noServicesStderr,
		},
	}

	runCmdTests(t, tests)
}

// serviceListJSON is the JSON rendering of the two-service list used by
// TestServiceListCmd, shared by the flag-driven and config-driven json cases.
const serviceListJSON = `[
  {
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
  },
  {
    "created": "2025-02-01T08:00:00Z",
    "endpoint": {
      "host": "svc-67890.project.tsdb.cloud.timescale.com",
      "port": 5432
    },
    "metrics": null,
    "name": "analytics-db",
    "project_id": "test-project-123",
    "region_code": "eu-west-1",
    "resources": [
      {
        "id": "resource-1",
        "spec": {
          "cpu_millis": 1000,
          "memory_gbs": 4
        }
      }
    ],
    "service_id": "svc-67890",
    "service_type": "POSTGRES",
    "status": "PAUSED",
    "role": "tsdbadmin",
    "host": "svc-67890.project.tsdb.cloud.timescale.com",
    "port": 5432,
    "database": "tsdb",
    "connection_string": "postgresql://tsdbadmin@svc-67890.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require",
    "console_url": "https://console.cloud.tigerdata.com/dashboard/services/svc-67890"
  }
]
`
