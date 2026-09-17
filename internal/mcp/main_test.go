package mcp

import (
	"context"
	"errors"
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

const testProjectID = "test-project-id"

func TestMain(m *testing.M) {
	// Backstop: replace the system keyring with an in-memory mock so that even a
	// test that forgets to reset can never read, write, or delete real
	// credentials or passwords. Per-test isolation comes from the fresh
	// keyring.MockInit() in runToolTest.
	keyring.MockInit()

	// Scrub inherited TIGER_* env vars: config.Load reads them through viper's
	// TIGER prefix, so a stray TIGER_READ_ONLY changes what these tests resolve.
	// Integration test credentials (TIGER_*_INTEGRATION) are preserved.
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "TIGER_") && !strings.HasSuffix(key, "_INTEGRATION") {
			os.Unsetenv(key)
		}
	}

	// Pin the local timezone to UTC so times the tools stamp with time.Now can
	// be asserted with plain literals. Done here, while the process is still
	// single-goroutine, since mutating time.Local later would race.
	time.Local = time.UTC

	os.Exit(m.Run())
}

// toolTest is one table-driven MCP tool test case, run by runToolTests. The
// tool is called through a real in-memory client/server session, so the case
// exercises the SDK's request path — middleware, schema validation, and the
// multi round-trip elicitation — not just the handler.
type toolTest struct {
	name string

	// tool and args are the call to make.
	tool string
	args map[string]any

	// config seeds the config file the server loads at startup. Analytics is
	// always off and the docs proxy disabled.
	config map[string]any
	// configAfterStart rewrites the config file once the server is running,
	// for behavior driven by a config change the per-request reload picks up
	// after tool registration has already happened.
	configAfterStart map[string]any

	// setupMock registers expectations on the mock API client.
	setupMock func(m *mocks.MockClientWithResponsesInterface)
	// clientErr, when set, is what the App's client factory returns, so the
	// handler sees a not-logged-in App.
	clientErr error

	// clientCaps overrides the MCP client's advertised capabilities. Left nil,
	// the client advertises elicitation only when answer is set.
	clientCaps *mcp.ClientCapabilities
	// wantPrompt is the elicitation the tool is expected to raise, as the
	// client receives it, and answer is the user's response to it. Left nil,
	// wantPrompt asserts no prompt is raised.
	wantPrompt *mcp.ElicitParams
	answer     *mcp.ElicitResult

	// wantErr is the exact text of the error result; empty asserts success.
	// wantOutput is the exact structured content; nil asserts none.
	wantErr    string
	wantOutput map[string]any
	// wantCallErr is the exact text of a transport error, which is how the SDK
	// reports a tool the server never registered — a handler's own error comes
	// back as a result, not as one of these. Set it and the result assertions
	// are skipped, since there is no result.
	wantCallErr string

	// checks are optional extra assertions, run in order after the standard ones.
	checks []toolCheckFunc

	// setup are t-scoped hooks run before the server is built. Use them to stub
	// a package-level var, so the t.Cleanup restoring it belongs to the case
	// that installed it rather than to the enclosing test function.
	setup []func(t *testing.T)

	// experimental turns on the App's experimental gate before the server is
	// built, so the preview-stage tools are registered for this case.
	experimental bool

	// synctest runs the case inside a [synctest] bubble, where time is
	// virtual: a poll interval or wait timeout elapses the instant every
	// goroutine is blocked on it. Set it for cases that wait on a timer (the
	// `wait` polling loop in common.WaitForService), so they assert against
	// realistic durations and finish instantly instead of sleeping.
	synctest bool
}

// runToolTests runs each case as a subtest.
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

func runToolTest(t *testing.T, tt toolTest) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Give the case a fresh, empty in-memory keyring so a password a tool saves
	// never leaks into a later case that asserts none is stored.
	keyring.MockInit()

	for _, setup := range tt.setup {
		setup(t)
	}

	configDir := t.TempDir()
	values := map[string]any{"analytics": false, "docs_mcp": false}
	maps.Copy(values, tt.config)
	writeConfigFile(t, configDir, values)
	t.Setenv("TIGER_CONFIG_DIR", configDir)

	mockClient := mocks.NewMockClientWithResponsesInterface(gomock.NewController(t))
	if tt.setupMock != nil {
		tt.setupMock(mockClient)
	}

	app := &common.App{Experimental: tt.experimental}
	app.SetClientFactory(func(context.Context, *config.Config) (api.ClientWithResponsesInterface, string, error) {
		if tt.clientErr != nil {
			return nil, "", tt.clientErr
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

	if tt.configAfterStart != nil {
		maps.Copy(values, tt.configAfterStart)
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
	clientOpts := &mcp.ClientOptions{Capabilities: tt.clientCaps}
	prompted := false
	if tt.answer != nil {
		clientOpts.ElicitationHandler = func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			prompted = true
			if tt.wantPrompt == nil {
				t.Errorf("unexpected prompt: %q", req.Params.Message)
			} else if diff := cmp.Diff(tt.wantPrompt, req.Params); diff != "" {
				t.Errorf("prompt mismatch (-want +got):\n%s", diff)
			}
			return tt.answer, nil
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
	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      tt.tool,
		Arguments: tt.args,
	})
	if tt.wantCallErr != "" {
		if err == nil {
			t.Fatalf("call %s: expected error %q, got none", tt.tool, tt.wantCallErr)
		}
		if got := err.Error(); got != tt.wantCallErr {
			t.Errorf("call error = %q, want %q", got, tt.wantCallErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("call %s: %v", tt.tool, err)
	}

	if tt.wantPrompt != nil && !prompted {
		t.Errorf("expected a prompt %q, got none", tt.wantPrompt.Message)
	}
	if got := resultError(res); got != tt.wantErr {
		t.Errorf("error = %q, want %q", got, tt.wantErr)
	}
	var wantStructured any
	if tt.wantOutput != nil {
		wantStructured = tt.wantOutput
	}
	if diff := cmp.Diff(wantStructured, res.StructuredContent); diff != "" {
		t.Errorf("structured content mismatch (-want +got):\n%s", diff)
	}

	for _, check := range tt.checks {
		check(t, configDir)
	}
}

// toolCheckFunc is an extra assertion a case can make after the standard ones,
// against state the tool left behind in its config directory.
type toolCheckFunc func(t *testing.T, configDir string)

// checkDefaultService returns a check asserting the service_id the tool wrote
// to the config file, with "" asserting it wrote none.
func checkDefaultService(want string) toolCheckFunc {
	return func(t *testing.T, configDir string) {
		t.Helper()
		got, _ := readConfigFile(t, configDir)["service_id"].(string)
		if got != want {
			t.Errorf("default service_id = %q, want %q", got, want)
		}
	}
}

// readConfigFile reads the config file back as raw keys, so a check sees what
// the tool actually wrote rather than the resolved config's defaults.
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

// writeConfigFile writes a config file with only the given keys, so everything
// else resolves from defaults.
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

// httpResponse creates an *http.Response with the given status code, for
// populating the generated response types.
func httpResponse(statusCode int) *http.Response {
	return &http.Response{StatusCode: statusCode}
}

// sampleService returns an api.Service with reasonable defaults. Use overrides
// to customize specific fields.
func sampleService(overrides ...func(*api.Service)) api.Service {
	svc := api.Service{
		ServiceID:   "e6ue9697jf",
		ProjectID:   testProjectID,
		Name:        "test-service",
		ServiceType: api.ServiceTypeTIMESCALEDB,
		RegionCode:  "us-east-1",
		Status:      api.DeployStatusREADY,
	}
	for _, override := range overrides {
		override(&svc)
	}
	return svc
}

// expectTaggedService expects GetService to be called calls times, returning
// the sample service with the given environment tag each time.
func expectTaggedService(tag string, calls int) func(*mocks.MockClientWithResponsesInterface) {
	return func(m *mocks.MockClientWithResponsesInterface) {
		svc := sampleService(func(s *api.Service) {
			s.Metadata = &api.ServiceMetadata{Environment: &tag}
		})
		m.EXPECT().GetServiceWithResponse(validCtx, testProjectID, "e6ue9697jf").
			Return(&api.GetServiceResponse{
				HTTPResponse: httpResponse(http.StatusOK),
				JSON200:      &svc,
			}, nil).
			Times(calls)
	}
}

// validCtx is a gomock matcher that verifies a context.Context parameter is
// non-nil. Use this instead of gomock.Any() for context parameters.
var validCtx = gomock.Cond(func(x any) bool {
	ctx, ok := x.(context.Context)
	return ok && ctx != nil
})

// errNotLoggedIn mirrors the error common.NewAPIClient returns when no
// credentials are stored.
var errNotLoggedIn = errors.New("authentication required: not logged in. Please run 'tiger auth login'")
