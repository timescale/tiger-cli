package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/zalando/go-keyring"
	"go.uber.org/mock/gomock"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// discardCmd returns a bare command whose output streams are discarded, for
// tests that call a helper taking a *cobra.Command without caring what it
// prints.
func discardCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

// stubIsTerminal makes util.IsTerminal report val for the duration of the test,
// so commands take their interactive path against a non-TTY stdin.
func stubIsTerminal(t *testing.T, val bool) {
	t.Helper()
	original := util.IsTerminal
	util.IsTerminal = func(any) bool { return val }
	t.Cleanup(func() { util.IsTerminal = original })
}

// stubReadPassword makes util.ReadPassword return password for the duration of
// the test. The real implementation needs stdin to be an *os.File, so password
// prompts can't be driven with cmd.SetIn.
func stubReadPassword(t *testing.T, password string) {
	t.Helper()
	original := util.ReadPassword
	util.ReadPassword = func(context.Context, io.Reader) (string, error) { return password, nil }
	t.Cleanup(func() { util.ReadPassword = original })
}

func TestMain(m *testing.M) {
	// Replace the system keyring with an in-memory mock so that tests never
	// read, write, or delete real credentials or passwords.
	keyring.MockInit()

	// Scrub inherited TIGER_* env vars (e.g. from the developer's shell or a
	// sourced .env file) so tests run with a consistent baseline. Integration
	// test credentials (TIGER_*_INTEGRATION) are deliberately preserved. Tests
	// that need an env var set opt in with withEnv.
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "TIGER_") && !strings.HasSuffix(key, "_INTEGRATION") {
			os.Unsetenv(key)
		}
	}

	os.Exit(m.Run())
}

// testProjectID is the project ID the injected client factory reports.
const testProjectID = "test-project-123"

type cmdResult struct {
	stdout    string
	stderr    string
	err       error
	configDir string
}

type runOption func(*runConfig)

type runConfig struct {
	stdin        io.Reader
	isTerminal   *bool // if set, overrides util.IsTerminal for this test
	ctx          context.Context
	envVars      map[string]string
	configDir    string         // if set, reused instead of a fresh t.TempDir()
	configValues map[string]any // if set, written to the config file before the command runs
	credentials  *config.Credentials
	clientErr    error              // if set, the client factory returns this error (nil client)
	openBrowser  func(string) error // if set, overrides the openBrowser stub for this test
	readPassword *string            // if set, util.ReadPassword returns this value
}

func withStdin(input string) runOption {
	return func(rc *runConfig) {
		rc.stdin = strings.NewReader(input)
	}
}

// withContext sets the context the command runs under. Use this for commands
// that block until the context is cancelled (e.g. `tiger mcp start`): pass an
// already-cancelled context to exercise the command without leaving a server
// running for the duration of the test.
func withContext(ctx context.Context) runOption {
	return func(rc *runConfig) {
		rc.ctx = ctx
	}
}

// withIsTerminal overrides util.IsTerminal for the duration of the test.
// Use this with withStdin to simulate interactive terminal input.
func withIsTerminal(isTerminal bool) runOption {
	return func(rc *runConfig) {
		rc.isTerminal = &isTerminal
	}
}

func withEnv(key, value string) runOption {
	return func(rc *runConfig) {
		rc.envVars[key] = value
	}
}

// withConfigDir reuses an existing config directory instead of a fresh
// t.TempDir(). Use it to chain commands that must observe each other's writes
// (e.g. `config set` followed by `config show`), passing the configDir from a
// previous cmdResult.
func withConfigDir(dir string) runOption {
	return func(rc *runConfig) {
		rc.configDir = dir
	}
}

// withConfig seeds the test's config file with the given keys before the
// command runs (e.g. map[string]any{"service_id": "svc-123", "read_only": true}).
func withConfig(values map[string]any) runOption {
	return func(rc *runConfig) {
		rc.configValues = values
	}
}

// withStoredCredentials stores the given credentials (in the test's mock
// keyring entry) before the command runs, for commands that read or rewrite
// stored credentials (e.g. `tiger auth status`, `tiger auth logout`).
func withStoredCredentials(creds config.Credentials) runOption {
	return func(rc *runConfig) {
		rc.credentials = &creds
	}
}

// withClientError makes the client factory return the given error instead of a
// mock client. This simulates scenarios where credentials are invalid.
func withClientError(err error) runOption {
	return func(rc *runConfig) {
		rc.clientErr = err
	}
}

// withNotLoggedIn makes the client factory fail exactly the way production does
// when no credentials are stored.
func withNotLoggedIn() runOption {
	return withClientError(notLoggedInError())
}

// notLoggedInError mirrors the error common.NewAPIClient returns when no
// credentials are stored.
func notLoggedInError() error {
	return common.ExitWithCode(common.ExitAuthenticationError,
		fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", config.ErrNotLoggedIn))
}

// withOpenBrowser overrides openBrowser for the duration of the test. By
// default, runCommand stubs openBrowser to return an error. Use this to
// simulate a successful browser open (pass a nil-returning func).
func withOpenBrowser(f func(string) error) runOption {
	return func(rc *runConfig) {
		rc.openBrowser = f
	}
}

// withReadPassword makes util.ReadPassword return the given password. The real
// implementation needs stdin to be an *os.File, so password prompts can't be
// driven with withStdin.
func withReadPassword(password string) runOption {
	return func(rc *runConfig) {
		rc.readPassword = &password
	}
}

// runCommand builds the root command, injects a mock API client, and executes
// with the given args against an isolated temp config directory. Returns
// captured stdout, stderr, and any error from Execute.
func runCommand(
	t *testing.T,
	args []string,
	setupMock func(m *mocks.MockClientWithResponsesInterface),
	opts ...runOption,
) cmdResult {
	t.Helper()

	rc := &runConfig{
		ctx:     context.Background(),
		envVars: map[string]string{},
	}
	for _, opt := range opts {
		opt(rc)
	}

	// Isolate keyring entries (credentials, saved passwords) per test.
	config.SetTestServiceName(t)

	// Set and restore env vars. Must happen before buildRootCmd, which reads
	// TIGER_EXPERIMENTAL at build time.
	for k, v := range rc.envVars {
		t.Setenv(k, v)
	}

	// Create mock
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockClientWithResponsesInterface(ctrl)
	if setupMock != nil {
		setupMock(mockClient)
	}

	// Build command and inject mock
	cmd, app, err := buildRootCmd(rc.ctx)
	if err != nil {
		t.Fatalf("buildRootCmd failed: %v", err)
	}

	configDir := rc.configDir
	if configDir == "" {
		configDir = t.TempDir()
	}
	if rc.configValues != nil {
		if _, err := config.UseTestConfig(configDir, rc.configValues); err != nil {
			t.Fatalf("failed to seed config file: %v", err)
		}
	}
	if rc.credentials != nil {
		seedCfg := &config.Config{ConfigDir: configDir}
		if rc.credentials.OAuth != nil {
			err = seedCfg.StoreOAuthCredentials(rc.credentials.OAuth, rc.credentials.ProjectID)
		} else {
			err = seedCfg.StoreCredentials(rc.credentials.APIKey, rc.credentials.ProjectID)
		}
		if err != nil {
			t.Fatalf("failed to seed stored credentials: %v", err)
		}
	}
	app.SetClientFactory(func(ctx context.Context, cfg *config.Config) (api.ClientWithResponsesInterface, string, error) {
		if rc.clientErr != nil {
			return nil, "", rc.clientErr
		}
		return mockClient, testProjectID, nil
	})

	// Capture output, stripping ANSI color/style sequences so tests can use
	// plain expected strings without embedded escape codes.
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&colorprofile.Writer{Forward: &stdout, Profile: colorprofile.NoTTY})
	cmd.SetErr(&colorprofile.Writer{Forward: &stderr, Profile: colorprofile.NoTTY})

	// Set stdin if provided
	if rc.stdin != nil {
		cmd.SetIn(rc.stdin)
	}

	// Override util.IsTerminal if requested
	if rc.isTerminal != nil {
		stubIsTerminal(t, *rc.isTerminal)
	}

	// Override util.ReadPassword if requested
	if rc.readPassword != nil {
		stubReadPassword(t, *rc.readPassword)
	}

	// Prevent browser opens in tests (default: return error). Tests that need
	// to simulate a successful browser open use withOpenBrowser.
	originalOpenBrowser := openBrowser
	if rc.openBrowser != nil {
		openBrowser = rc.openBrowser
	} else {
		openBrowser = func(url string) error {
			return errors.New("browser disabled in tests")
		}
	}
	t.Cleanup(func() { openBrowser = originalOpenBrowser })

	// Always include flags that prevent side effects in tests:
	// --config-dir: isolate from real config
	// --analytics=false: prevent analytics calls on the mock
	// --skip-update-check: prevent version check HTTP calls
	baseArgs := []string{
		"--config-dir", configDir,
		"--analytics=false",
		"--skip-update-check",
	}
	cmd.SetArgs(append(baseArgs, args...))

	execErr := cmd.Execute()

	return cmdResult{
		stdout:    stdout.String(),
		stderr:    stderr.String(),
		err:       execErr,
		configDir: configDir,
	}
}

// readStoredCredentials reads the credentials stored for the test (in the mock
// keyring or the config dir's fallback file). Use with withStoredCredentials
// to verify credential rewrites, or after `auth login` to verify storage.
func readStoredCredentials(t *testing.T, configDir string) (*config.Credentials, error) {
	t.Helper()
	cfg := &config.Config{ConfigDir: configDir}
	return cfg.GetStoredCredentials()
}

// httpResponse creates a minimal *http.Response with the given status code.
func httpResponse(statusCode int) *http.Response {
	return &http.Response{StatusCode: statusCode}
}

// sampleService returns an api.Service with reasonable defaults.
// Use overrides to customize specific fields.
func sampleService(overrides ...func(*api.Service)) api.Service {
	svc := api.Service{
		ServiceID:   "svc-12345",
		ProjectID:   testProjectID,
		Name:        "test-service",
		ServiceType: api.ServiceTypeTIMESCALEDB,
		RegionCode:  "us-east-1",
		Status:      api.DeployStatusREADY,
		Created:     time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		Endpoint: &api.Endpoint{
			Host: new("svc-12345.project.tsdb.cloud.timescale.com"),
			Port: new(5432),
		},
		Resources: []api.Resource{{
			ID: new("resource-1"),
			Spec: &api.ResourceSpec{
				CPUMillis: new(1000),
				MemoryGbs: new(4),
			},
		}},
	}
	for _, o := range overrides {
		o(&svc)
	}
	return svc
}

// validCtx is a gomock matcher that verifies a context.Context parameter is
// non-nil. Use this instead of gomock.Any() for context parameters.
var validCtx = gomock.Cond(func(x any) bool {
	ctx, ok := x.(context.Context)
	return ok && ctx != nil
})

// assertOutput checks that got exactly equals want, showing a unified diff on mismatch.
func assertOutput(t *testing.T, got, want string) {
	t.Helper()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}

// cmdTest is the standard test case struct for table-driven command tests.
type cmdTest struct {
	name       string
	args       []string
	setup      func(m *mocks.MockClientWithResponsesInterface)
	opts       []runOption
	wantStdout string
	wantStderr string
	wantErr    string
	check      func(t *testing.T, result cmdResult) // optional extra assertions after the standard ones
}

// runCmdTests runs a slice of table-driven command tests using the standard
// assertion pattern: check wantErr, then wantStdout, then wantStderr.
//
// When wantErr is set and wantStderr is empty, the expected stderr is
// automatically derived from the error message (Cobra prints "Error: <msg>\n"
// to stderr for any error returned by RunE).
func runCmdTests(t *testing.T, tests []cmdTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCommand(t, tt.args, tt.setup, tt.opts...)

			if tt.wantErr != "" {
				if result.err == nil {
					t.Fatal("expected error, got nil")
				}
				assertOutput(t, result.err.Error(), tt.wantErr)
			} else if result.err != nil {
				t.Fatalf("unexpected error: %v", result.err)
			}

			assertOutput(t, result.stdout, tt.wantStdout)

			wantStderr := tt.wantStderr
			if wantStderr == "" && tt.wantErr != "" {
				// Cobra prints "Error: <msg>\n" to stderr for RunE errors
				wantStderr = "Error: " + tt.wantErr + "\n"
			}
			assertOutput(t, result.stderr, wantStderr)

			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

// testFlags returns a flag set shaped like a command's, with --config-dir pointed
// at dir, so config.Load resolves the same way it would for a real command.
func testFlags(t *testing.T, dir string) *pflag.FlagSet {
	t.Helper()
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("config-dir", "", "config directory")
	if err := flags.Set("config-dir", dir); err != nil {
		t.Fatalf("Failed to set config-dir flag: %v", err)
	}
	return flags
}
