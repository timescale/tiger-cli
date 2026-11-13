package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func TestDocsProxy(t *testing.T) {
	everything := docsFeatures{tools: true, resources: true, prompts: true, completions: true}
	failing := func(methods ...string) *docsFeatures {
		features := everything
		features.fail = methods
		return &features
	}

	// What listOffered renders for each kind of the fake docs server's features.
	const (
		offersTools       = "tool:ask tool:search_docs"
		offersResources   = "resource:docs://guide resource:docs://intro"
		offersTemplates   = "template:docs://chapters/{chapter} template:docs://pages/{page}"
		offersPrompts     = "prompt:intro prompt:setup"
		offersCompletions = "completions"
	)
	offers := func(kinds ...string) string { return strings.Join(kinds, " ") }

	// The list requests Tiger MCP makes of a docs server that advertises
	// everything, as it starts.
	startup := []string{"tools/list", "resources/list", "resources/templates/list", "prompts/list"}
	afterStartup := func(methods ...string) []string { return append(slices.Clone(startup), methods...) }

	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()

	runProxyTests(t, []proxyTest{
		// Connecting
		{
			name:    "skips the proxy when docs MCP is disabled",
			docs:    &everything,
			config:  map[string]any{"docs_mcp": false},
			request: listOffered(),
		},
		{
			name:    "skips the proxy when there is no docs MCP URL",
			docs:    &everything,
			config:  map[string]any{"docs_mcp_url": ""},
			request: listOffered(),
		},
		{
			name:    "starts without docs features when the docs server is unreachable",
			docs:    &everything,
			config:  map[string]any{"docs_mcp_url": unreachable.URL},
			request: listOffered(),
		},

		// Registering what the docs server advertises
		{
			name:          "offers everything the docs server advertises",
			docs:          &everything,
			request:       listOffered(),
			want:          offers(offersTools, offersResources, offersTemplates, offersPrompts, offersCompletions),
			wantDocsCalls: startup,
		},
		{
			name:          "skips resources and completions the docs server doesn't advertise",
			docs:          &docsFeatures{tools: true, prompts: true},
			request:       listOffered(),
			want:          offers(offersTools, offersPrompts),
			wantDocsCalls: []string{"tools/list", "prompts/list"},
		},
		{
			name:          "skips prompts the docs server doesn't advertise",
			docs:          &docsFeatures{tools: true, resources: true},
			request:       listOffered(),
			want:          offers(offersTools, offersResources, offersTemplates),
			wantDocsCalls: []string{"tools/list", "resources/list", "resources/templates/list"},
		},
		{
			name:          "skips tools the docs server doesn't advertise",
			docs:          &docsFeatures{prompts: true},
			request:       listOffered(),
			want:          offers(offersPrompts),
			wantDocsCalls: []string{"prompts/list"},
		},
		{
			name:    "offers nothing when the docs server advertises no capabilities",
			docs:    &docsFeatures{tools: true, legacy: true, noCapabilities: true},
			request: listOffered(),
		},
		{
			name:    "offers everything from every page",
			docs:    &docsFeatures{tools: true, resources: true, prompts: true, pageSize: 1},
			request: listOffered(),
			want:    offers(offersTools, offersResources, offersTemplates, offersPrompts),
			wantDocsCalls: []string{
				"tools/list", "tools/list",
				"resources/list", "resources/list",
				"resources/templates/list", "resources/templates/list",
				"prompts/list", "prompts/list",
			},
		},
		{
			name:          "skips entries without a name or URI",
			docs:          &docsFeatures{tools: true, resources: true, prompts: true, unnamed: true},
			request:       listOffered(),
			want:          offers(offersTools, offersResources, offersTemplates, offersPrompts),
			wantDocsCalls: []string{"tools/list", "resources/list", "resources/templates/list", "prompts/list"},
		},
		{
			name:          "offers the rest when listing tools fails",
			docs:          failing("tools/list"),
			request:       listOffered(),
			want:          offers(offersResources, offersTemplates, offersPrompts, offersCompletions),
			wantDocsCalls: startup,
		},
		{
			name:          "offers the rest when listing resources fails",
			docs:          failing("resources/list"),
			request:       listOffered(),
			want:          offers(offersTools, offersTemplates, offersPrompts, offersCompletions),
			wantDocsCalls: startup,
		},
		{
			name:          "offers the rest when listing resource templates fails",
			docs:          failing("resources/templates/list"),
			request:       listOffered(),
			want:          offers(offersTools, offersResources, offersPrompts, offersCompletions),
			wantDocsCalls: startup,
		},
		{
			name:          "offers the rest when listing prompts fails",
			docs:          failing("prompts/list"),
			request:       listOffered(),
			want:          offers(offersTools, offersResources, offersTemplates, offersCompletions),
			wantDocsCalls: startup,
		},

		// Forwarding requests
		{
			name:          "forwards tool calls",
			docs:          &everything,
			request:       callTool("search_docs", nil),
			want:          "search results",
			wantDocsCalls: afterStartup("tools/call"),
		},
		{
			name:          "forwards prompt requests",
			docs:          &everything,
			request:       getPrompt("setup"),
			want:          "setup prompt",
			wantDocsCalls: afterStartup("prompts/get"),
		},
		{
			name:          "forwards resource reads",
			docs:          &everything,
			request:       readResource("docs://guide"),
			want:          "contents of docs://guide",
			wantDocsCalls: afterStartup("resources/read"),
		},
		{
			name:          "forwards resource template reads",
			docs:          &everything,
			request:       readResource("docs://pages/intro"),
			want:          "contents of docs://pages/intro",
			wantDocsCalls: afterStartup("resources/read"),
		},
		{
			name:          "forwards completion requests",
			docs:          &everything,
			request:       complete("setup", "topic", "hyper"),
			want:          "hypertables",
			wantDocsCalls: afterStartup("completion/complete"),
		},
		{
			name:          "reports the docs server's tool call errors",
			docs:          failing("tools/call"),
			request:       callTool("search_docs", nil),
			wantErr:       `calling "tools/call": remote tool call failed: calling "tools/call": docs server failure`,
			wantDocsCalls: afterStartup("tools/call"),
		},
		{
			name:          "reports the docs server's prompt request errors",
			docs:          failing("prompts/get"),
			request:       getPrompt("setup"),
			wantErr:       `calling "prompts/get": remote prompt request failed: calling "prompts/get": docs server failure`,
			wantDocsCalls: afterStartup("prompts/get"),
		},
		{
			name:          "reports the docs server's resource read errors",
			docs:          failing("resources/read"),
			request:       readResource("docs://guide"),
			wantErr:       `calling "resources/read": remote resource read failed: calling "resources/read": docs server failure`,
			wantDocsCalls: afterStartup("resources/read"),
		},
		{
			name:          "reports the docs server's completion errors",
			docs:          failing("completion/complete"),
			request:       complete("setup", "topic", "hyper"),
			wantErr:       `calling "completion/complete": remote completion failed: calling "completion/complete": docs server failure`,
			wantDocsCalls: afterStartup("completion/complete"),
		},
		{
			name:                 "calls tools the same way for agents on older protocol versions",
			docs:                 &everything,
			agentProtocolVersion: "2025-11-25",
			request:              callTool("search_docs", nil),
			want:                 "search results",
			wantDocsCalls:        afterStartup("tools/call"),
		},
		{
			name:          "calls tools on a docs server that only speaks older protocol versions",
			docs:          &docsFeatures{tools: true, legacy: true},
			request:       callTool("search_docs", nil),
			want:          "search results",
			wantDocsCalls: []string{"tools/list", "tools/call"},
		},
		{
			name:          "passes the docs server's input requests through to the agent",
			docs:          &everything,
			answer:        &mcp.ElicitResult{Action: "accept", Content: map[string]any{"color": "blue"}},
			request:       callTool("ask", nil),
			want:          "color=blue request_state=asked",
			wantDocsCalls: afterStartup("tools/call", "tools/call"),
		},
	})
}

// TestDocsProxyIntegration runs the docs proxy against the production docs MCP
// server (pg-aiguide), for agents on the latest and the previous protocol
// version, to catch an incompatibility as soon as either side changes. It
// asserts only what stays stable as the docs change: that each request returns
// content.
func TestDocsProxyIntegration(t *testing.T) {
	requireIntegration(t)

	// Keyword-only search, so the test doesn't depend on the docs server's
	// embeddings provider.
	search := callTool("search_docs", map[string]any{"source": "tiger", "query": "hypertable", "limit": 3, "semanticWeight": 0})
	// A skill name of "." lists the skills, which stays valid as they change.
	viewSkills := callTool("view_skill", map[string]any{"skill_name": ".", "path": ""})

	var tests []proxyTest
	for _, version := range []string{"2026-07-28", "2025-11-25"} {
		// The live docs change, so only that there's content can be asserted.
		tests = append(tests,
			proxyTest{name: "searches the docs, agent on " + version, agentProtocolVersion: version, request: search, want: matchNonEmpty()},
			proxyTest{name: "views the skills, agent on " + version, agentProtocolVersion: version, request: viewSkills, want: matchNonEmpty()},
			proxyTest{name: "gets a prompt, agent on " + version, agentProtocolVersion: version, request: getFirstPrompt(), want: matchNonEmpty()},
		)
	}
	runProxyTests(t, tests)
}

// proxyTest is a test case for the docs proxy: a fake docs MCP server for
// Tiger MCP to proxy, a request an agent makes of Tiger MCP, and what the agent
// and the docs server should see.
type proxyTest struct {
	name string

	// docs is the fake docs server Tiger MCP proxies. Left nil, Tiger MCP
	// proxies the production docs server instead, which only the integration
	// test does.
	docs *docsFeatures
	// config is merged over the config Tiger MCP starts with, which enables
	// the docs proxy and points it at docs.
	config map[string]any

	// agentProtocolVersion is the protocol version the agent asks Tiger MCP
	// for; the SDK's latest if empty.
	agentProtocolVersion string
	// answer, if set, makes the agent support elicitation and answer every
	// prompt with it.
	answer *mcp.ElicitResult

	request agentRequest

	// want is the response as request renders it, or a matchText for a live
	// server's, and wantErr the error it returns instead. Left unset, they
	// assert an empty response and no error respectively.
	want    any
	wantErr string
	// wantDocsCalls lists the methods the docs server received after the
	// handshake, in order. The runner also asserts that each came from the
	// proxy as a client in its own right.
	wantDocsCalls []string
}

// runProxyTests runs a slice of docs proxy test cases.
func runProxyTests(t *testing.T, tests []proxyTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { runProxyTest(t, tt) })
	}
}

// runProxyTest serves tt.docs over streamable HTTP, builds a real Tiger MCP
// server proxying it, connects an in-memory agent, makes tt.request, and
// asserts what the agent and the docs server saw. It closes Tiger MCP
// afterwards, asserting that closes cleanly too.
//
// Not t.Parallel-safe: it resets the process-wide mock keyring and sets
// TIGER_CONFIG_DIR.
func runProxyTest(t *testing.T, tt proxyTest) {
	t.Helper()
	ctx := t.Context()
	keyring.MockInit()

	values := map[string]any{"analytics": false, "docs_mcp": true}
	var docs *docsServer
	if tt.docs != nil {
		docs = startDocsServer(t, *tt.docs)
		values["docs_mcp_url"] = docs.url
	}
	maps.Copy(values, tt.config)
	configDir := t.TempDir()
	writeConfigFile(t, configDir, values)
	t.Setenv("TIGER_CONFIG_DIR", configDir)

	app := &common.App{}
	app.SetClientFactory(func(context.Context, *config.Config) (api.ClientWithResponsesInterface, string, error) {
		return nil, "", errors.New("the docs proxy uses no API client")
	})
	if _, _, _, err := app.Load(ctx); err != nil {
		t.Fatalf("load app: %v", err)
	}
	server, err := NewServer(ctx, app, nil, TransportStdio)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.mcpServer.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	clientOpts := &mcp.ClientOptions{}
	if tt.answer != nil {
		clientOpts.ElicitationHandler = func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return tt.answer, nil
		}
	}
	agent := mcp.NewClient(&mcp.Implementation{Name: "test-agent"}, clientOpts)
	session, err := agent.Connect(ctx, clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: tt.agentProtocolVersion})
	if err != nil {
		t.Fatalf("agent connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	got, err := tt.request(ctx, session)
	if tt.wantErr != "" {
		if err == nil {
			t.Errorf("expected error %q, got response %q", tt.wantErr, got)
		} else if err.Error() != tt.wantErr {
			t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
		}
	} else if err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if match, ok := tt.want.(matchText); ok {
		match(t, got)
	} else {
		want, _ := tt.want.(string)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("response mismatch (-want +got):\n%s", diff)
		}
	}

	if docs != nil {
		docs.assertCalls(t, tt.wantDocsCalls)
	}

	if err := server.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}

// matchText asserts a response that can't be matched exactly.
type matchText func(t *testing.T, got string)

// matchNonEmpty asserts the response isn't empty.
func matchNonEmpty() matchText {
	return func(t *testing.T, got string) {
		t.Helper()
		if got == "" {
			t.Error("response is empty, want content")
		}
	}
}

// agentRequest is a request an agent makes of Tiger MCP. It returns the
// response rendered as text, or the error the request returned.
type agentRequest func(ctx context.Context, session *mcp.ClientSession) (string, error)

// callTool calls the named tool with args, rendering the text content of the
// result.
func callTool(name string, args map[string]any) agentRequest {
	return func(ctx context.Context, session *mcp.ClientSession) (string, error) {
		if args == nil {
			args = map[string]any{}
		}
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			return "", err
		}
		var texts []string
		for _, content := range res.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				texts = append(texts, text.Text)
			}
		}
		if res.IsError {
			return "", fmt.Errorf("tool error: %s", strings.Join(texts, "\n"))
		}
		return strings.Join(texts, "\n"), nil
	}
}

// getPrompt gets the named prompt, rendering the text of its messages.
func getPrompt(name string) agentRequest {
	return func(ctx context.Context, session *mcp.ClientSession) (string, error) {
		res, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: name})
		if err != nil {
			return "", err
		}
		var texts []string
		for _, message := range res.Messages {
			if text, ok := message.Content.(*mcp.TextContent); ok {
				texts = append(texts, text.Text)
			}
		}
		return strings.Join(texts, "\n"), nil
	}
}

// getFirstPrompt gets the first prompt the agent is offered, for a server
// whose prompts aren't known in advance.
func getFirstPrompt() agentRequest {
	return func(ctx context.Context, session *mcp.ClientSession) (string, error) {
		for prompt, err := range session.Prompts(ctx, nil) {
			if err != nil {
				return "", err
			}
			return getPrompt(prompt.Name)(ctx, session)
		}
		return "", errors.New("no prompts offered")
	}
}

// readResource reads the resource at uri, rendering the text of its contents.
func readResource(uri string) agentRequest {
	return func(ctx context.Context, session *mcp.ClientSession) (string, error) {
		res, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			return "", err
		}
		var texts []string
		for _, contents := range res.Contents {
			texts = append(texts, contents.Text)
		}
		return strings.Join(texts, "\n"), nil
	}
}

// complete completes the value of the named prompt's argument, rendering the
// completions.
func complete(prompt, argument, value string) agentRequest {
	return func(ctx context.Context, session *mcp.ClientSession) (string, error) {
		res, err := session.Complete(ctx, &mcp.CompleteParams{
			Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: prompt},
			Argument: mcp.CompleteParamsArgument{Name: argument, Value: value},
		})
		if err != nil {
			return "", err
		}
		return strings.Join(res.Completion.Values, " "), nil
	}
}

// listOffered lists the docs features Tiger MCP offers the agent: tools,
// resources, resource templates, and prompts, each sorted and prefixed with its
// kind, then "completions" if it offers those.
func listOffered() agentRequest {
	return func(ctx context.Context, session *mcp.ClientSession) (string, error) {
		var tools, resources, templates, prompts []string
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				return "", err
			}
			if tool.Title == docsTitle {
				tools = append(tools, "tool:"+tool.Name)
			}
		}
		for resource, err := range session.Resources(ctx, nil) {
			if err != nil {
				return "", err
			}
			resources = append(resources, "resource:"+resource.URI)
		}
		for template, err := range session.ResourceTemplates(ctx, nil) {
			if err != nil {
				return "", err
			}
			templates = append(templates, "template:"+template.URITemplate)
		}
		for prompt, err := range session.Prompts(ctx, nil) {
			if err != nil {
				return "", err
			}
			prompts = append(prompts, "prompt:"+prompt.Name)
		}
		var offered []string
		for _, names := range [][]string{tools, resources, templates, prompts} {
			slices.Sort(names)
			offered = append(offered, names...)
		}
		if session.InitializeResult().Capabilities.Completions != nil {
			offered = append(offered, "completions")
		}
		return strings.Join(offered, " "), nil
	}
}

// docsFeatures picks what a fake docs MCP server exposes, and therefore which
// capabilities it advertises: two each of tools, resources, resource templates
// (with resources), and prompts, plus completions. The ask tool asks the
// client for input before it answers.
//
// The rest change how the server behaves. A nonzero pageSize splits its lists
// into pages. legacy serves only protocol versions before 2026-07-28, as
// pg-aiguide did until it upgraded its SDK, and noCapabilities makes such a
// server advertise none. unnamed adds an entry of each kind with no name or
// URI. fail lists methods the server fails with errDocsFailure.
type docsFeatures struct {
	tools          bool
	resources      bool
	prompts        bool
	completions    bool
	pageSize       int
	legacy         bool
	noCapabilities bool
	unnamed        bool
	fail           []string
}

// errDocsFailure is the error a fake docs server fails its docsFeatures.fail
// methods with.
var errDocsFailure = errors.New("docs server failure")

// docsTitle marks the tools a fake docs server exposes, to tell them apart
// from Tiger MCP's own.
const docsTitle = "Docs"

// docsServer is a fake docs MCP server, served over streamable HTTP, that
// records the requests it receives.
type docsServer struct {
	url    string
	legacy bool

	mu    sync.Mutex
	calls []docsCall
}

// docsCall is a request a fake docs server received: its method, and the
// client the server saw making it.
type docsCall struct {
	method string
	client string
}

// startDocsServer starts a fake docs server with the given features, stopping
// it when the test ends. The SDK serves protocol versions from 2026-07-28 on
// only from a stateless server, so a legacy docs server is a stateful one.
func startDocsServer(t *testing.T, features docsFeatures) *docsServer {
	t.Helper()
	d := &docsServer{legacy: features.legacy}

	opts := &mcp.ServerOptions{PageSize: features.pageSize}
	if features.completions {
		opts.CompletionHandler = func(_ context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{req.Params.Argument.Value + "tables"}}}, nil
		}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "docs"}, opts)

	if features.tools {
		search := func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "search results"}}}, nil
		}
		ask := func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			answer, ok := req.Params.InputResponses["color"].(*mcp.ElicitResult)
			if !ok {
				return &mcp.CallToolResult{
					InputRequests: mcp.InputRequestMap{"color": &mcp.ElicitParams{
						Message: "Favorite color?",
						RequestedSchema: map[string]any{
							"type":       "object",
							"properties": map[string]any{"color": map[string]any{"type": "string"}},
						},
					}},
					RequestState: "asked",
				}, nil
			}
			text := fmt.Sprintf("color=%v request_state=%s", answer.Content["color"], req.Params.RequestState)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
		}
		tools := map[string]mcp.ToolHandler{"search_docs": search, "ask": ask}
		if features.unnamed {
			tools[""] = search
		}
		for name, handler := range tools {
			server.AddTool(&mcp.Tool{Name: name, Title: docsTitle, InputSchema: map[string]any{"type": "object"}}, handler)
		}
	}

	if features.resources {
		read := func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "contents of " + req.Params.URI}}}, nil
		}
		resources := []string{"docs://guide", "docs://intro"}
		templates := []string{"docs://pages/{page}", "docs://chapters/{chapter}"}
		if features.unnamed {
			resources = append(resources, "")
			templates = append(templates, "")
		}
		for _, uri := range resources {
			server.AddResource(&mcp.Resource{Name: uri, URI: uri}, read)
		}
		for _, uriTemplate := range templates {
			server.AddResourceTemplate(&mcp.ResourceTemplate{Name: uriTemplate, URITemplate: uriTemplate}, read)
		}
	}

	if features.prompts {
		get := func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{
				Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: req.Params.Name + " prompt"}}},
			}, nil
		}
		prompts := []string{"setup", "intro"}
		if features.unnamed {
			prompts = append(prompts, "")
		}
		for _, name := range prompts {
			server.AddPrompt(&mcp.Prompt{Name: name}, get)
		}
	}

	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			d.record(method, req)
			if slices.Contains(features.fail, method) {
				return nil, errDocsFailure
			}
			res, err := next(ctx, method, req)
			if init, ok := res.(*mcp.InitializeResult); ok && features.noCapabilities {
				init.Capabilities = nil
			}
			return res, err
		}
	})

	srv := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: !features.legacy}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	d.url = srv.URL
	return d
}

// handshakeMethods are the requests that establish a session, which a fake
// docs server doesn't record.
var handshakeMethods = []string{"initialize", "notifications/initialized", "server/discover"}

// record records a request, with the client making it.
func (d *docsServer) record(method string, req mcp.Request) {
	if slices.Contains(handshakeMethods, method) {
		return
	}
	var client string
	if session, ok := req.GetSession().(*mcp.ServerSession); ok {
		if params := session.InitializeParams(); params != nil {
			client = describeClient(params)
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, docsCall{method: method, client: client})
}

// describeClient describes the client making a request: its name, its
// protocol version, and how its capabilities differ from none at all.
func describeClient(params *mcp.InitializeParams) string {
	var name string
	if params.ClientInfo != nil {
		name = params.ClientInfo.Name
	}
	return fmt.Sprintf("client=%s protocol_version=%s capabilities_diff=%q",
		name, params.ProtocolVersion, cmp.Diff(&mcp.ClientCapabilities{}, params.Capabilities))
}

// assertCalls asserts the methods the docs server received, in order, and
// that each came from the proxy as a client in its own right: never as the
// agent, at the protocol version it negotiated with the docs server, and
// advertising no capabilities.
func (d *docsServer) assertCalls(t *testing.T, want []string) {
	t.Helper()
	protocolVersion := "2026-07-28"
	if d.legacy {
		protocolVersion = "2025-11-25"
	}
	proxy := describeClient(&mcp.InitializeParams{
		ClientInfo:      &mcp.Implementation{Name: "tiger-mcp-proxy-client"},
		ProtocolVersion: protocolVersion,
		Capabilities:    &mcp.ClientCapabilities{},
	})

	d.mu.Lock()
	defer d.mu.Unlock()
	var methods []string
	for _, call := range d.calls {
		methods = append(methods, call.method)
		if call.client != proxy {
			t.Errorf("docs server received %s from %s, want %s", call.method, call.client, proxy)
		}
	}
	if diff := cmp.Diff(want, methods); diff != "" {
		t.Errorf("docs server calls mismatch (-want +got):\n%s", diff)
	}
}
