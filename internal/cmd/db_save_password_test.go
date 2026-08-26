package cmd

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
)

func TestDbSavePasswordCmd(t *testing.T) {
	setupGetService := func(m *mocks.MockClientWithResponsesInterface) {
		expectGetService(m, "svc-12345", sampleService())
	}

	// checkKeyringPassword asserts the mock keyring holds want for role.
	checkKeyringPassword := func(role, want string) func(*testing.T, cmdResult) {
		return func(t *testing.T, result cmdResult) {
			t.Helper()
			got, err := (&common.KeyringStorage{}).Get(sampleService(), role)
			if err != nil {
				t.Fatalf("failed to read saved password: %v", err)
			}
			if got != want {
				t.Errorf("expected stored password %q, got %q", want, got)
			}
		}
	}

	// pgpass entry prefix for sampleService's endpoint and the default role.
	const pgpassPrefix = "svc-12345.project.tsdb.cloud.timescale.com:5432:tsdb:tsdbadmin:"
	pgpassHome := t.TempDir()
	overwriteHome := t.TempDir()

	tests := []cmdTest{
		{
			name:    "not logged in",
			args:    []string{"db", "save-password", "svc-12345", "--password=pw"},
			opts:    []runOption{withNotLoggedIn()},
			wantErr: "authentication required: not logged in. Please run 'tiger auth login'",
			check:   checkExitCode(common.ExitAuthenticationError),
		},
		{
			name:    "service ID required",
			args:    []string{"db", "save-password", "--password=pw"},
			wantErr: "service ID is required. Provide it as an argument or set a default with 'tiger config set service_id <service-id>'",
		},
		{
			name: "network error fetching service",
			args: []string{"db", "save-password", "svc-12345", "--password=pw"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(nil, errors.New("connection refused"))
			},
			wantErr: "failed to fetch service details: connection refused",
		},
		{
			name: "API error fetching service",
			args: []string{"db", "save-password", "svc-12345", "--password=pw"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusNotFound),
						JSON4XX:      &api.Error{Message: new("service not found")},
					}, nil)
			},
			wantErr: "service not found",
			check:   checkExitCode(common.ExitServiceNotFound),
		},
		{
			name: "nil response body",
			args: []string{"db", "save-password", "svc-12345", "--password=pw"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "svc-12345").
					Return(&api.GetServiceResponse{
						HTTPResponse: httpResponse(http.StatusOK),
						JSON200:      nil,
					}, nil)
			},
			wantErr: "empty response from API",
		},
		{
			name:    "empty password via flag",
			args:    []string{"db", "save-password", "svc-12345", "--password="},
			setup:   setupGetService,
			wantErr: "password cannot be empty when provided via --password flag",
		},
		{
			name:    "no password without a TTY",
			args:    []string{"db", "save-password", "svc-12345"},
			setup:   setupGetService,
			wantErr: "TTY not detected - password required. Use --password flag or TIGER_NEW_PASSWORD environment variable",
		},
		{
			name:       "empty password at prompt",
			args:       []string{"db", "save-password", "svc-12345"},
			opts:       []runOption{withIsTerminal(true), withReadPassword("")},
			setup:      setupGetService,
			wantErr:    "password cannot be empty",
			wantStderr: "Enter password: \nError: password cannot be empty\n",
		},
		{
			name:       "saves password from flag",
			args:       []string{"db", "save-password", "svc-12345", "--password=flag-pw"},
			setup:      setupGetService,
			wantStderr: "Password saved successfully for service svc-12345 (role: tsdbadmin)\n",
			check:      checkKeyringPassword("tsdbadmin", "flag-pw"),
		},
		{
			name:       "default service ID from config",
			args:       []string{"db", "save-password", "--password=default-pw"},
			opts:       []runOption{withConfig(map[string]any{"service_id": "svc-12345"})},
			setup:      setupGetService,
			wantStderr: "Password saved successfully for service svc-12345 (role: tsdbadmin)\n",
			check:      checkKeyringPassword("tsdbadmin", "default-pw"),
		},
		{
			name:       "saves password from TIGER_NEW_PASSWORD",
			args:       []string{"db", "save-password", "svc-12345"},
			opts:       []runOption{withEnv("TIGER_NEW_PASSWORD", "env-pw")},
			setup:      setupGetService,
			wantStderr: "Password saved successfully for service svc-12345 (role: tsdbadmin)\n",
			check:      checkKeyringPassword("tsdbadmin", "env-pw"),
		},
		{
			name:       "flag takes precedence over TIGER_NEW_PASSWORD",
			args:       []string{"db", "save-password", "svc-12345", "--password=flag-pw"},
			opts:       []runOption{withEnv("TIGER_NEW_PASSWORD", "env-pw")},
			setup:      setupGetService,
			wantStderr: "Password saved successfully for service svc-12345 (role: tsdbadmin)\n",
			check:      checkKeyringPassword("tsdbadmin", "flag-pw"),
		},
		{
			name:       "prompts for password on a TTY",
			args:       []string{"db", "save-password", "svc-12345"},
			opts:       []runOption{withIsTerminal(true), withReadPassword("prompt-pw")},
			setup:      setupGetService,
			wantStderr: "Enter password: \nPassword saved successfully for service svc-12345 (role: tsdbadmin)\n",
			check:      checkKeyringPassword("tsdbadmin", "prompt-pw"),
		},
		{
			name:       "custom role",
			args:       []string{"db", "save-password", "svc-12345", "--password=readonly-pw", "--role", "readonly"},
			setup:      setupGetService,
			wantStderr: "Password saved successfully for service svc-12345 (role: readonly)\n",
			check: func(t *testing.T, result cmdResult) {
				checkKeyringPassword("readonly", "readonly-pw")(t, result)
				if pw, err := (&common.KeyringStorage{}).Get(sampleService(), "tsdbadmin"); err == nil {
					t.Errorf("expected no password stored for tsdbadmin, got %q", pw)
				}
			},
		},
		{
			name:       "pgpass storage",
			args:       []string{"db", "save-password", "svc-12345", "--password=pgpass-pw", "--password-storage", "pgpass"},
			opts:       []runOption{withEnv("HOME", pgpassHome)},
			setup:      setupGetService,
			wantStderr: "Password saved successfully for service svc-12345 (role: tsdbadmin)\n",
			check: func(t *testing.T, result cmdResult) {
				data, err := os.ReadFile(filepath.Join(pgpassHome, ".pgpass"))
				if err != nil {
					t.Fatalf("failed to read .pgpass: %v", err)
				}
				assertOutput(t, string(data), pgpassPrefix+"pgpass-pw\n")
			},
		},
		{
			name:       "pgpass save overwrites existing entry",
			args:       []string{"db", "save-password", "svc-12345", "--password=first-pw", "--password-storage", "pgpass"},
			opts:       []runOption{withEnv("HOME", overwriteHome)},
			setup:      setupGetService,
			wantStderr: "Password saved successfully for service svc-12345 (role: tsdbadmin)\n",
			check: func(t *testing.T, result cmdResult) {
				second := runCommand(t,
					[]string{"db", "save-password", "svc-12345", "--password=second-pw", "--password-storage", "pgpass"},
					setupGetService,
					withEnv("HOME", overwriteHome))
				if second.err != nil {
					t.Fatalf("second save failed: %v", second.err)
				}
				data, err := os.ReadFile(filepath.Join(overwriteHome, ".pgpass"))
				if err != nil {
					t.Fatalf("failed to read .pgpass: %v", err)
				}
				assertOutput(t, string(data), pgpassPrefix+"second-pw\n")
			},
		},
		{
			name:       "none storage saves nothing",
			args:       []string{"db", "save-password", "svc-12345", "--password=none-pw", "--password-storage", "none"},
			setup:      setupGetService,
			wantStderr: "Password saved successfully for service svc-12345 (role: tsdbadmin)\n",
			check: func(t *testing.T, result cmdResult) {
				if pw, err := (&common.KeyringStorage{}).Get(sampleService(), "tsdbadmin"); err == nil {
					t.Errorf("expected no stored password, got %q", pw)
				}
			},
		},
		{
			name: "replica ID saves against parent primary",
			args: []string{"db", "save-password", "rep-67890", "--password=replica-pw"},
			setup: func(m *mocks.MockClientWithResponsesInterface) {
				expectGetService(m, "rep-67890", sampleReplica())
				setupGetService(m)
			},
			wantStderr: "Read replicas share the primary's credentials; saving against primary svc-12345.\nPassword saved successfully for service svc-12345 (role: tsdbadmin)\n",
			check: func(t *testing.T, result cmdResult) {
				// Stored against the parent primary, matching the connect read path.
				checkKeyringPassword("tsdbadmin", "replica-pw")(t, result)
				if pw, err := (&common.KeyringStorage{}).Get(sampleReplica(), "tsdbadmin"); err == nil {
					t.Errorf("expected no password stored under the replica ID, got %q", pw)
				}
			},
		},
	}

	runCmdTests(t, tests)
}
