package mcp

import (
	"errors"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceFork(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "NOW"}

	// The request the SDK's schema defaults produce for a call that sets
	// nothing but service_id and fork_strategy.
	baseReq := api.ForkServiceCreate{
		ForkStrategy:   api.ForkStrategyNOW,
		EnvironmentTag: new(api.EnvironmentTagDEV),
	}

	forkedService := func(overrides ...func(*api.Service)) api.Service {
		return sampleService(append([]func(*api.Service){func(s *api.Service) {
			s.ServiceID = "u8me885b93"
			s.Name = "test-service-fork"
			s.Created = time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
			s.Endpoint = &api.Endpoint{
				Host: new("u8me885b93.test-project-id.tsdb.cloud.timescale.com"),
				Port: new(5432),
			}
		}}, overrides...)...)
	}

	expectFork := func(req api.ForkServiceCreate, overrides ...func(*api.Service)) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := forkedService(overrides...)
			m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", req).
				Return(&api.ForkServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}

	baseDetail := map[string]any{
		"id":                "u8me885b93",
		"name":              "test-service-fork",
		"status":            "READY",
		"type":              "TIMESCALEDB",
		"region":            "us-east-1",
		"created":           "2025-01-15T10:30:00Z",
		"environment":       "DEV",
		"replicas":          float64(0),
		"direct_endpoint":   "u8me885b93.test-project-id.tsdb.cloud.timescale.com:5432",
		"connection_string": "postgresql://tsdbadmin@u8me885b93.test-project-id.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require",
	}
	detail := func(overrides map[string]any) map[string]any {
		d := maps.Clone(baseDetail)
		maps.Copy(d, overrides)
		return d
	}

	const acceptedMsg = "Service fork request accepted. The forked service may still be provisioning."
	accepted := map[string]any{"service": baseDetail, "message": acceptedMsg}

	runToolTests(t, []toolTest{
		{
			name:      "not logged in",
			tool:      toolServiceFork,
			args:      args,
			clientErr: errNotLoggedIn,
			wantErr:   errNotLoggedIn.Error(),
		},
		{
			name:    "fork strategy is required",
			tool:    toolServiceFork,
			args:    map[string]any{"service_id": "e6ue9697jf"},
			wantErr: `validating "arguments": validating root: required: missing properties: ["fork_strategy"]`,
		},
		{
			name:    "unknown fork strategy",
			tool:    toolServiceFork,
			args:    map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "YESTERDAY"},
			wantErr: "validating \"arguments\": validating root: validating /properties/fork_strategy: enum: YESTERDAY does not equal any of: [NOW LAST_SNAPSHOT PITR]",
		},
		{
			// The schema's enum rejects a bad combination before the handler's
			// own ParseCPUMemory ever sees it.
			name: "invalid cpu/memory combination",
			tool: toolServiceFork,
			args: map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "NOW", "cpu_memory": "999 CPU/1 GB"},
			wantErr: "validating \"arguments\": validating root: validating /properties/cpu_memory: enum: 999 CPU/1 GB does not equal any of: " +
				"[shared/shared 0.5 CPU/2 GB 1 CPU/4 GB 2 CPU/8 GB 4 CPU/16 GB 8 CPU/32 GB 16 CPU/64 GB 32 CPU/128 GB]",
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:             "read-only all refuses without an API call",
			tool:             toolServiceFork,
			args:             args,
			configAfterStart: map[string]any{"read_only": "all"},
			wantErr:          "this operation is not allowed in read-only mode",
		},
		{
			// prod gates on the tag the fork is about to request, not the
			// source's, and needs no API call to decide.
			name:    "read-only prod refuses a PROD fork",
			tool:    toolServiceFork,
			args:    map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "NOW", "environment": "PROD"},
			config:  map[string]any{"read_only": "prod"},
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows the DEV default",
			tool:       toolServiceFork,
			args:       args,
			config:     map[string]any{"read_only": "prod"},
			setupMock:  expectFork(baseReq),
			wantOutput: accepted,
		},
		{
			name:    "PITR without a target time",
			tool:    toolServiceFork,
			args:    map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "PITR"},
			wantErr: "target_time is required when fork_strategy is 'PITR'",
		},
		{
			name:    "target time without PITR",
			tool:    toolServiceFork,
			args:    map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "NOW", "target_time": "2025-01-15T10:30:00Z"},
			wantErr: "target_time cannot be specified when fork_strategy is not 'PITR'",
		},
		{
			name: "network error",
			tool: toolServiceFork,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", baseReq).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fork service: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceFork,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", baseReq).
					Return(&api.ForkServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "nil response body",
			tool: toolServiceFork,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().ForkServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", baseReq).
					Return(&api.ForkServiceResponse{HTTPResponse: httpResponse(http.StatusAccepted)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "forks at the current state",
			tool:       toolServiceFork,
			args:       args,
			setupMock:  expectFork(baseReq),
			wantOutput: accepted,
		},
		{
			name: "forks from the last snapshot",
			tool: toolServiceFork,
			args: map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "LAST_SNAPSHOT"},
			setupMock: expectFork(api.ForkServiceCreate{
				ForkStrategy:   api.ForkStrategyLASTSNAPSHOT,
				EnvironmentTag: new(api.EnvironmentTagDEV),
			}),
			wantOutput: accepted,
		},
		{
			name: "forks to a point in time",
			tool: toolServiceFork,
			args: map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "PITR", "target_time": "2025-01-15T10:30:00Z"},
			setupMock: expectFork(api.ForkServiceCreate{
				ForkStrategy:   api.ForkStrategyPITR,
				TargetTime:     new(time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)),
				EnvironmentTag: new(api.EnvironmentTagDEV),
			}),
			wantOutput: accepted,
		},
		{
			name: "name, cpu/memory and PROD environment reach the request",
			tool: toolServiceFork,
			args: map[string]any{
				"service_id":    "e6ue9697jf",
				"fork_strategy": "NOW",
				"name":          "my-forked-db",
				"cpu_memory":    "1 CPU/4 GB",
				"environment":   "PROD",
			},
			setupMock: expectFork(api.ForkServiceCreate{
				ForkStrategy:   api.ForkStrategyNOW,
				Name:           new("my-forked-db"),
				CPUMillis:      new("1000"),
				MemoryGbs:      new("4"),
				EnvironmentTag: new(api.EnvironmentTagPROD),
			}, func(s *api.Service) {
				s.Name = "my-forked-db"
				s.Metadata = &api.ServiceMetadata{Environment: new("PROD")}
				s.Resources = []api.Resource{{Spec: &api.ResourceSpec{CPUMillis: new(1000), MemoryGbs: new(4)}}}
			}),
			wantOutput: map[string]any{
				"service": detail(map[string]any{
					"name":        "my-forked-db",
					"environment": "PROD",
					"resources":   map[string]any{"cpu": "1 cores", "memory": "4 GB"},
				}),
				"message": acceptedMsg,
			},
		},
		{
			name:      "stores the initial password",
			tool:      toolServiceFork,
			args:      args,
			setupMock: expectFork(baseReq, func(s *api.Service) { s.InitialPassword = new("fork-pass-123") }),
			wantOutput: map[string]any{
				"service": baseDetail,
				"message": acceptedMsg,
				"password_storage": map[string]any{
					"success": true,
					"method":  "keyring",
					"message": "Password saved to system keyring",
				},
			},
		},
		{
			name:      "with_password returns the password and embeds it in the connection string",
			tool:      toolServiceFork,
			args:      map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "NOW", "with_password": true},
			setupMock: expectFork(baseReq, func(s *api.Service) { s.InitialPassword = new("fork-pass-123") }),
			wantOutput: map[string]any{
				"service": detail(map[string]any{
					"password":          "fork-pass-123",
					"connection_string": "postgresql://tsdbadmin:fork-pass-123@u8me885b93.test-project-id.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require",
				}),
				"message": acceptedMsg,
				"password_storage": map[string]any{
					"success": true,
					"method":  "keyring",
					"message": "Password saved to system keyring",
				},
			},
		},
		{
			name:     "waits for the fork to be ready",
			tool:     toolServiceFork,
			args:     map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "NOW", "wait": true},
			synctest: true,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectFork(baseReq, func(s *api.Service) { s.Status = api.DeployStatusCONFIGURING })(m)
				ready := forkedService()
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "u8me885b93").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &ready,
					}, nil)
			},
			wantOutput: map[string]any{"service": baseDetail, "message": "Forked service is ready."},
		},
		{
			// A failed wait is reported in the message rather than as a tool
			// error: the fork itself was accepted.
			name:     "wait failure is reported in the message",
			tool:     toolServiceFork,
			args:     map[string]any{"service_id": "e6ue9697jf", "fork_strategy": "NOW", "wait": true},
			synctest: true,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectFork(baseReq, func(s *api.Service) { s.Status = api.DeployStatusCONFIGURING })(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "u8me885b93").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusNotFound)}, nil)
			},
			wantOutput: map[string]any{
				"service": detail(map[string]any{"status": "CONFIGURING"}),
				"message": "Error: service not found",
			},
		},
	})
}
