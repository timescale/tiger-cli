package mcp

import (
	"errors"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

// withGenerateServiceName pins the otherwise random auto-generated service
// name, so the request that carries it can be asserted exactly.
func withGenerateServiceName(name string) runOption {
	return withSetup(func(t *testing.T) {
		original := common.GenerateServiceName
		common.GenerateServiceName = func() string { return name }
		t.Cleanup(func() { common.GenerateServiceName = original })
	})
}

func TestServiceCreateTool(t *testing.T) {
	args := map[string]any{"name": "test-service"}
	waitArgs := map[string]any{"name": "test-service", "wait": true}

	// The request built from a name alone: the SDK applies the schema's
	// replicas and environment defaults before the handler runs.
	baseReq := api.ServiceCreate{
		Name:           "test-service",
		ReplicaCount:   new(0),
		EnvironmentTag: new(api.EnvironmentTagDEV),
	}

	newService := func(overrides ...func(*api.Service)) api.Service {
		return sampleService(append([]func(*api.Service){func(s *api.Service) {
			s.Created = time.Date(2025, 1, 15, 9, 30, 0, 0, time.UTC)
			s.Endpoint = &api.Endpoint{Host: new("e6ue9697jf.abc.tsdb.cloud.timescale.com"), Port: new(5432)}
		}}, overrides...)...)
	}
	provisioning := func(s *api.Service) { s.Status = api.DeployStatusCONFIGURING }
	withInitialPassword := func(s *api.Service) { s.InitialPassword = new("init-pass-123") }

	expectCreate := func(req api.ServiceCreate, overrides ...func(*api.Service)) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			svc := newService(overrides...)
			m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, req).
				Return(&api.CreateServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}

	baseDetail := map[string]any{
		"id":                "e6ue9697jf",
		"name":              "test-service",
		"status":            "READY",
		"type":              "TIMESCALEDB",
		"region":            "us-east-1",
		"created":           "2025-01-15T09:30:00Z",
		"environment":       "DEV",
		"direct_endpoint":   "e6ue9697jf.abc.tsdb.cloud.timescale.com:5432",
		"replicas":          float64(0),
		"connection_string": "postgresql://tsdbadmin@e6ue9697jf.abc.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require",
	}
	detail := func(overrides map[string]any) map[string]any {
		d := maps.Clone(baseDetail)
		maps.Copy(d, overrides)
		return d
	}

	const (
		acceptedMsg = "Service creation request accepted. The service may still be provisioning."
		readyMsg    = "Service is ready."
	)
	accepted := map[string]any{"service": baseDetail, "message": acceptedMsg}
	ready := map[string]any{"service": baseDetail, "message": readyMsg}
	// The keyring is the default storage and is in-memory for tests, so the
	// save succeeds unless a case replaces it.
	storedPassword := map[string]any{
		"success": true,
		"method":  "keyring",
		"message": "Password saved to system keyring",
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceCreate,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "unknown environment rejected by the schema",
			tool:    toolServiceCreate,
			args:    map[string]any{"name": "test-service", "environment": "STAGING"},
			wantErr: `validating "arguments": validating root: validating /properties/environment: enum: STAGING does not equal any of: [DEV PROD]`,
		},
		{
			// The enum is generated from the same configs ParseCPUMemory
			// accepts, so an invalid value never reaches the handler.
			name: "unknown cpu_memory rejected by the schema",
			tool: toolServiceCreate,
			args: map[string]any{"name": "test-service", "cpu_memory": "3 CPU/8 GB"},
			wantErr: `validating "arguments": validating root: validating /properties/cpu_memory: enum: 3 CPU/8 GB does not equal any of: ` +
				`[shared/shared 0.5 CPU/2 GB 1 CPU/4 GB 2 CPU/8 GB 4 CPU/16 GB 8 CPU/32 GB 16 CPU/64 GB 32 CPU/128 GB]`,
		},
		{
			name:    "replica count above the maximum rejected by the schema",
			tool:    toolServiceCreate,
			args:    map[string]any{"name": "test-service", "replicas": 6},
			wantErr: `validating "arguments": validating root: validating /properties/replicas: maximum: 6/1 is greater than 5.000000`,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceCreate,
			args:    args,
			opts:    []runOption{withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// prod gates on the tag being requested rather than on any existing
			// service, so no service is fetched.
			name:    "read-only prod refuses a requested PROD environment",
			tool:    toolServiceCreate,
			args:    map[string]any{"name": "test-service", "environment": "PROD"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows the DEV default",
			tool:       toolServiceCreate,
			args:       args,
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectCreate(baseReq),
			wantOutput: accepted,
		},
		{
			name: "network error",
			tool: toolServiceCreate,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create service: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceCreate,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.ClientError{Message: new("service limit reached")},
					}, nil)
			},
			wantErr: "service limit reached",
		},
		{
			name: "nil response body",
			tool: toolServiceCreate,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "creates a service with the schema defaults",
			tool:       toolServiceCreate,
			args:       args,
			mock:       expectCreate(baseReq),
			wantOutput: accepted,
		},
		{
			name: "auto-generates a name when none is given",
			tool: toolServiceCreate,
			args: map[string]any{},
			opts: []runOption{withGenerateServiceName("db-42424")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				req := baseReq
				req.Name = "db-42424"
				expectCreate(req, func(s *api.Service) { s.Name = "db-42424" })(m)
			},
			wantOutput: map[string]any{
				"service": detail(map[string]any{"name": "db-42424"}),
				"message": acceptedMsg,
			},
		},
		{
			name: "passes region, addons, replicas and cpu_memory through",
			tool: toolServiceCreate,
			args: map[string]any{
				"name":       "test-service",
				"region":     "us-west-2",
				"addons":     []any{"time-series", "ai"},
				"replicas":   2,
				"cpu_memory": "2 CPU/8 GB",
			},
			mock: expectCreate(api.ServiceCreate{
				Name:           "test-service",
				Addons:         &[]api.ServiceCreateAddons{"time-series", "ai"},
				RegionCode:     new("us-west-2"),
				ReplicaCount:   new(2),
				CPUMillis:      new("2000"),
				MemoryGbs:      new("8"),
				EnvironmentTag: new(api.EnvironmentTagDEV),
			}, func(s *api.Service) {
				s.RegionCode = "us-west-2"
				s.HaReplicas = &api.HAReplica{ReplicaCount: new(2)}
				s.Resources = []api.Resource{{Spec: &api.ResourceSpec{CPUMillis: new(2000), MemoryGbs: new(8)}}}
			}),
			wantOutput: map[string]any{
				"service": detail(map[string]any{
					"region":    "us-west-2",
					"replicas":  float64(2),
					"resources": map[string]any{"cpu": "2 cores", "memory": "8 GB"},
				}),
				"message": acceptedMsg,
			},
		},
		{
			name: "stores the initial password without returning it",
			tool: toolServiceCreate,
			args: args,
			mock: expectCreate(baseReq, withInitialPassword),
			wantOutput: map[string]any{
				"service":          baseDetail,
				"message":          acceptedMsg,
				"password_storage": storedPassword,
			},
		},
		{
			name: "with_password returns the password and embeds it in the connection string",
			tool: toolServiceCreate,
			args: map[string]any{"name": "test-service", "with_password": true},
			mock: expectCreate(baseReq, withInitialPassword),
			wantOutput: map[string]any{
				"service": detail(map[string]any{
					"password":          "init-pass-123",
					"connection_string": "postgresql://tsdbadmin:init-pass-123@e6ue9697jf.abc.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require",
				}),
				"message":          acceptedMsg,
				"password_storage": storedPassword,
			},
		},
		{
			// A storage failure isn't fatal: the service exists either way, so
			// the tool reports the failure rather than erroring.
			name: "reports a password storage failure",
			tool: toolServiceCreate,
			args: args,
			opts: []runOption{withKeyringError(errors.New("keyring is locked"))},
			mock: expectCreate(baseReq, withInitialPassword),
			wantOutput: map[string]any{
				"service": baseDetail,
				"message": acceptedMsg,
				"password_storage": map[string]any{
					"success": false,
					"method":  "keyring",
					"message": "Failed to save password to keyring: keyring is locked",
				},
			},
		},
		{
			// set_default defaults to true, so the plain create above already
			// stored one; this pins that it is the created service.
			name:       "sets the new service as the default",
			tool:       toolServiceCreate,
			args:       args,
			mock:       expectCreate(baseReq),
			wantOutput: accepted,
			checks:     []checkFunc{checkDefaultService("e6ue9697jf")},
		},
		{
			name:       "set_default false leaves the default service unset",
			tool:       toolServiceCreate,
			args:       map[string]any{"name": "test-service", "set_default": false},
			mock:       expectCreate(baseReq),
			wantOutput: accepted,
			checks:     []checkFunc{checkDefaultService("")},
		},
		{
			// Already at the target status, so the wait returns without polling.
			name:       "wait returns immediately when the service is already ready",
			tool:       toolServiceCreate,
			args:       waitArgs,
			mock:       expectCreate(baseReq),
			wantOutput: ready,
		},
		{
			name:     "wait polls until the service is ready",
			synctest: true,
			tool:     toolServiceCreate,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectCreate(baseReq, provisioning)(m)
				expectGetService(m, "e6ue9697jf", newService())
			},
			wantOutput: ready,
		},
		{
			// A failed wait is reported in the message rather than as an error:
			// the service was created either way.
			name:     "wait reports a failed poll in the message",
			synctest: true,
			tool:     toolServiceCreate,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectCreate(baseReq, provisioning)(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusNotFound)}, nil)
			},
			wantOutput: map[string]any{
				"service": detail(map[string]any{"status": "CONFIGURING"}),
				"message": "Error: service not found",
			},
		},
		{
			// The full 10-minute timeout elapses instantly in the bubble.
			// AnyTimes because the loop polls once a second for the whole of
			// it: the count is timer-driven, not something the case asserts.
			name:     "wait reports a timeout in the message",
			synctest: true,
			tool:     toolServiceCreate,
			args:     waitArgs,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectCreate(baseReq, provisioning)(m)
				expectGetService(m, "e6ue9697jf", newService(provisioning)).AnyTimes()
			},
			wantOutput: map[string]any{
				"service": detail(map[string]any{"status": "CONFIGURING"}),
				"message": "Error: wait timeout reached after 10m0s - service may still be provisioning",
			},
		},
	})
}
