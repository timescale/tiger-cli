package mcp

import (
	"net/http"
	"testing"
	"time"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceListTool(t *testing.T) {
	created := time.Date(2025, 1, 15, 9, 30, 0, 0, time.UTC)

	expectServices := func(services []api.Service) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
				Return(&api.GetServicesResponse{
					HTTPResponse: httpResponse(http.StatusOK),
					JSON200:      &services,
				}, nil)
		}
	}

	// One service per conversion branch: whole cores with an untagged (DEV)
	// service, fractional cores with a PROD tag, a free tier service whose null
	// CPU and memory read as "shared", and a service with no resources at all.
	services := []api.Service{
		sampleService(func(s *api.Service) {
			s.Created = created
			s.Resources = []api.Resource{{Spec: &api.ResourceSpec{CPUMillis: new(4000), MemoryGbs: new(16)}}}
		}),
		sampleService(func(s *api.Service) {
			s.ServiceID = "u8me885b93"
			s.Name = "prod-service"
			s.Created = created
			s.Metadata = &api.ServiceMetadata{Environment: new("PROD")}
			s.Resources = []api.Resource{{Spec: &api.ResourceSpec{CPUMillis: new(500), MemoryGbs: new(2)}}}
		}),
		sampleService(func(s *api.Service) {
			s.ServiceID = "fr33tier01"
			s.Name = "free-service"
			s.ServiceType = api.ServiceTypePOSTGRES
			s.Created = created
			s.Resources = []api.Resource{{Spec: &api.ResourceSpec{}}}
		}),
		sampleService(func(s *api.Service) {
			s.ServiceID = "pausedsvc1"
			s.Name = "paused-service"
			s.Status = api.DeployStatusPAUSED
			s.Created = created
		}),
	}

	listed := map[string]any{
		"services": []any{
			map[string]any{
				"id":          "e6ue9697jf",
				"name":        "test-service",
				"status":      "READY",
				"type":        "TIMESCALEDB",
				"region":      "us-east-1",
				"created":     "2025-01-15T09:30:00Z",
				"environment": "DEV",
				"resources":   map[string]any{"cpu": "4 cores", "memory": "16 GB"},
			},
			map[string]any{
				"id":          "u8me885b93",
				"name":        "prod-service",
				"status":      "READY",
				"type":        "TIMESCALEDB",
				"region":      "us-east-1",
				"created":     "2025-01-15T09:30:00Z",
				"environment": "PROD",
				"resources":   map[string]any{"cpu": "0.5 cores", "memory": "2 GB"},
			},
			map[string]any{
				"id":          "fr33tier01",
				"name":        "free-service",
				"status":      "READY",
				"type":        "POSTGRES",
				"region":      "us-east-1",
				"created":     "2025-01-15T09:30:00Z",
				"environment": "DEV",
				"resources":   map[string]any{"cpu": "shared", "memory": "shared"},
			},
			map[string]any{
				"id":          "pausedsvc1",
				"name":        "paused-service",
				"status":      "PAUSED",
				"type":        "TIMESCALEDB",
				"region":      "us-east-1",
				"created":     "2025-01-15T09:30:00Z",
				"environment": "DEV",
			},
		},
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceList,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name: "API error",
			tool: toolServiceList,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
					Return(&api.GetServicesResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSON4XX:      &api.ClientError{Message: new("insufficient permissions")},
					}, nil)
			},
			wantErr: "insufficient permissions",
		},
		{
			// A 200 with no body is an empty list, not an error.
			name: "nil response body lists nothing",
			tool: toolServiceList,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServicesWithResponse(validCtx, testProjectID).
					Return(&api.GetServicesResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantOutput: map[string]any{"services": []any{}},
		},
		{
			name:       "empty list",
			tool:       toolServiceList,
			mock:       expectServices([]api.Service{}),
			wantOutput: map[string]any{"services": []any{}},
		},
		{
			name:       "lists services",
			tool:       toolServiceList,
			mock:       expectServices(services),
			wantOutput: listed,
		},
	})
}
