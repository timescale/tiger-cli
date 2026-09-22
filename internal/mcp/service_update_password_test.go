package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
)

func TestServiceUpdatePasswordTool(t *testing.T) {
	const password = "MySecurePassword123!"
	args := map[string]any{"service_id": "e6ue9697jf", "password": password}

	expectUpdate := func(status int) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().UpdatePasswordWithResponse(validCtx, testProjectID, "e6ue9697jf", api.UpdatePasswordInput{Password: password}).
				Return(&api.UpdatePasswordResponse{HTTPResponse: httpResponse(status)}, nil)
		}
	}
	expectGetServiceAndUpdate := func(svc api.Service, status int) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectGetService(m, "e6ue9697jf", svc)
			expectUpdate(status)(m)
		}
	}
	expectTaggedServiceAndUpdate := func(tag string, status int) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectTaggedService(tag, 1)(m)
			expectUpdate(status)(m)
		}
	}

	// The keyring is the default storage and is in-memory for tests, so the
	// save always succeeds.
	updated := map[string]any{
		"message": "Password updated for tsdbadmin",
		"password_storage": map[string]any{
			"success": true,
			"method":  "keyring",
			"message": "Password saved to system keyring",
		},
	}

	runToolTests(t, []toolTest{
		{
			name:    "not logged in",
			tool:    toolServiceUpdatePassword,
			args:    args,
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
		},
		{
			name:    "service ID failing the schema pattern",
			tool:    toolServiceUpdatePassword,
			args:    map[string]any{"service_id": "not-an-id", "password": password},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "not-an-id" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			name:    "missing password",
			tool:    toolServiceUpdatePassword,
			args:    map[string]any{"service_id": "e6ue9697jf"},
			wantErr: `validating "arguments": validating root: required: missing properties: ["password"]`,
		},
		{
			// The tool isn't registered under read_only=all at startup; this is
			// the handler's own up-front check catching a config change made
			// since, before any fetch.
			name:    "read-only all refuses without an API call",
			tool:    toolServiceUpdatePassword,
			args:    args,
			opts:    []runOption{withConfigAfterStart(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			name: "service lookup network error",
			tool: toolServiceUpdatePassword,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get service details: connection refused",
		},
		{
			name: "service lookup API error",
			tool: toolServiceUpdatePassword,
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
			name: "nil service lookup body",
			tool: toolServiceUpdatePassword,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(&api.GetServiceResponse{HTTPResponse: httpResponse(http.StatusOK)}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			// Only the fetch is registered: an attempted update fails as an
			// unexpected call.
			name:    "read-only prod refuses PROD service",
			tool:    toolServiceUpdatePassword,
			args:    args,
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name:       "read-only prod allows DEV service",
			tool:       toolServiceUpdatePassword,
			args:       args,
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"})},
			mock:       expectTaggedServiceAndUpdate("DEV", http.StatusOK),
			wantOutput: updated,
		},
		{
			name: "read replica names its primary",
			tool: toolServiceUpdatePassword,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) {
					s.ForkedFrom = &api.ForkSpec{ServiceID: new("u8me885b93"), IsStandby: new(true)}
				}))
			},
			wantErr: `"e6ue9697jf" is a read replica; update the password on its primary service "u8me885b93" instead`,
		},
		{
			// A fork that isn't a standby shares nothing, so it updates normally.
			name: "non-standby fork is updated",
			tool: toolServiceUpdatePassword,
			args: args,
			mock: expectGetServiceAndUpdate(sampleService(func(s *api.Service) {
				s.ForkedFrom = &api.ForkSpec{ServiceID: new("u8me885b93")}
			}), http.StatusOK),
			wantOutput: updated,
		},
		{
			name: "update network error",
			tool: toolServiceUpdatePassword,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", sampleService())
				m.EXPECT().UpdatePasswordWithResponse(validCtx, testProjectID, "e6ue9697jf", api.UpdatePasswordInput{Password: password}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to update service password: connection refused",
		},
		{
			name: "update API error",
			tool: toolServiceUpdatePassword,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", sampleService())
				m.EXPECT().UpdatePasswordWithResponse(validCtx, testProjectID, "e6ue9697jf", api.UpdatePasswordInput{Password: password}).
					Return(&api.UpdatePasswordResponse{
						HTTPResponse: httpResponse(http.StatusBadRequest),
						JSON4XX:      &api.ClientError{Message: new("password is too weak")},
					}, nil)
			},
			wantErr: "password is too weak",
		},
		{
			name:       "updates password on a 200",
			tool:       toolServiceUpdatePassword,
			args:       args,
			mock:       expectGetServiceAndUpdate(sampleService(), http.StatusOK),
			wantOutput: updated,
		},
		{
			name:       "updates password on a 204",
			tool:       toolServiceUpdatePassword,
			args:       args,
			mock:       expectGetServiceAndUpdate(sampleService(), http.StatusNoContent),
			wantOutput: updated,
		},
		{
			// A storage failure isn't fatal: the password is changed either
			// way, so the tool reports the failure rather than erroring. The
			// pgpass save fails on sampleService's missing endpoint.
			name: "reports a password storage failure",
			tool: toolServiceUpdatePassword,
			args: args,
			opts: []runOption{withConfig(map[string]any{"password_storage": "pgpass"})},
			mock: expectGetServiceAndUpdate(sampleService(), http.StatusOK),
			wantOutput: map[string]any{
				"message": "Password updated for tsdbadmin",
				"password_storage": map[string]any{
					"success": false,
					"method":  "pgpass",
					"message": "Failed to save password to ~/.pgpass: service endpoint not available",
				},
			},
		},
	})
}
