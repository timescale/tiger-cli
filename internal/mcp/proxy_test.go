package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
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
	// The protocol versions before 2026-07-28, newest first, all that pg-aiguide
	// supported until it upgraded its SDK.
	olderVersions := []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}
	failing := func(methods ...string) *docsFeatures {
		features := everything
		features.fail = methods
		return &features
	}

	// What listOffered returns for each kind of the fake docs server's features.
	offersTools := []string{"tool:ask", "tool:search_docs"}
	offersResources := []string{"resource:docs://guide", "resource:docs://intro"}
	offersTemplates := []string{"template:docs://chapters/{chapter}", "template:docs://pages/{page}"}
	offersPrompts := []string{"prompt:intro", "prompt:setup"}
	offersCompletions := []string{"completions"}
	offers := slices.Concat[[]string]

	// The _meta Tiger MCP puts on a result for an agent on 2026-07-28, in place
	// of the docs server's. An agent on an older version gets none.
	tigerMeta := mcp.Meta{mcp.MetaKeyServerInfo: &mcp.Implementation{Name: ServerName, Title: serverTitle, Version: config.Version}}

	searchDocs := callTool("search_docs", map[string]any{"query": "hypertables", "limit": 3})
	setupPrompt := getPrompt("setup", map[string]string{"topic": "hypertables"})
	completeTopic := complete(&mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: "setup"},
		Argument: mcp.CompleteParamsArgument{Name: "topic", Value: "hyper"},
		Context:  &mcp.CompleteContext{Arguments: map[string]string{"service": "db"}},
	})

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
			name:          "skips tools the docs server doesn't advertise",
			docs:          &docsFeatures{resources: true, prompts: true, completions: true},
			request:       listOffered(),
			want:          offers(offersResources, offersTemplates, offersPrompts, offersCompletions),
			wantDocsCalls: []string{"resources/list", "resources/templates/list", "prompts/list"},
		},
		{
			name:          "skips resources the docs server doesn't advertise",
			docs:          &docsFeatures{tools: true, prompts: true, completions: true},
			request:       listOffered(),
			want:          offers(offersTools, offersPrompts, offersCompletions),
			wantDocsCalls: []string{"tools/list", "prompts/list"},
		},
		{
			name:          "skips prompts the docs server doesn't advertise",
			docs:          &docsFeatures{tools: true, resources: true, completions: true},
			request:       listOffered(),
			want:          offers(offersTools, offersResources, offersTemplates, offersCompletions),
			wantDocsCalls: []string{"tools/list", "resources/list", "resources/templates/list"},
		},
		{
			name:          "skips completions the docs server doesn't advertise",
			docs:          &docsFeatures{tools: true, resources: true, prompts: true},
			request:       listOffered(),
			want:          offers(offersTools, offersResources, offersTemplates, offersPrompts),
			wantDocsCalls: startup,
		},
		{
			name:    "offers nothing when the docs server advertises no capabilities",
			docs:    &docsFeatures{tools: true, protocolVersions: olderVersions, noCapabilities: true},
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
			request:       searchDocs,
			want:          withMeta(docsToolResult(), tigerMeta),
			wantDocsCalls: afterStartup("tools/call"),
		},
		{
			name:          "forwards prompt requests",
			docs:          &everything,
			request:       setupPrompt,
			want:          withMeta(docsPromptResult(), tigerMeta),
			wantDocsCalls: afterStartup("prompts/get"),
		},
		{
			name:          "forwards resource reads",
			docs:          &everything,
			request:       readResource("docs://guide"),
			want:          withMeta(docsResourceResult(), tigerMeta),
			wantDocsCalls: afterStartup("resources/read"),
		},
		{
			name:          "forwards resource template reads",
			docs:          &everything,
			request:       readResource("docs://pages/intro"),
			want:          withMeta(docsResourceResult(), tigerMeta),
			wantDocsCalls: afterStartup("resources/read"),
		},
		{
			name:          "forwards completion requests",
			docs:          &everything,
			request:       completeTopic,
			want:          withMeta(docsCompletionResult(), tigerMeta),
			wantDocsCalls: afterStartup("completion/complete"),
		},
		{
			name:          "reports the docs server's tool call errors",
			docs:          failing("tools/call"),
			request:       searchDocs,
			wantErr:       `calling "tools/call": remote tool call failed: calling "tools/call": docs server failure`,
			wantDocsCalls: afterStartup("tools/call"),
		},
		{
			name:          "reports the docs server's prompt request errors",
			docs:          failing("prompts/get"),
			request:       setupPrompt,
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
			request:       completeTopic,
			wantErr:       `calling "completion/complete": remote completion failed: calling "completion/complete": docs server failure`,
			wantDocsCalls: afterStartup("completion/complete"),
		},
		{
			name:                 "calls tools the same way for agents on older protocol versions",
			docs:                 &everything,
			agentProtocolVersion: "2025-11-25",
			request:              searchDocs,
			want:                 withMeta(docsToolResult(), nil),
			wantDocsCalls:        afterStartup("tools/call"),
		},
		{
			name:          "calls tools on a docs server that only speaks older protocol versions",
			docs:          &docsFeatures{tools: true, protocolVersions: olderVersions},
			request:       searchDocs,
			want:          withMeta(docsToolResult(), tigerMeta),
			wantDocsCalls: []string{"tools/list", "tools/call"},
		},
		{
			name:    "passes the docs server's input requests through to the agent",
			docs:    &everything,
			answer:  &mcp.ElicitResult{Action: "accept", Content: map[string]any{"color": "blue"}},
			request: callTool("ask", nil),
			want: &mcp.CallToolResult{
				Meta:              tigerMeta,
				Content:           []mcp.Content{},
				StructuredContent: map[string]any{"color": "blue", "request_state": "asked"},
			},
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
			proxyTest{
				name:                 "searches the docs, agent on " + version,
				agentProtocolVersion: version,
				request:              search,
				want:                 matchNonEmpty("content"),
			},
			proxyTest{
				name:                 "views the skills, agent on " + version,
				agentProtocolVersion: version,
				request:              viewSkills,
				want:                 matchNonEmpty("content"),
			},
			proxyTest{
				name:                 "gets a prompt, agent on " + version,
				agentProtocolVersion: version,
				request:              getFirstPrompt(),
				want:                 matchNonEmpty("messages"),
			},
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

	// want is the response, compared as JSON, as the agent would see it on
	// the wire, or a matchResult for a live server's. wantErr is the error the
	// request returns instead. Left unset, they assert an empty response and
	// no error respectively.
	want    any
	wantErr string
	// wantDocsCalls lists the methods the docs server received after the
	// handshake, in order. The runner also asserts that every client the docs
	// server saw was the proxy as a client in its own right, and that it
	// received the request's params unchanged apart from _meta.
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

	got, err := tt.request.send(ctx, session)
	if tt.wantErr != "" {
		if err == nil {
			t.Errorf("expected error %q, got none", tt.wantErr)
		} else if err.Error() != tt.wantErr {
			t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
		}
	} else if err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if match, ok := tt.want.(matchResult); ok {
		match(t, wireJSON(t, got))
	} else if diff := cmp.Diff(wireJSON(t, tt.want), wireJSON(t, got)); diff != "" {
		t.Errorf("response mismatch (-want +got):\n%s", diff)
	}

	if docs != nil {
		docs.assertCalls(t, tt.wantDocsCalls, tt.request.params)
	}

	if err := server.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}

// wireJSON returns v as decoded from its JSON encoding, as it travels on the
// wire, leaving out resultType: the SDK sets that on every result it decodes,
// to record how it was delivered.

func wireJSON(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encode %T: %v", v, err)
	}
	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("decode %T: %v", v, err)
	}
	if m, ok := decoded.(map[string]any); ok {
		delete(m, "resultType")
	}
	return decoded
}

// matchResult asserts a response that can't be matched exactly, given as
// decoded from its JSON encoding.
type matchResult func(t *testing.T, got any)

// matchNonEmpty asserts a response whose field holds a non-empty list, and
// that isn't a tool error.
func matchNonEmpty(field string) matchResult {
	return func(t *testing.T, got any) {
		t.Helper()
		m, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("response = %v, want an object", got)
		}
		if isError, _ := m["isError"].(bool); isError {
			t.Errorf("response is a tool error: %v", m)
		}
		if list, _ := m[field].([]any); len(list) == 0 {
			t.Errorf("response %s = %v, want a non-empty list", field, m[field])
		}
	}
}

// agentRequest is a request an agent makes of Tiger MCP.
type agentRequest struct {
	// params is what the agent sends, for a request Tiger MCP forwards to the
	// docs server. Nil for a request Tiger MCP answers itself.
	params mcp.Params
	// send makes the request, returning the response.
	send func(ctx context.Context, session *mcp.ClientSession) (any, error)
}

// callTool calls the named tool with args.
func callTool(name string, args map[string]any) agentRequest {
	params := &mcp.CallToolParams{Name: name, Arguments: args}
	return agentRequest{params: params, send: func(ctx context.Context, session *mcp.ClientSession) (any, error) {
		// The SDK adds _meta to the params it sends, so send a copy.
		params := *params
		return session.CallTool(ctx, &params)
	}}
}

// getPrompt gets the named prompt with args.
func getPrompt(name string, args map[string]string) agentRequest {
	params := &mcp.GetPromptParams{Name: name, Arguments: args}
	return agentRequest{params: params, send: func(ctx context.Context, session *mcp.ClientSession) (any, error) {
		params := *params
		return session.GetPrompt(ctx, &params)
	}}
}

// readResource reads the resource at uri.
func readResource(uri string) agentRequest {
	params := &mcp.ReadResourceParams{URI: uri}
	return agentRequest{params: params, send: func(ctx context.Context, session *mcp.ClientSession) (any, error) {
		params := *params
		return session.ReadResource(ctx, &params)
	}}
}

// complete requests completions for params.
func complete(params *mcp.CompleteParams) agentRequest {
	return agentRequest{params: params, send: func(ctx context.Context, session *mcp.ClientSession) (any, error) {
		params := *params
		return session.Complete(ctx, &params)
	}}
}

// getFirstPrompt gets the first prompt the agent is offered, for a server
// whose prompts aren't known in advance.
func getFirstPrompt() agentRequest {
	return agentRequest{send: func(ctx context.Context, session *mcp.ClientSession) (any, error) {
		for prompt, err := range session.Prompts(ctx, nil) {
			if err != nil {
				return nil, err
			}
			return session.GetPrompt(ctx, &mcp.GetPromptParams{Name: prompt.Name})
		}
		return nil, errors.New("no prompts offered")
	}}
}

// listOffered lists the docs features Tiger MCP offers the agent: tools,
// resources, resource templates, and prompts, each sorted and prefixed with its
// kind, then "completions" if it offers those.
func listOffered() agentRequest {
	return agentRequest{send: func(ctx context.Context, session *mcp.ClientSession) (any, error) {
		var tools, resources, templates, prompts []string
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				return nil, err
			}
			if tool.Title == docsTitle {
				tools = append(tools, "tool:"+tool.Name)
			}
		}
		for resource, err := range session.Resources(ctx, nil) {
			if err != nil {
				return nil, err
			}
			resources = append(resources, "resource:"+resource.URI)
		}
		for template, err := range session.ResourceTemplates(ctx, nil) {
			if err != nil {
				return nil, err
			}
			templates = append(templates, "template:"+template.URITemplate)
		}
		for prompt, err := range session.Prompts(ctx, nil) {
			if err != nil {
				return nil, err
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
		return offered, nil
	}}
}

// docsFeatures picks what a fake docs MCP server exposes, and therefore which
// capabilities it advertises: two each of tools, resources, resource templates
// (with resources), and prompts, plus completions. Each returns the same
// fixed result whatever it's asked (docsToolResult and the like), except the
// ask tool, which asks the client for input before it answers.
//
// The rest change how the server behaves. A nonzero pageSize splits its lists
// into pages. protocolVersions, newest first, limits the protocol versions the
// server supports, which are the SDK's when empty. noCapabilities makes it
// advertise none, which only a server limited to versions before 2026-07-28
// can do. unnamed adds an entry of each kind with no name or URI. fail lists
// methods the server fails with errDocsFailure.
type docsFeatures struct {
	tools            bool
	resources        bool
	prompts          bool
	completions      bool
	pageSize         int
	protocolVersions []string
	noCapabilities   bool
	unnamed          bool
	fail             []string
}

// errDocsFailure is the error a fake docs server fails its docsFeatures.fail
// methods with.
var errDocsFailure = errors.New("docs server failure")

// docsTitle marks the tools a fake docs server exposes, to tell them apart
// from Tiger MCP's own.
const docsTitle = "Docs"

// docsToolResult is the result a fake docs server's tools return, with every
// kind of content, structured content, and _meta. The proxy should pass it
// through unchanged apart from the _meta (see withMeta).
func docsToolResult() *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Meta: mcp.Meta{"docs/source": "fake"},
		Content: []mcp.Content{
			&mcp.TextContent{Text: "hypertables", Annotations: &mcp.Annotations{Audience: []mcp.Role{"user"}, Priority: 0.5}},
			&mcp.ImageContent{Data: []byte("png"), MIMEType: "image/png"},
			&mcp.AudioContent{Data: []byte("wav"), MIMEType: "audio/wav"},
			&mcp.ResourceLink{URI: "docs://guide", Name: "guide", MIMEType: "text/markdown"},
			&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "docs://intro", MIMEType: "text/markdown", Text: "intro"}},
		},
		StructuredContent: map[string]any{"results": []any{"hypertables"}},
	}
}

// docsPromptResult is the result a fake docs server's prompts return.
func docsPromptResult() *mcp.GetPromptResult {
	return &mcp.GetPromptResult{
		Meta:        mcp.Meta{"docs/source": "fake"},
		Description: "Set up hypertables",
		Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: "Set up hypertables"}},
			{Role: "assistant", Content: &mcp.ImageContent{Data: []byte("png"), MIMEType: "image/png"}},
			{Role: "user", Content: &mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "docs://intro", Text: "intro"}}},
		},
	}
}

// docsResourceResult is the result a fake docs server's resources and
// resource templates return, with text and binary contents and caching hints.
func docsResourceResult() *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{
		Meta:       mcp.Meta{"docs/source": "fake"},
		TTLMs:      60000,
		CacheScope: "private",
		Contents: []*mcp.ResourceContents{
			{URI: "docs://guide", MIMEType: "text/markdown", Text: "guide"},
			{URI: "docs://guide/diagram", MIMEType: "image/png", Blob: []byte("png")},
		},
	}
}

// withMeta returns result with its _meta replaced by meta, as the agent gets a
// result the proxy passes through.
func withMeta[R mcp.Result](result R, meta mcp.Meta) R {
	result.SetMeta(meta)
	return result
}

// docsCompletionResult is the result a fake docs server's completions return.
func docsCompletionResult() *mcp.CompleteResult {
	return &mcp.CompleteResult{
		Meta:       mcp.Meta{"docs/source": "fake"},
		Completion: mcp.CompletionResultDetails{Values: []string{"hypertables", "hyperfunctions"}, Total: 2, HasMore: true},
	}
}

// docsServer is a fake docs MCP server, served over streamable HTTP, that
// records the requests it receives and the clients making them.
type docsServer struct {
	url string
	// protocolVersion is the protocol version the proxy negotiates with the
	// server.
	protocolVersion string

	mu        sync.Mutex
	methods   []string     // the methods received after the handshake, in order
	clients   []docsClient // the client behind each request that declares one
	forwarded [][]byte     // the JSON params of each request the proxy forwarded from the agent
}

// docsClient is the client a fake docs server saw making a request, and the
// request's method.
type docsClient struct {
	method string
	client string
}

// startDocsServer starts a fake docs server with the given features, stopping
// it when the test ends. Like pg-aiguide, it's stateless.
func startDocsServer(t *testing.T, features docsFeatures) *docsServer {
	t.Helper()
	d := &docsServer{protocolVersion: "2026-07-28"}
	if len(features.protocolVersions) > 0 {
		d.protocolVersion = features.protocolVersions[0]
	}

	opts := &mcp.ServerOptions{
		PageSize:                  features.pageSize,
		SupportedProtocolVersions: features.protocolVersions,
	}
	if features.completions {
		opts.CompletionHandler = func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return docsCompletionResult(), nil
		}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "docs"}, opts)

	if features.tools {
		search := func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return docsToolResult(), nil
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
			return &mcp.CallToolResult{
				StructuredContent: map[string]any{"color": answer.Content["color"], "request_state": req.Params.RequestState},
			}, nil
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
		read := func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return docsResourceResult(), nil
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
		get := func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return docsPromptResult(), nil
		}
		prompts := []string{"setup", "intro"}
		if features.unnamed {
			prompts = append(prompts, "")
		}
		for _, name := range prompts {
			server.AddPrompt(&mcp.Prompt{Name: name, Arguments: []*mcp.PromptArgument{{Name: "topic"}}}, get)
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
	}, &mcp.StreamableHTTPOptions{Stateless: true}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	d.url = srv.URL
	return d
}

// handshakeMethods are the requests that establish a session, whose methods a
// fake docs server doesn't record.
var handshakeMethods = []string{"initialize", "notifications/initialized", "server/discover"}

// forwardedMethods are the requests the proxy forwards from the agent, whose
// params a fake docs server records.
var forwardedMethods = []string{"tools/call", "prompts/get", "resources/read", "completion/complete"}

// record records a request's method, unless it's part of the handshake, and
// the client making it, if the request declares one. Before 2026-07-28, only
// the initialize request does, since a stateless server forgets it afterwards;
// from 2026-07-28 on, every request does, in its _meta.
func (d *docsServer) record(method string, req mcp.Request) {
	params, ok := req.GetParams().(*mcp.InitializeParams)
	if !ok {
		if session, ok := req.GetSession().(*mcp.ServerSession); ok {
			params = session.InitializeParams()
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !slices.Contains(handshakeMethods, method) {
		d.methods = append(d.methods, method)
	}
	if params != nil && params.ClientInfo != nil {
		d.clients = append(d.clients, docsClient{method: method, client: describeClient(params)})
	}
	if slices.Contains(forwardedMethods, method) {
		forwarded, err := json.Marshal(req.GetParams())
		if err != nil {
			panic(err)
		}
		d.forwarded = append(d.forwarded, forwarded)
	}
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

// assertCalls asserts the methods the docs server received after the
// handshake, in order, and that every client it saw was the proxy as a client
// in its own right: never the agent, at the protocol version it negotiated
// with the docs server, and advertising no capabilities. It also asserts that
// the docs server received each request the proxy forwarded with the agent's
// params, unchanged apart from _meta. The agent's SDK adds inputResponses and
// requestState itself when it retries a request after answering an input
// request, so those are left out of the comparison too.
func (d *docsServer) assertCalls(t *testing.T, want []string, params mcp.Params) {
	t.Helper()
	proxy := describeClient(&mcp.InitializeParams{
		ClientInfo:      &mcp.Implementation{Name: "tiger-mcp-proxy-client"},
		ProtocolVersion: d.protocolVersion,
		Capabilities:    &mcp.ClientCapabilities{},
	})

	d.mu.Lock()
	defer d.mu.Unlock()
	for _, c := range d.clients {
		if c.client != proxy {
			t.Errorf("docs server received %s from %s, want %s", c.method, c.client, proxy)
		}
	}
	if diff := cmp.Diff(want, d.methods); diff != "" {
		t.Errorf("docs server calls mismatch (-want +got):\n%s", diff)
	}

	if params == nil {
		return
	}
	sent, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("encode params: %v", err)
	}
	wantParams := comparableParams(t, sent)
	for _, forwarded := range d.forwarded {
		if diff := cmp.Diff(wantParams, comparableParams(t, forwarded)); diff != "" {
			t.Errorf("docs server params mismatch (-agent sent +docs server received):\n%s", diff)
		}
	}
}

// comparableParams decodes JSON params, leaving out the fields assertCalls
// doesn't compare.
func comparableParams(t *testing.T, params []byte) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(params, &decoded); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	for _, field := range []string{"_meta", "inputResponses", "requestState"} {
		delete(decoded, field)
	}
	return decoded
}
