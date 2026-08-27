package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/zalando/go-keyring"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

func TestMain(m *testing.M) {
	// Backstop: replace the system keyring with an in-memory mock so that even
	// a test that forgets to reset can never read, write, or delete real
	// credentials or passwords. Per-test isolation comes from the fresh
	// keyring.MockInit() in runCommand and in tests that use the keyring
	// directly.
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

	// Pin the local timezone to UTC so output that renders local times is
	// deterministic. This must happen here, while the process is still
	// single-goroutine: mutating time.Local mid-run (as a per-test option
	// once did) races with any background goroutine that calls time.Now —
	// e.g. an idle HTTP transport read-loop left over from an earlier test's
	// httptest traffic.
	time.Local = time.UTC

	os.Exit(m.Run())
}

// testProjectID is the project ID the injected client factory reports.
const testProjectID = "test-project-123"

// cmdTest is the standard test case struct for table-driven command tests.
//
// The want fields hold either a string, compared exactly (the standard), or a
// matcher (matchRegexp, matchPrefix, matchFunc) for output that is inherently
// nondeterministic — say why in a comment. Left unset, wantStdout/wantStderr
// assert that the stream is empty, and wantErr asserts that the command
// succeeded.
type cmdTest struct {
	name       string
	args       []string
	setup      func(m *mocks.MockClientWithResponsesInterface)
	opts       []runOption
	wantStdout any
	wantStderr any
	wantErr    any
	checks     []checkFunc // optional extra assertions, run in order after the standard ones

	// synctest runs the case inside a [synctest] bubble, where time is
	// virtual: a poll interval or wait timeout elapses the instant every
	// goroutine is blocked on it. Set it for cases that wait on a timer (the
	// `--wait` polling loop in common.WaitForService), so they assert against
	// realistic durations and finish instantly instead of sleeping.
	//
	// It is opt-in rather than the default because a bubble only advances its
	// clock while every goroutine in it is durably blocked, and real network
	// I/O isn't: a case that waits on a timer *and* on a loopback
	// httptest.NewServer hangs until the test binary's own timeout. Cases that
	// talk to such a server and never wait on a timer are fine either way, and
	// simply don't need it.
	//
	// httptest.NewTestServer's in-memory network is bubble-safe and would lift
	// that restriction, but only the client from its Client method reaches it,
	// and api.HTTPClient — which every request under test goes through — has
	// no seam for swapping in that transport.
	synctest bool
}

// runCmdTests runs a slice of table-driven command tests using the standard
// assertion pattern: check wantErr, then wantStdout, then wantStderr.
//
// When wantErr is set and wantStderr isn't, the expected stderr is
// automatically derived: for a string, "Error: <msg>\n" (what Cobra prints for
// any error returned by RunE); for a matcher, the same matcher is applied to
// stderr with that framing stripped. Commands that set SilenceErrors print
// differently, so their error cases must set wantStderr explicitly (note an
// explicit "" is a real assertion — nil and "" differ for these any-typed
// fields).
func runCmdTests(t *testing.T, tests []cmdTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.synctest {
				synctest.Test(t, func(t *testing.T) { runCmdTest(t, tt) })
				return
			}
			runCmdTest(t, tt)
		})
	}
}

// runCmdTest runs one case and makes the standard assertions. Split out of
// runCmdTests so a case can opt into running inside a synctest bubble.
func runCmdTest(t *testing.T, tt cmdTest) {
	t.Helper()
	result := runCommand(t, tt.args, tt.setup, tt.opts...)

	if tt.wantErr != nil {
		if result.err == nil {
			t.Fatal("expected error, got nil")
		}
		assertMatch(t, "error", result.err.Error(), tt.wantErr)
	} else if result.err != nil {
		t.Fatalf("unexpected error: %v", result.err)
	}

	assertMatch(t, "stdout", result.stdout, tt.wantStdout)

	wantStderr := tt.wantStderr
	if wantStderr == nil && tt.wantErr != nil {
		if msg, ok := tt.wantErr.(string); ok {
			wantStderr = "Error: " + msg + "\n"
		} else {
			// A matcher can't be framed, so unframe stderr instead.
			got := strings.TrimSuffix(strings.TrimPrefix(result.stderr, "Error: "), "\n")
			assertMatch(t, "stderr", got, tt.wantErr)
			wantStderr = matchFunc(func(*testing.T, string) {}) // already asserted
		}
	}
	assertMatch(t, "stderr", result.stderr, wantStderr)

	for _, check := range tt.checks {
		check(t, result)
	}
}

// assertMatch asserts got against want: nil expects empty, a string is
// compared exactly (the standard), and a matcher applies its own assertion.
func assertMatch(t *testing.T, name, got string, want any) {
	t.Helper()
	switch want := want.(type) {
	case nil:
		if got != "" {
			t.Errorf("%s = %q, want empty", name, got)
		}
	case string:
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", name, diff)
		}
	case matcher:
		want(t, name, got)
	default:
		t.Fatalf("%s: unsupported want type %T", name, want)
	}
}

// matcher is a non-exact assertion for a cmdTest want field. Use the
// constructors below; exact string matching remains the standard.
type matcher func(t *testing.T, name, got string)

// matchRegexp asserts that the value matches the anchored pattern.
func matchRegexp(pattern string) matcher {
	re := regexp.MustCompile("^(?:" + pattern + ")$")
	return func(t *testing.T, name, got string) {
		t.Helper()
		if !re.MatchString(got) {
			t.Errorf("%s = %q, want match for %q", name, got, pattern)
		}
	}
}

// matchPrefix asserts that the value starts with prefix (for output whose tail
// is environment-dependent).
func matchPrefix(prefix string) matcher {
	return func(t *testing.T, name, got string) {
		t.Helper()
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("%s = %q, want prefix %q", name, got, prefix)
		}
	}
}

// matchFunc adapts an arbitrary assertion into a matcher, for output that
// needs bespoke verification (e.g. parse-and-inspect).
func matchFunc(f func(t *testing.T, got string)) matcher {
	return func(t *testing.T, _, got string) {
		t.Helper()
		f(t, got)
	}
}

// assertOutput checks that got exactly equals want, showing a unified diff on mismatch.
func assertOutput(t *testing.T, got, want string) {
	t.Helper()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}

type runOption func(*runConfig)

type runConfig struct {
	// Execution environment
	ctx   context.Context
	stdin io.Reader
	setup []func(t *testing.T) // t-scoped setup hooks, run before the command tree is built (see withSetup)

	// Seeded state: config file and stored credentials (env vars are seeded
	// via setup hooks; see withEnv)
	configValues map[string]any      // merged across withConfig calls; written to the config file before the command runs
	credentials  *config.Credentials // if set, stored (in the mocked keyring) before the command runs

	// API client injection
	clientErr error // if set, the client factory returns this error (nil client)
}

type cmdResult struct {
	stdout    string
	stderr    string
	err       error
	configDir string

	// cfg is the config the invocation actually resolved — captured from the
	// client factory, which app.Load calls with the config it just built, so
	// it reflects the real flag > env > file > default precedence the command
	// saw. Commands that reload the config in place (`tiger config set`,
	// clearStaleDefaultService) mutate this same struct, so it also shows the
	// end state. Nil when the command never loaded (help, completion, a
	// config-parse failure).
	cfg *config.Config
}

// runCommand builds the root command, injects a mock API client, and executes
// with the given args against an isolated temp config directory. Returns
// captured stdout, stderr, and any error from Execute.
//
// Not t.Parallel-safe: it stubs package-level vars (openBrowser, and via
// options util.IsTerminal and util.ReadPassword), resets the process-wide
// mock keyring, and withEnv uses t.Setenv.
func runCommand(
	t *testing.T,
	args []string,
	setupMock func(m *mocks.MockClientWithResponsesInterface),
	opts ...runOption,
) cmdResult {
	t.Helper()

	rc := &runConfig{
		ctx: context.Background(),
	}
	for _, opt := range opts {
		opt(rc)
	}

	// Give the run a fresh, empty in-memory keyring so credentials and saved
	// passwords never leak between tests. Tests that seed keyring entries do
	// it via options (withStoredCredentials, withSetup), which run after this.
	keyring.MockInit()

	// Prevent browser opens in tests (default: return error). Installed before
	// the setup hooks so a withOpenBrowser override lands on top of it.
	originalOpenBrowser := openBrowser
	openBrowser = func(url string) error {
		return errors.New("browser disabled in tests")
	}
	t.Cleanup(func() { openBrowser = originalOpenBrowser })

	// Run t-scoped setup hooks (see withSetup) in option order. These must run
	// before buildRootCmd, which reads withEnv's TIGER_EXPERIMENTAL at build
	// time.
	for _, f := range rc.setup {
		f(t)
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

	configDir := t.TempDir()
	if rc.configValues != nil {
		writeConfigFile(t, configDir, rc.configValues)
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
	// The factory runs inside app.Load, so it also serves as the hook for
	// capturing the config the invocation resolved (see cmdResult.cfg).
	var loadedCfg *config.Config
	app.SetClientFactory(func(ctx context.Context, cfg *config.Config) (api.ClientWithResponsesInterface, string, error) {
		loadedCfg = cfg
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
		cfg:       loadedCfg,
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

func withStdin(input string) runOption {
	return func(rc *runConfig) {
		rc.stdin = strings.NewReader(input)
	}
}

// withSetup runs f with the subtest's *testing.T just before the command tree
// is built. Use it from options that stub package-level vars, so their
// t.Cleanup restores at the end of the case that ran them rather than at the
// end of an outer test function whose t they'd otherwise have to capture.
func withSetup(f func(t *testing.T)) runOption {
	return func(rc *runConfig) {
		rc.setup = append(rc.setup, f)
	}
}

// withEnv sets an environment variable for the duration of the test (restored
// when the test ends, not per runCommand call — a chained runCommand inside a
// check still sees it). Options apply in the order they're given.
func withEnv(key, value string) runOption {
	return withSetup(func(t *testing.T) {
		t.Setenv(key, value)
	})
}

// withIsTerminal makes util.IsTerminal report the given value for the duration
// of the test (true lets commands take their interactive path against a
// non-TTY stdin). Use this with withStdin to simulate interactive input.
func withIsTerminal(isTerminal bool) runOption {
	return withSetup(func(t *testing.T) {
		original := util.IsTerminal
		util.IsTerminal = func(any) bool { return isTerminal }
		t.Cleanup(func() { util.IsTerminal = original })
	})
}

// withReadPassword makes util.ReadPassword return the given password. The real
// implementation needs stdin to be an *os.File, so password prompts can't be
// driven with withStdin.
func withReadPassword(password string) runOption {
	return withSetup(func(t *testing.T) {
		original := util.ReadPassword
		util.ReadPassword = func(context.Context, io.Reader) (string, error) { return password, nil }
		t.Cleanup(func() { util.ReadPassword = original })
	})
}

// withOpenBrowser overrides openBrowser for the duration of the test. By
// default, runCommand stubs openBrowser to return an error (installed before
// the setup hooks run, so this override lands on top of it). Use this to
// simulate a successful browser open (pass a nil-returning func).
func withOpenBrowser(f func(string) error) runOption {
	return withSetup(func(t *testing.T) {
		original := openBrowser
		openBrowser = f
		t.Cleanup(func() { openBrowser = original })
	})
}

// withConfig seeds the test's config file with the given keys before the
// command runs (e.g. map[string]any{"service_id": "svc-123", "read_only": true}).
// Repeated withConfig options merge, later values winning per key.
func withConfig(values map[string]any) runOption {
	return func(rc *runConfig) {
		if rc.configValues == nil {
			rc.configValues = map[string]any{}
		}
		maps.Copy(rc.configValues, values)
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

// notLoggedInMsg is the message of the error withNotLoggedIn makes the client
// factory return; test cases expect it as their wantErr.
const notLoggedInMsg = "authentication required: not logged in. Please run 'tiger auth login'"

// notLoggedInError mirrors the error common.NewAPIClient returns when no
// credentials are stored.
func notLoggedInError() error {
	return common.ExitWithCode(common.ExitAuthenticationError,
		fmt.Errorf("authentication required: %w. Please run 'tiger auth login'", config.ErrNotLoggedIn))
}

// checkFunc is an extra assertion a cmdTest runs after the standard ones.
type checkFunc func(t *testing.T, result cmdResult)

// checkExitCode returns a check asserting the command failed with the given
// exit code.
func checkExitCode(want int) checkFunc {
	return func(t *testing.T, result cmdResult) {
		t.Helper()
		var exitErr common.ExitCodeError
		if !errors.As(result.err, &exitErr) {
			t.Fatalf("expected an exit code error, got %v", result.err)
		}
		if exitErr.ExitCode() != want {
			t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), want)
		}
	}
}

// checkDefaultService returns a check asserting the config file's default
// service_id after the command ran.
func checkDefaultService(want string) checkFunc {
	return func(t *testing.T, result cmdResult) {
		t.Helper()
		cfg, err := config.Load(testFlags(t, result.configDir))
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}
		if cfg.ServiceID != want {
			t.Errorf("default service_id = %q, want %q", cfg.ServiceID, want)
		}
	}
}

// writeConfigFile writes exactly the given keys to the config file in
// configDir, creating the directory if needed. Only these keys are written, so
// unspecified ones still resolve from the environment or their defaults —
// which is what makes a seeded config file a real precedence test.
func writeConfigFile(t *testing.T, configDir string, values map[string]any) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	contents, err := yaml.Marshal(values)
	if err != nil {
		t.Fatalf("failed to marshal config values: %v", err)
	}
	if err := os.WriteFile(config.GetConfigFile(configDir), contents, 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
}

// checkNotLoaded asserts the invocation never loaded the config or built an
// API client. wrapCommands deliberately leaves help, completion, and
// __complete unwrapped so they stay clear of the config file, the system
// keyring, and the network; a nil cmdResult.cfg is what that looks like.
func checkNotLoaded(t *testing.T, result cmdResult) {
	t.Helper()
	if result.cfg != nil {
		t.Errorf("expected no config load, but one happened")
	}
}

// readConfigFile parses the config file persisted in configDir.
func readConfigFile(t *testing.T, configDir string) map[string]any {
	t.Helper()
	contents, err := os.ReadFile(config.GetConfigFile(configDir))
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	var values map[string]any
	if err := yaml.Unmarshal(contents, &values); err != nil {
		t.Fatalf("failed to parse config file: %v", err)
	}
	return values
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
// Its Body is nil — fine for the generated response structs' StatusCode()
// checks, but any code path that reads the body would panic.
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

// discardCmd returns a bare command whose output streams are discarded, for
// tests that call a helper taking a *cobra.Command without caring what it
// prints.
func discardCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
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
