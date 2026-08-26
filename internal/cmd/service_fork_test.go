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

func TestServiceForkCmd(t *testing.T) {
	// The default request sent by `service fork <id> --now`.
	baseReq := api.ForkServiceCreate{
		ForkStrategy:   api.ForkStrategyNOW,
		EnvironmentTag: new(api.EnvironmentTagDEV),
	}

	forked := sampleService(func(s *api.Service) {
		s.ServiceID = "svc-67890"
		s.Name = "test-service-fork"
	})

	// Env rendering of the forked service, for cases where the table would be
	// noise. The endpoint host comes from sampleService.
	forkedEnv := `PGHOST=svc-12345.project.tsdb.cloud.timescale.com
PGPORT=5432
PGDATABASE=tsdb
PGUSER=tsdbadmin
`

	// stderr for --no-wait --no-set-default fork variants, which differ only
	// in the strategy description of the first line.
	noWaitStderr := func(firstLine string) string {
		return firstLine + `
✅ Fork request accepted!
📋 New Service ID: svc-67890
⏳ Service is being forked. Use 'tiger service list' to check status.
`
	}

	tests := []cmdTest{
		{
			name:    "missing timing flag",
			args:    []string{"service", "fork", "svc-12345"},
			wantErr: "must specify --now, --last-snapshot or --to-timestamp",
		},
		{
			name:    "multiple timing flags",
			args:    []string{"service", "fork", "svc-12345", "--now", "--last-snapshot"},
			wantErr: "can only specify one of --now, --last-snapshot or --to-timestamp",
		},
		{
			name:    "unparseable timestamp",
			args:    []string{"service", "fork", "svc-12345", "--to-timestamp", "not-a-timestamp"},
			wantErr: "invalid argument \"not-a-timestamp\" for \"--to-timestamp\" flag: invalid time format `not-a-timestamp` must be one of: `2006-01-02T15:04:05Z07:00`",
		},
		{
			name:    "invalid environment",
			args:    []string{"service", "fork", "svc-12345", "--now", "--environment", "staging"},
			wantErr: "environment must be one of: DEV, PROD (got 'STAGING')",
		},
		{
			name:    "not logged in",
			args:    []string{"service", "fork", "svc-12345", "--now"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: "authentication required: not logged in. Please run 'tiger auth login'",
		},
		{
			name:    "read-only mode",
			args:    []string{"service", "fork", "svc-12345", "--now"},
			opts:    []runOption{withConfig(map[string]any{"read_only": true})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name:    "missing service id",
			args:    []string{"service", "fork", "--now"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "invalid cpu/memory combination",
			args:    []string{"service", "fork", "svc-12345", "--now", "--cpu", "999", "--memory", "1"},
			wantErr: "invalid CPU/Memory combination. Allowed combinations: shared/shared, 0.5 CPU/2 GB, 1 CPU/4 GB, 2 CPU/8 GB, 4 CPU/16 GB, 8 CPU/32 GB, 16 CPU/64 GB, 32 CPU/128 GB",
		},
		{
			name: "network error",
			args: []string{"service", "fork", "svc-12345", "--now"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "svc-12345", baseReq).
					Return(nil, errors.New("connection refused"))
			},
			wantErr:    "failed to fork Service: connection refused",
			wantStderr: "🍴 Forking service 'svc-12345' to create '(auto-generated)' at current state...\nError: failed to fork Service: connection refused\n",
		},
		{
			name: "API error",
			args: []string{"service", "fork", "svc-12345", "--now"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "svc-12345", baseReq).
					Return(&api.ForkServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr:    "service not found",
			wantStderr: "🍴 Forking service 'svc-12345' to create '(auto-generated)' at current state...\nError: service not found\n",
			check:      checkExitCode(common.ExitServiceNotFound),
		},
		{
			name: "nil response body",
			args: []string{"service", "fork", "svc-12345", "--now"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "svc-12345", baseReq).
					Return(&api.ForkServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantErr:    "empty response from API",
			wantStderr: "🍴 Forking service 'svc-12345' to create '(auto-generated)' at current state...\nError: empty response from API\n",
		},
		{
			name: "fork now success with wait",
			args: []string{"service", "fork", "svc-12345", "--now"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				pwForked := sampleService(func(s *api.Service) {
					s.ServiceID = "svc-67890"
					s.Name = "test-service-fork"
					s.InitialPassword = new("fork-pass-123")
				})
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "svc-12345", baseReq).
					Return(&api.ForkServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &pwForked,
					}, nil)
			},
			wantStdout: sampleForkedServiceTable,
			wantStderr: `🍴 Forking service 'svc-12345' to create '(auto-generated)' at current state...
✅ Fork request accepted!
📋 New Service ID: svc-67890
🔐 Password saved to system keyring for automatic authentication
🎯 Set service 'svc-67890' as default service.
⏳ Waiting for fork to complete (timeout: 30m0s)...
🎉 Service fork completed successfully!
🔌 Run 'tiger db connect' to connect to your new service
`,
			check: func(t *testing.T, result cmdResult) {
				checkDefaultService("svc-67890")(t, result)
				checkStoredPassword("svc-67890", "fork-pass-123")(t, result)
			},
		},
		{
			name: "source service id from config",
			args: []string{"service", "fork", "--now", "--no-wait", "--no-set-default", "-o", "env"},
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "svc-12345", baseReq).
					Return(&api.ForkServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &forked,
					}, nil)
			},
			wantStdout: forkedEnv,
			wantStderr: noWaitStderr("🍴 Forking service 'svc-12345' to create '(auto-generated)' at current state..."),
			check:      checkDefaultService("svc-12345"),
		},
		{
			name: "last snapshot strategy",
			args: []string{"service", "fork", "svc-12345", "--last-snapshot", "--no-wait", "--no-set-default", "-o", "env"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ForkServiceCreate{
					ForkStrategy:   api.ForkStrategyLASTSNAPSHOT,
					EnvironmentTag: new(api.EnvironmentTagDEV),
				}).Return(&api.ForkServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &forked,
				}, nil)
			},
			wantStdout: forkedEnv,
			wantStderr: noWaitStderr("🍴 Forking service 'svc-12345' to create '(auto-generated)' at last snapshot..."),
		},
		{
			name: "to-timestamp strategy",
			args: []string{"service", "fork", "svc-12345", "--to-timestamp", "2025-01-15T10:30:00Z", "--no-wait", "--no-set-default", "-o", "env"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ForkServiceCreate{
					ForkStrategy:   api.ForkStrategyPITR,
					TargetTime:     new(time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)),
					EnvironmentTag: new(api.EnvironmentTagDEV),
				}).Return(&api.ForkServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &forked,
				}, nil)
			},
			wantStdout: forkedEnv,
			wantStderr: noWaitStderr("🍴 Forking service 'svc-12345' to create '(auto-generated)' at point-in-time: 2025-01-15T10:30:00Z..."),
		},
		{
			name: "custom name environment and resources",
			args: []string{
				"service", "fork", "svc-12345", "--now", "--name", "my-fork",
				"--cpu", "2000", "--memory", "8", "--environment", "prod",
				"--no-wait", "--no-set-default", "-o", "env",
			},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "svc-12345", api.ForkServiceCreate{
					ForkStrategy:   api.ForkStrategyNOW,
					Name:           new("my-fork"),
					CPUMillis:      new("2000"),
					MemoryGbs:      new("8"),
					EnvironmentTag: new(api.EnvironmentTagPROD),
				}).Return(&api.ForkServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &forked,
				}, nil)
			},
			wantStdout: forkedEnv,
			wantStderr: noWaitStderr("🍴 Forking service 'svc-12345' to create 'my-fork' at current state..."),
		},
	}

	runCmdTests(t, tests)
}

// sampleForkedServiceTable is the table rendering of sampleService with
// ServiceID svc-67890 and Name test-service-fork.
const sampleForkedServiceTable = `┌───────────────────┬─────────────────────────────────────────────────────────────────────────────────────────────┐
│     PROPERTY      │                                            VALUE                                            │
├───────────────────┼─────────────────────────────────────────────────────────────────────────────────────────────┤
│ Service ID        │ svc-67890                                                                                   │
│ Name              │ test-service-fork                                                                           │
│ Status            │ READY                                                                                       │
│ Type              │ TIMESCALEDB                                                                                 │
│ Region            │ us-east-1                                                                                   │
│ CPU               │ 1 cores (1000m)                                                                             │
│ Memory            │ 4 GB                                                                                        │
│ Direct Endpoint   │ svc-12345.project.tsdb.cloud.timescale.com:5432                                             │
│ Created           │ 2025-01-15 10:30:00 UTC                                                                     │
│ Connection String │ postgresql://tsdbadmin@svc-12345.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require │
│ Console URL       │ https://console.cloud.tigerdata.com/dashboard/services/svc-67890                            │
└───────────────────┴─────────────────────────────────────────────────────────────────────────────────────────────┘
`
