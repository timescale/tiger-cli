package mcp

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"os"
	"strings"
	"testing"

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
	// Backstop: replace the system keyring with an in-memory mock so no test can
	// read, write, or delete real credentials or passwords.
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

	// experimental gates preview-stage tools at registration (see CLAUDE.md's
	// "Experimental Feature Gating"). A tool behind the gate is never added to
	// the server unless this is true.
	experimental bool

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

	// withProgressToken opts the call into sending a progress token, so the
	// handler's NotifyProgress calls (if any) actually go out on the wire.
	// wantProgress is the exact, ordered list of progress notification
	// messages expected; nil asserts none were sent.
	withProgressToken bool
	wantProgress      []string
}

// runToolTests runs each case as a subtest.
func runToolTests(t *testing.T, tests []toolTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runToolTest(t, tt)
		})
	}
}

func runToolTest(t *testing.T, tt toolTest) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

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
	var gotProgress []string
	clientOpts.ProgressNotificationHandler = func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
		gotProgress = append(gotProgress, req.Params.Message)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, clientOpts)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	params := &mcp.CallToolParams{
		Name:      tt.tool,
		Arguments: tt.args,
	}
	if tt.withProgressToken {
		params.SetProgressToken("test-token")
	}

	// A handler's returned error travels as an IsError result, not a transport
	// error, so a non-nil err here means the call itself broke.
	res, err := clientSession.CallTool(ctx, params)
	if err != nil {
		t.Fatalf("call %s: %v", tt.tool, err)
	}

	if tt.wantPrompt != nil && !prompted {
		t.Errorf("expected a prompt %q, got none", tt.wantPrompt.Message)
	}
	if diff := cmp.Diff(tt.wantProgress, gotProgress); diff != "" {
		t.Errorf("progress messages mismatch (-want +got):\n%s", diff)
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
