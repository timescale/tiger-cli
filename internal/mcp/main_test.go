package mcp

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/api/mocks"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestMain(m *testing.M) {
	// Backstop: replace the system keyring with an in-memory mock so that even
	// a test that forgets to reset can never read, write, or delete real
	// credentials or passwords. Per-test isolation comes from the fresh
	// keyring.MockInit() in runTool.
	keyring.MockInit()

	// Scrub inherited TIGER_* env vars (e.g. from the developer's shell or a
	// sourced .env file) so tests run with a consistent baseline. Integration
	// test credentials (TIGER_*_INTEGRATION) are deliberately preserved.
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "TIGER_") && !strings.HasSuffix(key, "_INTEGRATION") {
			os.Unsetenv(key)
		}
	}

	// Pin the local timezone to UTC so output that renders local times is
	// deterministic. This must happen here, while the process is still
	// single-goroutine: mutating time.Local mid-run races with any background
	// goroutine that calls time.Now.
	time.Local = time.UTC

	os.Exit(m.Run())
}

// testProjectID is the project ID the injected client factory reports.
const testProjectID = "test-project-id"

// toolTest is the standard test case struct for table-driven MCP tool tests.
// The tool is called through a real in-memory client/server session, so a
// case exercises the SDK's request path — middleware, schema validation, and
// the elicitation round trips — not just the handler.
//
// wantErr is the exact text of the error result and wantOutput the exact
// structured content. Left unset, wantErr asserts the call succeeded and
// wantOutput asserts it returned no structured content.
type toolTest struct {
	name   string
	tool   string
	args   map[string]any
	mock   func(m *mocks.MockClientWithResponsesInterface)
	opts   []runOption
	checks []checkFunc // optional extra assertions, run in order after the standard ones

	wantErr    string
	wantOutput map[string]any
	// wantPrompt is the elicitation the tool is expected to raise, as the
	// client receives it after the JSON round trip. Left nil, it asserts no
	// prompt is raised. The user's answer comes from withElicitation.
	wantPrompt *mcp.ElicitParams
	// wantCallErr is the exact text of a transport error, which is how the SDK
	// reports a tool the server never registered — a handler's own error comes
	// back as a result, not as one of these. Set it and the result assertions
	// are skipped, since there is no result.
	wantCallErr string

	// synctest runs the case inside a [synctest] bubble, where time is
	// virtual: a poll interval or wait timeout elapses the instant every
	// goroutine is blocked on it. Set it for cases that wait on a timer (the
	// `wait` polling loop in common.WaitForService), so they assert against
	// realistic durations and finish instantly instead of sleeping.
	//
	// The bubble's clock also always starts at the same instant, 2000-01-01
	// UTC, so a case whose request depends on time.Now (service_logs sends it
	// as the default upper bound) sets synctest too and spells that time out
	// as a plain literal.
	synctest bool
}

// runToolTests runs a slice of table-driven tool tests using the standard
// assertion pattern: check wantCallErr, then wantPrompt, wantErr, and
// wantOutput, then the checks.
func runToolTests(t *testing.T, tests []toolTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.synctest {
				synctest.Test(t, func(t *testing.T) { runToolTest(t, tt) })
				return
			}
			runToolTest(t, tt)
		})
	}
}

// runToolTest runs one case and makes the standard assertions. Split out of
// runToolTests so a case can opt into running inside a synctest bubble.
func runToolTest(t *testing.T, tt toolTest) {
	t.Helper()
	result := runTool(t, tt.tool, tt.args, tt.mock, tt.opts...)

	if tt.wantCallErr != "" {
		if result.err == nil {
			t.Fatalf("expected call error %q, got none", tt.wantCallErr)
		}
		if got := result.err.Error(); got != tt.wantCallErr {
			t.Errorf("call error = %q, want %q", got, tt.wantCallErr)
		}
		return
	}
	if result.err != nil {
		t.Fatalf("unexpected call error: %v", result.err)
	}

	if diff := cmp.Diff(tt.wantPrompt, result.prompt); diff != "" {
		t.Errorf("prompt mismatch (-want +got):\n%s", diff)
	}
	if got := resultError(result.res); got != tt.wantErr {
		t.Errorf("error = %q, want %q", got, tt.wantErr)
	}
	var wantStructured any
	if tt.wantOutput != nil {
		wantStructured = tt.wantOutput
	}
	if diff := cmp.Diff(wantStructured, result.res.StructuredContent); diff != "" {
		t.Errorf("structured content mismatch (-want +got):\n%s", diff)
	}

	for _, check := range tt.checks {
		check(t, result)
	}
}

type runOption func(*runConfig)

type runConfig struct {
	// Execution environment
	setup []func(t *testing.T) // t-scoped setup hooks, run before the server is built (see withSetup)

	// Seeded state: the config file the server starts with, and a rewrite of
	// it once the server is running
	configValues     map[string]any // merged across withConfig calls; written to the config file before the server starts
	configAfterStart map[string]any // if set, merged over configValues and written once the server is running (see withConfigAfterStart)

	// API client injection
	clientErr    error // if set, the client factory returns this error (nil client)
	experimental bool  // turns on the App's experimental gate, so the gated tools are registered

	// MCP client
	clientCaps *mcp.ClientCapabilities // overrides the client's advertised capabilities
	answer     *mcp.ElicitResult       // if set, the client supports elicitation and answers every prompt with this
}

type toolResult struct {
	res       *mcp.CallToolResult
	err       error // the transport error, if the call itself broke
	configDir string

	// prompt is the elicitation the tool raised, as the client received it
	// after the JSON round trip (which is where the SDK fills in the mode).
	// Nil when the tool never prompted.
	prompt *mcp.ElicitParams
}

// runTool builds a real server over a mock API client and an isolated temp
// config directory, connects an in-memory MCP client, and calls tool with args
// through the SDK's actual request path. Returns the result, or the transport
// error if the call itself failed.
//
// Not t.Parallel-safe: it resets the process-wide mock keyring, sets
// TIGER_CONFIG_DIR, and options stub package-level vars.
func runTool(
	t *testing.T,
	tool string,
	args map[string]any,
	setupMock func(m *mocks.MockClientWithResponsesInterface),
	opts ...runOption,
) toolResult {
	t.Helper()

	rc := &runConfig{}
	for _, opt := range opts {
		opt(rc)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Give the run a fresh, empty in-memory keyring so a password a tool saves
	// never leaks into a later case that asserts none is stored. Options that
	// replace the keyring (withKeyringError) run after this.
	keyring.MockInit()

	// Run t-scoped setup hooks (see withSetup) in option order, before the
	// server is built.
	for _, f := range rc.setup {
		f(t)
	}

	// Create mock
	ctrl := gomock.NewController(t)
	mockClient := mocks.NewMockClientWithResponsesInterface(ctrl)
	if setupMock != nil {
		setupMock(mockClient)
	}

	// Analytics is always off and the docs proxy disabled, so a case neither
	// tracks events on the mock nor reaches out to the remote docs server.
	configDir := t.TempDir()
	values := map[string]any{"analytics": false, "docs_mcp": false}
	maps.Copy(values, rc.configValues)
	writeConfigFile(t, configDir, values)
	t.Setenv("TIGER_CONFIG_DIR", configDir)

	// Build the App and inject the mock
	app := &common.App{Experimental: rc.experimental}
	app.SetClientFactory(func(context.Context, *config.Config) (api.ClientWithResponsesInterface, string, error) {
		if rc.clientErr != nil {
			return nil, "", rc.clientErr
		}
		return mockClient, testProjectID, nil
	})
	if _, _, _, err := app.Load(ctx); err != nil {
		t.Fatalf("load app: %v", err)
	}

	server, err := NewServer(ctx, app, nil)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	// Registration has happened by now, so this rewrite reaches only the
	// per-request reload.
	if rc.configAfterStart != nil {
		maps.Copy(values, rc.configAfterStart)
		writeConfigFile(t, configDir, values)
	}

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	serverSession, err := server.mcpServer.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	// The SDK's default client capabilities don't include elicitation, so a
	// client with no handler is one that can't prompt.
	result := toolResult{configDir: configDir}
	clientOpts := &mcp.ClientOptions{Capabilities: rc.clientCaps}
	if rc.answer != nil {
		clientOpts.ElicitationHandler = func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			result.prompt = req.Params
			return rc.answer, nil
		}
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, clientOpts)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	// A handler's returned error travels as an IsError result, not a transport
	// error, so a non-nil err here means the call itself broke.
	result.res, result.err = clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      tool,
		Arguments: args,
	})
	return result
}

// withSetup runs f with the subtest's *testing.T just before the server is
// built. Use it from options that stub package-level vars, so their t.Cleanup
// restores at the end of the case that ran them rather than at the end of an
// outer test function whose t they'd otherwise have to capture.
func withSetup(f func(t *testing.T)) runOption {
	return func(rc *runConfig) {
		rc.setup = append(rc.setup, f)
	}
}

// withConfig seeds the config file the server starts with (e.g.
// map[string]any{"read_only": "prod"}), which also decides which tools are
// registered. Repeated withConfig options merge, later values winning per key.
func withConfig(values map[string]any) runOption {
	return func(rc *runConfig) {
		if rc.configValues == nil {
			rc.configValues = map[string]any{}
		}
		maps.Copy(rc.configValues, values)
	}
}

// withConfigAfterStart rewrites the config file once the server is running,
// for behavior driven by a config change the per-request reload picks up
// after tool registration has already happened (e.g. read_only=all refusing a
// call to a tool it would never have registered).
func withConfigAfterStart(values map[string]any) runOption {
	return func(rc *runConfig) {
		if rc.configAfterStart == nil {
			rc.configAfterStart = map[string]any{}
		}
		maps.Copy(rc.configAfterStart, values)
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

// withExperimental turns on the App's experimental gate before the server is
// built, so the preview-stage tools are registered.
func withExperimental() runOption {
	return func(rc *runConfig) {
		rc.experimental = true
	}
}

// withClientCapabilities overrides the capabilities the MCP client advertises
// (e.g. an elicitation capability the tool can't use).
func withClientCapabilities(caps *mcp.ClientCapabilities) runOption {
	return func(rc *runConfig) {
		rc.clientCaps = caps
	}
}

// withElicitation gives the MCP client elicitation support and makes it answer
// every prompt with answer. Assert the prompt itself with wantPrompt.
func withElicitation(answer *mcp.ElicitResult) runOption {
	return func(rc *runConfig) {
		rc.answer = answer
	}
}

// withKeyringError makes every keyring operation fail with err for the
// duration of the test, for exercising a tool's password storage failure
// path. The keyring is restored to an empty in-memory one afterwards.
func withKeyringError(err error) runOption {
	return withSetup(func(t *testing.T) {
		keyring.MockInitWithError(err)
		t.Cleanup(keyring.MockInit)
	})
}

// notLoggedInMsg is the message of the error withNotLoggedIn makes the client
// factory return; test cases expect it as their wantErr.
const notLoggedInMsg = "authentication required: not logged in. Run 'tiger auth login'"

// notLoggedInError mirrors the error common.NewAPIClient returns when no
// credentials are stored.
func notLoggedInError() error {
	return common.ExitWithCode(common.ExitAuthenticationError,
		fmt.Errorf("authentication required: %w. Run 'tiger auth login'", config.ErrNotLoggedIn))
}

// checkFunc is an extra assertion a toolTest runs after the standard ones.
type checkFunc func(t *testing.T, result toolResult)

// checkDefaultService returns a check asserting the config file's default
// service_id after the tool ran, with "" asserting it set none.
func checkDefaultService(want string) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		got, _ := readConfigFile(t, result.configDir)["service_id"].(string)
		if got != want {
			t.Errorf("default service_id = %q, want %q", got, want)
		}
	}
}

// writeConfigFile writes exactly the given keys to the config file in
// configDir. Only these keys are written, so unspecified ones still resolve
// from the environment or their defaults.
func writeConfigFile(t *testing.T, configDir string, values map[string]any) {
	t.Helper()
	contents, err := yaml.Marshal(values)
	if err != nil {
		t.Fatalf("failed to marshal config values: %v", err)
	}
	if err := os.WriteFile(config.GetConfigFile(configDir), contents, 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
}

// readConfigFile parses the config file persisted in configDir, so a check
// sees what the tool actually wrote rather than the resolved config's defaults.
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

// resultError returns the text of an IsError result, or "" for a success.
func resultError(res *mcp.CallToolResult) string {
	if !res.IsError {
		return ""
	}
	var texts []string
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// httpResponse creates a minimal *http.Response with the given status code.
// Its Body is nil — fine for the generated response structs' StatusCode()
// checks, but any code path that reads the body would panic.
func httpResponse(statusCode int) *http.Response {
	return &http.Response{StatusCode: statusCode}
}

// sampleService returns an api.Service with reasonable defaults. It carries no
// endpoint, so a tool that connects to it fails before opening a connection.
// Use overrides to customize specific fields.
func sampleService(overrides ...func(*api.Service)) api.Service {
	svc := api.Service{
		ServiceID:   "e6ue9697jf",
		ProjectID:   testProjectID,
		Name:        "test-service",
		ServiceType: api.ServiceTypeTIMESCALEDB,
		RegionCode:  "us-east-1",
		Status:      api.DeployStatusREADY,
	}
	for _, o := range overrides {
		o(&svc)
	}
	return svc
}

// sampleReplica returns a paused standby read replica of sampleService, shaped
// the way GetService returns it when given a read replica set ID. A replica
// connects to its own endpoint but borrows the parent primary's credentials,
// so resolving one fetches both services.
func sampleReplica(overrides ...func(*api.Service)) api.Service {
	svc := sampleService(func(s *api.Service) {
		s.ServiceID = "u8me885b93"
		s.Name = "replica-service"
		s.Status = api.DeployStatusPAUSED
		s.ForkedFrom = &api.ForkSpec{
			IsStandby: new(true),
			ProjectID: new(testProjectID),
			ServiceID: new("e6ue9697jf"),
		}
	})
	for _, o := range overrides {
		o(&svc)
	}
	return svc
}

// The readiness errors handleDatabaseError returns before a tool connects,
// and the error sampleService's missing endpoint produces once it would.
const (
	pausedMsg     = "service is paused — start it with the service_start tool"
	notReadyMsg   = "service is not ready — check its status with service_get and try again"
	noEndpointMsg = "failed to build connection string: service endpoint not available"
)

// expectGetService expects one GetService call for id, returning svc. The call
// is returned so a caller can adjust its cardinality (a wait loop's poll).
func expectGetService(m *mocks.MockClientWithResponsesInterface, id string, svc api.Service) *gomock.Call {
	return m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, id).
		Return(&api.GetServiceResponse{
			HTTPResponse: httpResponse(http.StatusOK),
			JSON200:      &svc,
		}, nil)
}

// expectTaggedService expects GetService to be called calls times, returning
// the sample service with the given environment tag each time.
func expectTaggedService(tag string, calls int) func(*mocks.MockClientWithResponsesInterface) {
	return func(m *mocks.MockClientWithResponsesInterface) {
		expectGetService(m, "e6ue9697jf", sampleService(func(s *api.Service) {
			s.Metadata = &api.ServiceMetadata{Environment: &tag}
		})).Times(calls)
	}
}

// validCtx is a gomock matcher that verifies a context.Context parameter is
// non-nil. Use this instead of gomock.Any() for context parameters.
var validCtx = gomock.Cond(func(x any) bool {
	ctx, ok := x.(context.Context)
	return ok && ctx != nil
})
