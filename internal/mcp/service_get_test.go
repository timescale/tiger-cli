package mcp

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceGet(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}
	argsWithPassword := map[string]any{"service_id": "e6ue9697jf", "with_password": true}

	created := time.Date(2025, 1, 15, 9, 30, 0, 0, time.UTC)
	endpoint := &api.Endpoint{
		Host: new("e6ue9697jf.project.tsdb.cloud.timescale.com"),
		Port: new(5432),
	}

	// A fully populated service, so one success case covers every optional
	// field: resources, HA replicas, and both endpoints.
	fullService := sampleService(func(s *api.Service) {
		s.Created = created
		s.Metadata = &api.ServiceMetadata{Environment: new("PROD")}
		s.Resources = []api.Resource{{Spec: &api.ResourceSpec{CPUMillis: new(2000), MemoryGbs: new(8)}}}
		s.HaReplicas = &api.HAReplica{ReplicaCount: new(2)}
		s.Endpoint = endpoint
		s.ConnectionPooler = &api.ConnectionPooler{
			Endpoint: &api.Endpoint{
				Host: new("e6ue9697jf.project.pooler.tsdb.cloud.timescale.com"),
				Port: new(6432),
			},
		}
	})
	// The keyring is empty in tests, so initial_password is the only password
	// a service can supply.
	passwordService := sampleService(func(s *api.Service) {
		s.Created = created
		s.Endpoint = endpoint
		s.InitialPassword = new("super-secret-pw")
	})
	noPasswordService := sampleService(func(s *api.Service) {
		s.Created = created
		s.Endpoint = endpoint
	})

	setupGet := func(svc api.Service) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "e6ue9697jf", svc)
		}
	}

	full := map[string]any{
		"service": map[string]any{
			"id":                "e6ue9697jf",
			"name":              "test-service",
			"status":            "READY",
			"type":              "TIMESCALEDB",
			"region":            "us-east-1",
			"created":           "2025-01-15T09:30:00Z",
			"environment":       "PROD",
			"resources":         map[string]any{"cpu": "2 cores", "memory": "8 GB"},
			"replicas":          float64(2),
			"direct_endpoint":   "e6ue9697jf.project.tsdb.cloud.timescale.com:5432",
			"pooler_endpoint":   "e6ue9697jf.project.pooler.tsdb.cloud.timescale.com:6432",
			"connection_string": "postgresql://tsdbadmin@e6ue9697jf.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require",
		},
	}
	withPassword := map[string]any{
		"service": map[string]any{
			"id":                "e6ue9697jf",
			"name":              "test-service",
			"status":            "READY",
			"type":              "TIMESCALEDB",
			"region":            "us-east-1",
			"created":           "2025-01-15T09:30:00Z",
			"environment":       "DEV",
			"replicas":          float64(0),
			"direct_endpoint":   "e6ue9697jf.project.tsdb.cloud.timescale.com:5432",
			"password":          "super-secret-pw",
			"connection_string": "postgresql://tsdbadmin:super-secret-pw@e6ue9697jf.project.tsdb.cloud.timescale.com:5432/tsdb?sslmode=require",
		},
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceGet,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// The service_id pattern is enforced by the SDK's schema validation,
			// before the handler runs, so no API call is registered.
			name:    "service id fails schema validation",
			tool:    toolServiceGet,
			args:    map[string]any{"service_id": "not-an-id"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "not-an-id" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name: "network error",
			tool: toolServiceGet,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get service details: connection refused",
		},
		{
			name: "API error",
			tool: toolServiceGet,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "nil response body",
			tool: toolServiceGet,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:       "gets service without password",
			tool:       toolServiceGet,
			args:       args,
			mock:       setupGet(fullService),
			wantOutput: full,
		},
		{
			name:       "gets service with password",
			tool:       toolServiceGet,
			args:       argsWithPassword,
			mock:       setupGet(passwordService),
			wantOutput: withPassword,
		},
		{
			name:    "password requested but unavailable",
			tool:    toolServiceGet,
			args:    argsWithPassword,
			mock:    setupGet(noPasswordService),
			wantErr: "requested password but password not available",
		},
	})
}
