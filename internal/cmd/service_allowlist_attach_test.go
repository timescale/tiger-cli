package cmd

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceAllowListAttachCmd(t *testing.T) {
	// The command is experimental-gated (see the gate test in service_test.go),
	// so every case registers it explicitly.
	experimental := withEnv("TIGER_EXPERIMENTAL", "true")

	attached := sampleService()

	// overrides apply to the resolved service, whose tag the prod gate reads.
	setupAttach := func(overrides ...func(*api.Service)) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectResolveRef(m, "svc-12345", sampleService(overrides...))
			m.EXPECT().AttachServiceToAllowListWithResponse(validCtx, testProjectID, "svc-12345", api.ServiceAllowListInput{AllowListID: "1234567890"}).
				Return(&api.AttachServiceToAllowListResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &attached,
				}, nil)
		}
	}

	runCmdTests(t, []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890"},
			opts:    []runOption{experimental, withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "missing service id",
			args:    []string{"service", "allowlist", "attach", "--allow-list", "1234567890"},
			opts:    []runOption{experimental},
			wantErr: "service name or ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "missing allow-list flag",
			args:    []string{"service", "allowlist", "attach", "svc-12345"},
			opts:    []runOption{experimental},
			wantErr: `required flag(s) "allow-list" not set`,
		},
		{
			name:    "read-only all refuses",
			args:    []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890"},
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "read-only prod refuses PROD service",
			args:    []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890"},
			opts:    []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD"),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			args:       []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890"},
			opts:       []runOption{experimental, withConfig(map[string]any{"read_only": "prod"})},
			mock:       setupAttach(envTag("DEV")),
			wantStderr: "Service 'test-service' (svc-12345) attached to IP allow list '1234567890'.\n",
		},
		{
			name: "ambiguous name refused",
			args: []string{"service", "allowlist", "attach", "my-api-db", "--allow-list", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefStatus(m, "my-api-db", http.StatusBadRequest, &api.Error{Message: new("ambiguous service name matches multiple services")})
			},
			wantErr: "ambiguous service name matches multiple services\nRun 'tiger service list' to find the ID you want",
			checks:  []checkFunc{checkExitCode(common.ExitInvalidParameters)},
		},
		{
			name: "network error",
			args: []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().AttachServiceToAllowListWithResponse(validCtx, testProjectID, "svc-12345", api.ServiceAllowListInput{AllowListID: "1234567890"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to attach service to IP allow list: connection refused",
		},
		{
			name: "API error",
			args: []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().AttachServiceToAllowListWithResponse(validCtx, testProjectID, "svc-12345", api.ServiceAllowListInput{AllowListID: "1234567890"}).
					Return(&api.AttachServiceToAllowListResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("IP allow list not found")},
					}, nil)
			},
			wantErr: "IP allow list not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body",
			args: []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890"},
			opts: []runOption{experimental},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectResolveRefID(m, "svc-12345")
				m.EXPECT().AttachServiceToAllowListWithResponse(validCtx, testProjectID, "svc-12345", api.ServiceAllowListInput{AllowListID: "1234567890"}).
					Return(&api.AttachServiceToAllowListResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "table output",
			args:       []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890"},
			opts:       []runOption{experimental},
			mock:       setupAttach(),
			wantStderr: "Service 'test-service' (svc-12345) attached to IP allow list '1234567890'.\n",
		},
		{
			name:       "default service id from config",
			args:       []string{"service", "allowlist", "attach", "--allow-list", "1234567890"},
			opts:       []runOption{experimental, withConfig(map[string]any{"service_id": "svc-12345"})},
			mock:       setupAttach(),
			wantStderr: "Service 'test-service' (svc-12345) attached to IP allow list '1234567890'.\n",
		},
		{
			name:       "json output",
			args:       []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890", "-o", "json"},
			opts:       []runOption{experimental},
			mock:       setupAttach(),
			wantStderr: "Service 'test-service' (svc-12345) attached to IP allow list '1234567890'.\n",
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
			name:       "yaml output",
			args:       []string{"service", "allowlist", "attach", "svc-12345", "--allow-list", "1234567890", "-o", "yaml"},
			opts:       []runOption{experimental},
			mock:       setupAttach(),
			wantStderr: "Service 'test-service' (svc-12345) attached to IP allow list '1234567890'.\n",
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
	})
}
