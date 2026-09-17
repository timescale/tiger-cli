package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestDBSchema(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	// A read replica connects to its own endpoint but borrows the parent
	// primary's credentials, so resolving one fetches both services.
	replica := sampleService(func(s *api.Service) {
		s.ServiceID = "u8me885b93"
		s.Name = "replica-service"
		s.Status = api.DeployStatusPAUSED
		s.ForkedFrom = &api.ForkSpec{
			IsStandby: new(true),
			ProjectID: new(testProjectID),
			ServiceID: new("e6ue9697jf"),
		}
	})

	expectGet := func(id string, svc api.Service) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, id).
				Return(&api.GetServiceResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &svc,
				}, nil)
		}
	}
	expectStatus := func(status api.DeployStatus) func(*mocks.MockClientWithResponsesInterface) {
		return expectGet("e6ue9697jf", sampleService(func(s *api.Service) { s.Status = status }))
	}

	const (
		pausedErr   = "service is paused — start it with the service_start tool"
		notReadyErr = "service is not ready — check its status with service_get and try again"
		// sampleService carries no endpoint, so a READY service fails here
		// rather than opening a connection.
		noEndpointErr = "failed to build connection string: service endpoint not available"
	)

	runToolTests(t, []toolTest{
		{
			name:      "not logged in",
			tool:      "db_schema",
			args:      args,
			clientErr: errNotLoggedIn,
			wantErr:   errNotLoggedIn.Error(),
		},
		{
			// The service_id pattern is enforced by the SDK's schema validation,
			// before the handler runs, so no API call is registered.
			name:    "service id fails schema validation",
			tool:    "db_schema",
			args:    map[string]any{"service_id": "not-an-id"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "not-an-id" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name: "service lookup network error",
			tool: "db_schema",
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "service lookup API error",
			tool: "db_schema",
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
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
			tool: "db_schema",
			args: args,
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:      "service paused",
			tool:      "db_schema",
			args:      args,
			setupMock: expectStatus(api.DeployStatusPAUSED),
			wantErr:   pausedErr,
		},
		{
			name:      "service pausing",
			tool:      "db_schema",
			args:      args,
			setupMock: expectStatus(api.DeployStatusPAUSING),
			wantErr:   pausedErr,
		},
		{
			name:      "service not ready",
			tool:      "db_schema",
			args:      args,
			setupMock: expectStatus(api.DeployStatusQUEUED),
			wantErr:   notReadyErr,
		},
		{
			// Both services are fetched: the replica for its endpoint, the
			// parent for its credentials. The replica's own status then stops it.
			name: "read replica resolves its parent",
			tool: "db_schema",
			args: map[string]any{"service_id": "u8me885b93"},
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGet("u8me885b93", replica)(m)
				expectGet("e6ue9697jf", sampleService())(m)
			},
			wantErr: pausedErr,
		},
		{
			name: "read replica parent lookup fails",
			tool: "db_schema",
			args: map[string]any{"service_id": "u8me885b93"},
			setupMock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGet("u8me885b93", replica)(m)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: `failed to fetch parent service "e6ue9697jf" for read replica: service not found`,
		},
		{
			name:      "ready service without an endpoint",
			tool:      "db_schema",
			args:      args,
			setupMock: expectStatus(api.DeployStatusREADY),
			wantErr:   noEndpointErr,
		},
		{
			name: "pooled without a pooler",
			tool: "db_schema",
			args: map[string]any{"service_id": "e6ue9697jf", "pooled": true},
			setupMock: expectGet("e6ue9697jf", sampleService(func(s *api.Service) {
				s.Endpoint = &api.Endpoint{
					Host: new("e6ue9697jf.project.tsdb.cloud.timescale.com"),
					Port: new(5432),
				}
			})),
			wantErr: "connection pooler not available for this service",
		},
		{
			// The introspection options only shape the SQL issued after
			// connecting; all that's observable here is that they're accepted.
			name: "introspection options accepted",
			tool: "db_schema",
			args: map[string]any{
				"service_id":  "e6ue9697jf",
				"schema":      "public",
				"internal":    true,
				"definitions": true,
				"comments":    true,
				"role":        "readonly",
				"pooled":      false,
			},
			setupMock: expectStatus(api.DeployStatusPAUSED),
			wantErr:   pausedErr,
		},
	})
}
