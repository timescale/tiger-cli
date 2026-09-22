package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceRename(t *testing.T) {
	args := map[string]any{"service_id": "e6ue9697jf", "name": "renamed-service"}

	renamed := sampleService(func(s *api.Service) { s.Name = "renamed-service" })

	expectRename := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().RenameServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceRename{Name: "renamed-service"}).
			Return(&api.RenameServiceResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &renamed,
			}, nil)
	}
	expectTaggedServiceAndRename := func(tag string) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectTaggedService(tag, 1)(m)
			expectRename(m)
		}
	}

	output := map[string]any{
		"message":    "Service renamed successfully.",
		"service_id": "e6ue9697jf",
		"name":       "renamed-service",
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceRename,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceRename,
			args:    map[string]any{"service_id": "not-an-id", "name": "renamed-service"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "not-an-id" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "missing name",
			tool:    toolServiceRename,
			args:    map[string]any{"service_id": "e6ue9697jf"},
			wantErr: `validating "arguments": validating root: required: missing properties: ["name"]`,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own check catching a config change made since.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceRename,
			args:    args,
			opts:    []runOption{withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// Only the tag lookup is registered: an attempted rename fails as an
			// unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceRename,
			args:    args,
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `service e6ue9697jf: this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			tool:       toolServiceRename,
			args:       args,
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectTaggedServiceAndRename("DEV"),
			wantOutput: output,
		},
		{
			name: "rename request fails",
			tool: toolServiceRename,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().RenameServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceRename{Name: "renamed-service"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to rename service: connection refused",
		},
		{
			name: "rename API error",
			tool: toolServiceRename,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().RenameServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceRename{Name: "renamed-service"}).
					Return(&api.RenameServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.ClientError{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
		},
		{
			name: "nil response body",
			tool: toolServiceRename,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().RenameServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceRename{Name: "renamed-service"}).
					Return(&api.RenameServiceResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// read_only is off by default, so the gate makes no API call and the
			// rename is the only request.
			name:       "renames service",
			tool:       toolServiceRename,
			args:       args,
			mock:       expectRename,
			wantOutput: output,
		},
		{
			// The name the API stored is echoed back, not the one requested.
			name: "reports the name the API stored",
			tool: toolServiceRename,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				stored := sampleService(func(s *api.Service) { s.Name = "renamed-service-1" })
				m.EXPECT().RenameServiceWithResponse(validCtx, testProjectID, "e6ue9697jf", api.ServiceRename{Name: "renamed-service"}).
					Return(&api.RenameServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &stored,
					}, nil)
			},
			wantOutput: map[string]any{
				"message":    "Service renamed successfully.",
				"service_id": "e6ue9697jf",
				"name":       "renamed-service-1",
			},
		},
	})
}
