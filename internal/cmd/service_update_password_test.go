package cmd

import (
	"errors"
	"net/http"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestServiceUpdatePasswordCmd(t *testing.T) {
	setupGet := func(m *mocks.MockClientWithResponsesInterface) {
		svc := sampleService()
		m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
			Return(&api.GetServiceResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &svc,
			}, nil)
	}

	setupUpdate := func(password string) func(m *mocks.MockClientWithResponsesInterface) {
		return func(m *mocks.MockClientWithResponsesInterface) {
			m.EXPECT().UpdatePasswordWithResponse(validCtx, testProjectID, "svc-12345", api.UpdatePasswordInput{Password: password}).
				Return(&api.UpdatePasswordResponse{
					HTTPResponse: httpResponse(http.StatusOK),
				}, nil)
		}
	}

	// setupUpdateAny is for the paths that generate a random password.
	setupUpdateAny := func(m *mocks.MockClientWithResponsesInterface) {
		m.EXPECT().UpdatePasswordWithResponse(validCtx, testProjectID, "svc-12345", gomock.Any()).
			Return(&api.UpdatePasswordResponse{
				HTTPResponse: httpResponse(http.StatusOK),
			}, nil)
	}

	// storedPassword reads the password the command saved for the sample
	// service in the per-test mock keyring ("" if none is stored). The shared
	// checkStoredPassword helper can't express "nothing stored" or "random,
	// but 32 characters", so those cases read the keyring through this.
	storedPassword := func(t *testing.T) string {
		t.Helper()
		storage := &common.KeyringStorage{}
		password, err := storage.Get(api.Service{ProjectID: testProjectID, ServiceID: "svc-12345"}, "tsdbadmin")
		if err != nil {
			return ""
		}
		return password
	}

	savedStderr := "Password saved to system keyring for automatic authentication\n" +
		"To view your new password, run: \n\t tiger service get svc-12345 --with-password\n" +
		"✅ Master password for 'tsdbadmin' user updated successfully\n"

	runCmdTests(t, []cmdTest{
		{
			name:    "new-password and auto-generate conflict",
			args:    []string{"service", "update-password", "svc-12345", "--new-password", "newpass123", "--auto-generate"},
			wantErr: "if any flags in the group [new-password auto-generate] are set none of the others can be; [auto-generate new-password] were all set",
		},
		{
			name:    "not logged in",
			args:    []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: notLoggedInMsg,
			checks:  []checkFunc{checkExitCode(common.ExitAuthenticationError)},
		},
		{
			name:    "read-only all refuses",
			args:    []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "all"})},
			wantErr: "this operation is not allowed in read-only mode",
		},
		{
			// prod judges the fetched service by its environment tag; no
			// update expectation is registered, so an attempted mutation
			// fails as an unexpected call.
			name:    "read-only prod refuses PROD service",
			args:    []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			opts:    []runOption{withConfig(map[string]any{"read_only": "prod"})},
			setup:   expectTaggedService("PROD"),
			wantErr: `this operation is not allowed on services tagged PROD while read_only is set to "prod"`,
		},
		{
			name: "read-only prod allows DEV service",
			args: []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			opts: []runOption{withConfig(map[string]any{"read_only": "prod"})},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectTaggedService("DEV")(m)
				setupUpdate("newpass123")(m)
			},
			wantStderr: savedStderr,
			checks:     []checkFunc{checkStoredPassword("svc-12345", "newpass123")},
		},
		{
			name:    "missing service id",
			args:    []string{"service", "update-password", "--new-password", "newpass123"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name:    "env password and auto-generate conflict",
			args:    []string{"service", "update-password", "svc-12345", "--auto-generate"},
			opts:    []runOption{withEnv("TIGER_NEW_PASSWORD", "env-pass-456")},
			wantErr: "cannot use --auto-generate and --new-password together",
		},
		{
			name: "network error on get",
			args: []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to get service details: connection refused",
		},
		{
			name: "API error on get",
			args: []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			checks:  []checkFunc{checkExitCode(common.ExitServiceNotFound)},
		},
		{
			name: "nil response body on get",
			args: []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name: "read replica rejected",
			args: []string{"service", "update-password", "rep1234567", "--new-password", "newpass123"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				replica := sampleService(func(s *api.Service) {
					s.ServiceID = "rep1234567"
					s.ForkedFrom = &api.ForkSpec{IsStandby: new(true), ServiceID: new("svcprimary")}
				})
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "rep1234567").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      &replica,
					}, nil)
			},
			wantErr: `"rep1234567" is a read replica; update the password on its primary service "svcprimary" instead`,
		},
		{
			name:    "non-interactive without password",
			args:    []string{"service", "update-password", "svc-12345"},
			setup:   setupGet,
			wantErr: "TTY not detected - use --new-password flag, --auto-generate flag, or TIGER_NEW_PASSWORD environment variable",
		},
		{
			name: "network error on update",
			args: []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupGet(m)
				m.EXPECT().UpdatePasswordWithResponse(validCtx, testProjectID, "svc-12345", api.UpdatePasswordInput{Password: "newpass123"}).
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to update password: connection refused",
		},
		{
			name: "API error on update",
			args: []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupGet(m)
				m.EXPECT().UpdatePasswordWithResponse(validCtx, testProjectID, "svc-12345", api.UpdatePasswordInput{Password: "newpass123"}).
					Return(&api.UpdatePasswordResponse{
						HTTPResponse: httpResponse(http.StatusForbidden),
						JSON4XX:      &api.Error{Message: new("permission denied")},
					}, nil)
			},
			wantErr: "permission denied",
			checks:  []checkFunc{checkExitCode(common.ExitPermissionDenied)},
		},
		{
			name: "new-password flag",
			args: []string{"service", "update-password", "svc-12345", "--new-password", "newpass123"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupGet(m)
				setupUpdate("newpass123")(m)
			},
			wantStderr: savedStderr,
			checks:     []checkFunc{checkStoredPassword("svc-12345", "newpass123")},
		},
		{
			name: "env var password",
			args: []string{"service", "update-password", "svc-12345"},
			opts: []runOption{withEnv("TIGER_NEW_PASSWORD", "env-pass-456")},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupGet(m)
				setupUpdate("env-pass-456")(m)
			},
			wantStderr: savedStderr,
			checks:     []checkFunc{checkStoredPassword("svc-12345", "env-pass-456")},
		},
		{
			name: "auto-generate",
			args: []string{"service", "update-password", "svc-12345", "--auto-generate"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupGet(m)
				setupUpdateAny(m)
			},
			wantStderr: "Successfully generated a new password.\n" + savedStderr,
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				if got := storedPassword(t); len(got) != 32 {
					t.Errorf("stored password length = %d, want 32", len(got))
				}
			}},
		},
		{
			name: "interactive prompt",
			args: []string{"service", "update-password", "svc-12345"},
			opts: []runOption{withIsTerminal(true), withReadPassword("prompted-pass")},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupGet(m)
				setupUpdate("prompted-pass")(m)
			},
			wantStderr: "Enter new password (leave empty to generate): \n" + savedStderr,
			checks:     []checkFunc{checkStoredPassword("svc-12345", "prompted-pass")},
		},
		{
			name: "interactive prompt empty generates",
			args: []string{"service", "update-password", "svc-12345"},
			opts: []runOption{withIsTerminal(true), withReadPassword("")},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupGet(m)
				setupUpdateAny(m)
			},
			wantStderr: "Enter new password (leave empty to generate): \n" +
				"Successfully generated a new password.\n" + savedStderr,
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				if got := storedPassword(t); len(got) != 32 {
					t.Errorf("stored password length = %d, want 32", len(got))
				}
			}},
		},
		{
			name: "password storage none",
			args: []string{"service", "update-password", "svc-12345", "--new-password", "newpass123", "--password-storage", "none"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupGet(m)
				setupUpdate("newpass123")(m)
			},
			wantStderr: "✅ Master password for 'tsdbadmin' user updated successfully\n",
			checks: []checkFunc{func(t *testing.T, result cmdResult) {
				if got := storedPassword(t); got != "" {
					t.Errorf("stored password = %q, want none stored", got)
				}
			}},
		},
		{
			name: "default service id from config",
			args: []string{"service", "update-password", "--new-password", "newpass123"},
			opts: []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				setupGet(m)
				setupUpdate("newpass123")(m)
			},
			wantStderr: savedStderr,
			checks:     []checkFunc{checkStoredPassword("svc-12345", "newpass123")},
		},
	})
}
