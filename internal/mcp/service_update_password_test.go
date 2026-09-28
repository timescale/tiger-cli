package mcp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

// withGenerateSecurePassword stubs common.GenerateSecurePassword, asserting the
// requested length and returning password and err in place of a random
// password, so a case can expect the exact update request and output.
func withGenerateSecurePassword(wantLength int, password string, err error) runOption {
	return withSetup(func(t *testing.T) {
		original := common.GenerateSecurePassword
		common.GenerateSecurePassword = func(length int) (string, error) {
			t.Helper()
			if length != wantLength {
				t.Errorf("GenerateSecurePassword length = %d, want %d", length, wantLength)
			}
			return password, err
		}
		t.Cleanup(func() { common.GenerateSecurePassword = original })
	})
}

func TestServiceUpdatePasswordTool(t *testing.T) {
	const password = "Xq3vN8sLr2KpW7mZt5YcB1dHf6JgA9eU"
	args := map[string]any{"service_id": "e6ue9697jf"}
	withPasswordArgs := map[string]any{"service_id": "e6ue9697jf", "with_password": true}
	generated := withGenerateSecurePassword(32, password, nil)

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
	expectTaggedServiceAndUpdate := func(tag string, calls int) func(*mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			expectTaggedService(tag, calls)(m)
			expectUpdate(http.StatusOK)(m)
		}
	}

	// The keyring is the default storage and is in-memory for tests, so the
	// save succeeds unless a case makes it fail.
	savedToKeyring := map[string]any{
		"success": true,
		"method":  "keyring",
		"message": "Password saved to system keyring",
	}
	updated := map[string]any{
		"updated":          true,
		"message":          "Password updated for tsdbadmin.",
		"password_storage": savedToKeyring,
	}
	cancelled := map[string]any{
		"updated": false,
		"message": `Password update cancelled: the user did not confirm updating the password of PROD service "e6ue9697jf" by typing its ID.`,
	}
	const noElicitationMsg = "updating the password of service e6ue9697jf requires the user's confirmation because it is tagged PROD, " +
		"but this MCP client does not support elicitation; ask the user to run 'tiger service update-password e6ue9697jf' instead"

	// The prompt raised for a PROD service, as the client receives it after
	// the JSON round trip (which is where the SDK fills in the mode).
	prompt := &mcp.ElicitParams{
		Mode:    "form",
		Message: `Update the password of PRODUCTION service "test-service" (e6ue9697jf)? The tsdbadmin password is replaced with a newly generated one: existing connections may be terminated, and applications using the old password will fail to authenticate.`,
		RequestedSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"service_id": map[string]any{
					"type":        "string",
					"title":       "Service ID",
					"description": "Type the service ID e6ue9697jf to confirm",
				},
			},
			"required": []any{"service_id"},
		},
	}
	// confirm answers the prompt by typing id back.
	confirm := func(id string) runOption {
		return withElicitation(&mcp.ElicitResult{Action: "accept", Content: map[string]any{"service_id": id}})
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
			args:    map[string]any{"service_id": "not-an-id"},
			wantErr: `validating "arguments": validating root: validating /properties/service_id: pattern: "not-an-id" does not match regular expression "^[a-z0-9]{10}$"`,
		},
		{
			// The tool generates the password itself, so an agent still
			// passing one is told so rather than having it silently ignored.
			name:    "password argument rejected",
			tool:    toolServiceUpdatePassword,
			args:    map[string]any{"service_id": "e6ue9697jf", "password": "MySecurePassword123!"},
			wantErr: `validating "arguments": validating root: unexpected additional properties ["password"]`,
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
			name: "disabled password storage refuses without with_password before any API call",
			tool: toolServiceUpdatePassword,
			args: args,
			opts: []runOption{withConfig(map[string]any{"password_storage": "none"})},
			wantErr: "password storage is disabled (password_storage=none), so the generated password would be lost; " +
				"set with_password to true to include it in the result, or ask the user to run 'tiger service update-password e6ue9697jf' to set a specific password",
		},
		{
			name: "service lookup network error",
			tool: toolServiceUpdatePassword,
			args: args,
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
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
			opts:       []runOption{withConfig(map[string]any{"read_only": "prod"}), generated},
			mock:       expectTaggedServiceAndUpdate("DEV", 1),
			wantOutput: updated,
		},
		{
			// Refused before the prompt, so the user isn't asked to confirm an
			// update that can't happen.
			name: "read replica names its primary",
			tool: toolServiceUpdatePassword,
			args: args,
			opts: []runOption{confirm("e6ue9697jf")},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) {
					s.Metadata = &api.ServiceMetadata{Environment: new("PROD")}
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
			opts: []runOption{generated},
			mock: expectGetServiceAndUpdate(sampleService(func(s *api.Service) {
				s.ForkedFrom = &api.ForkSpec{ServiceID: new("u8me885b93")}
			}), http.StatusOK),
			wantOutput: updated,
		},
		{
			// The client here can't prompt, which proves DEV updates never try.
			name:       "updates DEV service without prompting",
			tool:       toolServiceUpdatePassword,
			args:       args,
			opts:       []runOption{generated},
			mock:       expectTaggedServiceAndUpdate("DEV", 1),
			wantOutput: updated,
		},
		{
			// The handler runs twice — once to raise the prompt, once with the
			// answer — and fetches the service on both runs.
			name:       "updates PROD service once the user confirms",
			tool:       toolServiceUpdatePassword,
			args:       args,
			opts:       []runOption{confirm("e6ue9697jf"), generated},
			mock:       expectTaggedServiceAndUpdate("PROD", 2),
			wantPrompt: prompt,
			wantOutput: updated,
		},
		{
			name:       "PROD update cancelled when the typed ID does not match",
			tool:       toolServiceUpdatePassword,
			args:       args,
			opts:       []runOption{confirm("u8me885b93")},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD update cancelled when the user declines",
			tool:       toolServiceUpdatePassword,
			args:       args,
			opts:       []runOption{withElicitation(&mcp.ElicitResult{Action: "decline"})},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:       "PROD update cancelled when the user dismisses the prompt",
			tool:       toolServiceUpdatePassword,
			args:       args,
			opts:       []runOption{withElicitation(&mcp.ElicitResult{Action: "cancel"})},
			mock:       expectTaggedService("PROD", 2),
			wantPrompt: prompt,
			wantOutput: cancelled,
		},
		{
			name:    "PROD update refused when the client lacks elicitation",
			tool:    toolServiceUpdatePassword,
			args:    args,
			mock:    expectTaggedService("PROD", 1),
			wantErr: noElicitationMsg,
		},
		{
			name: "PROD update refused when the client supports only url elicitation",
			tool: toolServiceUpdatePassword,
			args: args,
			opts: []runOption{withClientCapabilities(&mcp.ClientCapabilities{
				Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}},
			})},
			mock:    expectTaggedService("PROD", 1),
			wantErr: noElicitationMsg,
		},
		{
			name:    "password generation fails",
			tool:    toolServiceUpdatePassword,
			args:    args,
			opts:    []runOption{withGenerateSecurePassword(32, "", errors.New("failed to generate random password: entropy unavailable"))},
			mock:    expectTaggedService("DEV", 1),
			wantErr: "failed to generate random password: entropy unavailable",
		},
		{
			name: "update network error",
			tool: toolServiceUpdatePassword,
			args: args,
			opts: []runOption{generated},
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
			opts: []runOption{generated},
			mock: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "e6ue9697jf", sampleService())
				m.EXPECT().UpdatePasswordWithResponse(validCtx, testProjectID, "e6ue9697jf", api.UpdatePasswordInput{Password: password}).
					Return(&api.UpdatePasswordResponse{
						HTTPResponse: httpResponse(http.StatusInternalServerError),
					}, nil)
			},
			wantErr: "unknown error",
		},
		{
			name:       "updates password on a 204",
			tool:       toolServiceUpdatePassword,
			args:       args,
			opts:       []runOption{generated},
			mock:       expectGetServiceAndUpdate(sampleService(), http.StatusNoContent),
			wantOutput: updated,
		},
		{
			name: "with_password includes the generated password",
			tool: toolServiceUpdatePassword,
			args: withPasswordArgs,
			opts: []runOption{generated},
			mock: expectGetServiceAndUpdate(sampleService(), http.StatusOK),
			wantOutput: map[string]any{
				"updated":          true,
				"message":          "Password updated for tsdbadmin.",
				"password":         password,
				"password_storage": savedToKeyring,
			},
		},
		{
			name: "with_password allows disabled password storage",
			tool: toolServiceUpdatePassword,
			args: withPasswordArgs,
			opts: []runOption{withConfig(map[string]any{"password_storage": "none"}), generated},
			mock: expectGetServiceAndUpdate(sampleService(), http.StatusOK),
			wantOutput: map[string]any{
				"updated":  true,
				"message":  "Password updated for tsdbadmin.",
				"password": password,
				"password_storage": map[string]any{
					"success": false,
					"method":  "none",
					"message": "Password not saved (--password-storage=none). Make sure to store it securely.",
				},
			},
		},
		{
			// A storage failure isn't fatal: the password is changed either
			// way, so the tool reports the failure rather than erroring. The
			// pgpass save fails on sampleService's missing endpoint.
			name: "reports a password storage failure",
			tool: toolServiceUpdatePassword,
			args: args,
			opts: []runOption{withConfig(map[string]any{"password_storage": "pgpass"}), generated},
			mock: expectGetServiceAndUpdate(sampleService(), http.StatusOK),
			wantOutput: map[string]any{
				"updated": true,
				"message": "Password updated for tsdbadmin. Warning: the new password could not be saved, and was not returned because with_password was false.",
				"password_storage": map[string]any{
					"success": false,
					"method":  "pgpass",
					"message": "Failed to save password to ~/.pgpass: service endpoint not available",
				},
			},
		},
	})
}
