package mcp

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

// withFetchExtensions replaces common.FetchExtensions so tests run without a
// real database. It checks what it is called with and returns the given result.
func withFetchExtensions(wantTarget *common.ConnectionTarget, wantRole string, extensions []common.Extension, err error) runOption {
	return withSetup(func(t *testing.T) {
		original := common.FetchExtensions
		common.FetchExtensions = func(_ context.Context, _ *config.Config, gotTarget *common.ConnectionTarget, gotRole string) ([]common.Extension, error) {
			t.Helper()
			if diff := cmp.Diff(wantTarget, gotTarget); diff != "" {
				t.Errorf("FetchExtensions target mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(wantRole, gotRole); diff != "" {
				t.Errorf("FetchExtensions role mismatch (-want +got):\n%s", diff)
			}
			return extensions, err
		}
		t.Cleanup(func() { common.FetchExtensions = original })
	})
}

func TestDBExtensionsListTool(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf"}

	setupGetWithStatus := func(status api.DeployStatus) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) { s.Status = status }))
		}
	}

	extensions := []common.Extension{
		{Name: "timescaledb", Version: "2.21.0", Schema: "public", Description: "Enables scalable inserts and complex queries for time-series data"},
		{Name: "vector", Version: "0.8.0", Schema: "public"},
	}
	extensionsOutput := []any{
		map[string]any{"name": "timescaledb", "version": "2.21.0", "schema": "public", "description": "Enables scalable inserts and complex queries for time-series data"},
		map[string]any{"name": "vector", "version": "0.8.0", "schema": "public"},
	}

	primary := sampleService()
	primaryTarget := common.NewServiceConnectionTarget(primary)
	replica := sampleReplica(func(s *api.Service) { s.Status = api.DeployStatusREADY })
	// A replica connects to its own endpoint with its primary's credentials.
	replicaTarget := common.NewServiceConnectionTarget(replica)
	replicaTarget.Kind = common.TargetReadReplica
	replicaTarget.Credentials = common.ServiceCredentials(primary)

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolDBExtensionsList,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			// Rejected before the tool runs, so no API call is made.
			name:    "service id fails schema validation",
			tool:    toolDBExtensionsList,
			args:    map[string]any{"service_id": "not-an-id"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "not-an-id" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name: "service lookup network error",
			tool: toolDBExtensionsList,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "service lookup API error",
			tool: toolDBExtensionsList,
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
			tool: toolDBExtensionsList,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "read replica parent lookup fails",
			tool: toolDBExtensionsList,
			args: map[string]any{"service_id": "u8me885b93"},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "u8me885b93", replica)
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: `failed to fetch parent service "e6ue9697jf" for read replica: service not found`,
		},
		{
			name:    "service paused",
			tool:    toolDBExtensionsList,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusPAUSED),
			wantErr: pausedMsg,
		},
		{
			name:    "service not ready",
			tool:    toolDBExtensionsList,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusQUEUED),
			wantErr: notReadyMsg,
		},
		{
			name:    "ready service without an endpoint",
			tool:    toolDBExtensionsList,
			args:    args,
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			wantErr: noEndpointMsg,
		},
		{
			name:    "fetch fails",
			tool:    toolDBExtensionsList,
			args:    args,
			opts:    []runOption{withFetchExtensions(primaryTarget, "tsdbadmin", nil, errors.New("failed to connect to database: no route to host"))},
			mock:    setupGetWithStatus(api.DeployStatusREADY),
			wantErr: "failed to connect to database: no route to host",
		},
		{
			name:       "returns the extensions with the parameter defaults",
			tool:       toolDBExtensionsList,
			args:       args,
			opts:       []runOption{withFetchExtensions(primaryTarget, "tsdbadmin", extensions, nil)},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: map[string]any{"extensions": extensionsOutput},
		},
		{
			name:       "returns an empty list",
			tool:       toolDBExtensionsList,
			args:       args,
			opts:       []runOption{withFetchExtensions(primaryTarget, "tsdbadmin", []common.Extension{}, nil)},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: map[string]any{"extensions": []any{}},
		},
		{
			name:       "passes the role down to the fetch",
			tool:       toolDBExtensionsList,
			args:       map[string]any{"service_id": "e6ue9697jf", "role": "readonly"},
			opts:       []runOption{withFetchExtensions(primaryTarget, "readonly", extensions, nil)},
			mock:       setupGetWithStatus(api.DeployStatusREADY),
			wantOutput: map[string]any{"extensions": extensionsOutput},
		},
		{
			name: "returns the extensions of a read replica",
			tool: toolDBExtensionsList,
			args: map[string]any{"service_id": "u8me885b93"},
			opts: []runOption{withFetchExtensions(replicaTarget, "tsdbadmin", extensions, nil)},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "u8me885b93", replica)
				expectGetService(m, "e6ue9697jf", primary)
			},
			wantOutput: map[string]any{"extensions": extensionsOutput},
		},
	})
}
