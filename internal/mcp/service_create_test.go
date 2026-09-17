package mcp

import (
	"errors"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceCreate(t *testing.T) {
	args := map[string]any{"name": "test-service"}

	// The request built from a name alone: the SDK applies the schema's
	// replicas and environment defaults before the handler runs.
	baseReq := api.ServiceCreate{
		Name:           "test-service",
		ReplicaCount:   new(0),
		EnvironmentTag: new(api.EnvironmentTagDEV),
	}

	newService := sampleService(func(s *api.Service) {
		s.Created = time.Date(2025, 1, 15, 9, 30, 0, 0, time.UTC)
		s.Endpoint = &api.Endpoint{Host: new("e6ue9697jf.abc.tsdb.cloud.timescale.com"), Port: new(5432)}
	})
	provisioning := func(s *api.Service) { s.Status = api.DeployStatusCONFIGURING }
	withPassword := func(s *api.Service) { s.InitialPassword = new("init-pass-123") }

	expectCreate := func(req api.ServiceCreate, overrides ...func(*api.Service)) func(*mocks.MockClientWithResponsesInterface) {
		svc := newService
		for _, override := range overrides {
			override(&svc)
		}
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, req).
				Return(&api.CreateServiceResponse{
					HTTPResponse: httpResponse(http.StatusAccepted),
					JSON202:      &svc,
				}, nil)
		}
	}

	// expectPoll registers the wait loop's single status check, reporting the
	// service ready.
	expectPoll := func(m *mocks.MockClientWithResponsesInterface) {
		ready := newService
		m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
			Return(&api.GetServiceResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &ready,
			}, nil)
	}

	const (
		acceptedMsg = "Service creation request accepted. The service may still be provisioning."
		readyMsg    = "Service is ready."
		connString  = "postgresql://tsdbadmin@e6ue9697jf.abc.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require"
	)

	detail := map[string]any{
		"id":                "e6ue9697jf",
		"name":              "test-service",
		"status":            "READY",
		"type":              "TIMESCALEDB",
		"region":            "us-east-1",
		"created":           "2025-01-15T09:30:00Z",
		"environment":       "DEV",
		"direct_endpoint":   "e6ue9697jf.abc.tsdb.cloud.timescale.com:5432",
		"replicas":          float64(0),
		"connection_string": connString,
	}
	output := func(message string, mutate ...func(service, out map[string]any)) map[string]any {
		service := maps.Clone(detail)
		out := map[string]any{"service": service, "message": message}
		for _, m := range mutate {
			m(service, out)
		}
		return out
	}
	stillProvisioning := func(service, _ map[string]any) { service["status"] = "CONFIGURING" }
	storedPassword := func(_, out map[string]any) {
		out["password_storage"] = map[string]any{
			"success": true,
			"method":  "keyring",
			"message": "Password saved to system keyring",
		}
	}

	runToolTests(t, []toolTest{
		{
			name:      "not logged in",
			tool:      toolServiceCreate,
			args:      args,
			clientErr: errNotLoggedIn,
			wantErr:   errNotLoggedIn.Error(),
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
			name:             "read-only all refuses without an API call",
			tool:             toolServiceCreate,
			args:             args,
			configAfterStart: map[string]any{"read_only": "all"},
			wantErr:          "this operation is not allowed in read-only mode",
		},
		{
			// prod gates on the tag being requested rather than on any existing
			// service, so no service is fetched.
			name:    "read-only prod refuses a requested PROD environment",
			tool:    toolServiceCreate,
			args:    map[string]any{"name": "test-service", "environment": "PROD"},
			config:  map[string]any{"read_only": "prod"},
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows the DEV default",
			tool:       toolServiceCreate,
			args:       args,
			config:     map[string]any{"read_only": "prod"},
			setupMock:  expectCreate(baseReq),
			wantOutput: output(acceptedMsg),
		},
		{
			name: "network error",
			tool: toolServiceCreate,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, baseReq).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to create service: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceCreate,
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
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
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
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
			setupMock:  expectCreate(baseReq),
			wantOutput: output(acceptedMsg),
		},
		{
			// The generated name is random, so the request is matched on every
			// other field; the mocked response is the usual sample service.
			name: "auto-generates a name when none is given",
			tool: toolServiceCreate,
			args: map[string]any{},
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				generatedName := gomock.Cond(func(req api.ServiceCreate) bool {
					want := baseReq
					want.Name = req.Name
					return req.Name != "" && req.Name != "test-service" && cmp.Equal(want, req)
				})
				m.EXPECT().CreateServiceWithResponse(validCtx, testProjectID, generatedName).
					Return(&api.CreateServiceResponse{
						HTTPResponse: httpResponse(http.StatusAccepted),
						JSON202:      &newService,
					}, nil)
			},
			wantOutput: output(acceptedMsg),
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
			setupMock: expectCreate(api.ServiceCreate{
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
			wantOutput: output(acceptedMsg, func(service, _ map[string]any) {
				service["region"] = "us-west-2"
				service["replicas"] = float64(2)
				service["resources"] = map[string]any{"cpu": "2 cores", "memory": "8 GB"}
			}),
		},
		{
			name:       "stores the initial password without returning it",
			tool:       toolServiceCreate,
			args:       args,
			setupMock:  expectCreate(baseReq, withPassword),
			wantOutput: output(acceptedMsg, storedPassword),
		},
		{
			name:      "with_password returns the password and embeds it in the connection string",
			tool:      toolServiceCreate,
			args:      map[string]any{"name": "test-service", "with_password": true},
			setupMock: expectCreate(baseReq, withPassword),
			wantOutput: output(acceptedMsg, storedPassword, func(service, _ map[string]any) {
				service["password"] = "init-pass-123"
				service["connection_string"] = "postgresql://tsdbadmin:init-pass-123@e6ue9697jf.abc.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require"
			}),
		},
		{
			name:       "wait returns immediately when the service is already ready",
			tool:       toolServiceCreate,
			args:       map[string]any{"name": "test-service", "wait": true},
			setupMock:  expectCreate(baseReq),
			wantOutput: output(readyMsg),
		},
		{
			name:     "wait polls until the service is ready",
			tool:     toolServiceCreate,
			args:     map[string]any{"name": "test-service", "wait": true},
			synctest: true,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectCreate(baseReq, provisioning)(m)
				expectPoll(m)
			},
			wantOutput: output(readyMsg),
		},
		{
			// set_default defaults to true, so the plain create above already
			// stored one; this pins that it is the created service.
			name:       "sets the new service as the default",
			tool:       toolServiceCreate,
			args:       args,
			setupMock:  expectCreate(baseReq),
			wantOutput: output(acceptedMsg),
			checks:     []toolCheckFunc{checkDefaultService("e6ue9697jf")},
		},
		{
			name:       "set_default false leaves the default service unset",
			tool:       toolServiceCreate,
			args:       map[string]any{"name": "test-service", "set_default": false},
			setupMock:  expectCreate(baseReq),
			wantOutput: output(acceptedMsg),
			checks:     []toolCheckFunc{checkDefaultService("")},
		},
		{
			// A failed wait is reported in the message rather than as an error:
			// the service was created either way.
			name:     "wait reports a polling failure in the message",
			tool:     toolServiceCreate,
			args:     map[string]any{"name": "test-service", "wait": true},
			synctest: true,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectCreate(baseReq, provisioning)(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusNotFound)}, nil)
			},
			wantOutput: output("Error: service not found", stillProvisioning),
		},
		{
			name:     "wait times out while the service is still provisioning",
			tool:     toolServiceCreate,
			args:     map[string]any{"name": "test-service", "wait": true},
			synctest: true,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectCreate(baseReq, provisioning)(m)
				pending := newService
				provisioning(&pending)
				// AnyTimes because the loop polls once a second for the full
				// timeout: the count is timer-driven, not something asserted here.
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &pending,
					}, nil).
					AnyTimes()
			},
			wantOutput: output("Error: wait timeout reached after 10m0s - service may still be provisioning", stillProvisioning),
		},
	})
}
