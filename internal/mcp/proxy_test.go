package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDocsProxy(t *testing.T) {
	everything := docsFeatures{tools: true, resources: true, prompts: true, completions: true}
	full := newDocsServer(everything)
	toolsAndPrompts := newDocsServer(docsFeatures{tools: true, prompts: true})
	toolsAndResources := newDocsServer(docsFeatures{tools: true, resources: true})
	promptsOnly := newDocsServer(docsFeatures{prompts: true})
	legacy := newDocsServer(docsFeatures{tools: true, legacy: true})
	noCapabilities := newDocsServer(docsFeatures{tools: true, legacy: true, noCapabilities: true})
	paginated := newDocsServer(docsFeatures{tools: true, resources: true, prompts: true, pageSize: 1})
	unnamed := newDocsServer(docsFeatures{tools: true, resources: true, prompts: true, unnamed: true})
	failingLists := func(methods ...string) *docsServer {
		features := everything
		features.fail = methods
		return newDocsServer(features)
	}
	failingCalls := failingLists("tools/call", "prompts/get", "resources/read", "completion/complete")

	// The proxy calls the docs server as a client in its own right, whatever
	// the agent sends or supports.
	proxyIdentity := func(protocolVersion string) map[string]any {
		return map[string]any{
			"client":            "tiger-mcp-proxy-client",
			"protocol_version":  protocolVersion,
			"capabilities_diff": "",
		}
	}
	const proxyIdentityText = `client=tiger-mcp-proxy-client protocol_version=2026-07-28 capabilities_diff=""`

	everythingRegistered := docsRegistration{
		tools:     []string{"ask", "search_docs", "view_skill", "whoami"},
		resources: []string{"docs://ask", "docs://guide"},
		templates: []string{"docs://chapters/{chapter}", "docs://pages/{page}"},
		prompts:   []string{"ask", "setup"},
	}
	nothingRegistered := docsRegistration{}
	answerBlue := withElicitation(&mcp.ElicitResult{Action: "accept", Content: map[string]any{"color": "blue"}})
	completeSetup := &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: "setup"},
		Argument: mcp.CompleteParamsArgument{Name: "topic", Value: "hyper"},
	}

	runToolTests(t, []toolTest{
		// Connecting
		{
			name:        "skips the proxy when docs MCP is disabled",
			tool:        "search_docs",
			args:        map[string]any{},
			wantCallErr: `calling "tools/call": unknown tool "search_docs"`,
			checks: []checkFunc{
				checkStartupLogs("level=INFO msg=\"Docs MCP proxy is disabled\"\n"),
				checkDocsRegistered(nothingRegistered),
			},
		},
		{
			name:        "skips the proxy when there is no docs MCP URL",
			tool:        "search_docs",
			args:        map[string]any{},
			opts:        []runOption{withConfig(map[string]any{"docs_mcp": true, "docs_mcp_url": ""})},
			wantCallErr: `calling "tools/call": unknown tool "search_docs"`,
			checks: []checkFunc{
				checkStartupLogs("level=INFO msg=\"Docs MCP proxy is disabled\"\n"),
				checkDocsRegistered(nothingRegistered),
			},
		},
		{
			name:        "starts without docs features when the docs server is unreachable",
			tool:        "search_docs",
			args:        map[string]any{},
			opts:        []runOption{withUnreachableDocsServer()},
			wantCallErr: `calling "tools/call": unknown tool "search_docs"`,
			checks: []checkFunc{
				// The connection error names a random port and comes from the
				// OS, so only the message itself can be matched exactly.
				checkStartupLogsPrefix(`level=ERROR msg="Failed to connect to docs MCP server" url=http://127.0.0.1:`),
				checkDocsRegistered(nothingRegistered),
			},
		},

		// Registering what the docs server advertises
		{
			name:       "registers everything the server advertises",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(full)},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				checkStartupLogs(""),
				full.checkListed("tools/list", "resources/list", "resources/templates/list", "prompts/list"),
				checkDocsRegistered(everythingRegistered),
				checkAdvertisesCompletions(true),
			},
		},
		{
			name:       "skips resources and completions the server doesn't advertise",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(toolsAndPrompts)},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				toolsAndPrompts.checkListed("tools/list", "prompts/list"),
				checkDocsRegistered(docsRegistration{tools: everythingRegistered.tools, prompts: everythingRegistered.prompts}),
				checkAdvertisesCompletions(false),
			},
		},
		{
			name:       "skips prompts the server doesn't advertise",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(toolsAndResources)},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				toolsAndResources.checkListed("tools/list", "resources/list", "resources/templates/list"),
				checkDocsRegistered(docsRegistration{
					tools:     everythingRegistered.tools,
					resources: everythingRegistered.resources,
					templates: everythingRegistered.templates,
				}),
			},
		},
		{
			name:        "skips tools the server doesn't advertise",
			tool:        "search_docs",
			args:        map[string]any{},
			opts:        []runOption{withDocsServer(promptsOnly)},
			wantCallErr: `calling "tools/call": unknown tool "search_docs"`,
			checks: []checkFunc{
				promptsOnly.checkListed("prompts/list"),
				checkDocsRegistered(docsRegistration{prompts: everythingRegistered.prompts}),
			},
		},
		{
			name:        "registers nothing when the server advertises no capabilities",
			tool:        "search_docs",
			args:        map[string]any{},
			opts:        []runOption{withDocsServer(noCapabilities)},
			wantCallErr: `calling "tools/call": unknown tool "search_docs"`,
			checks: []checkFunc{
				noCapabilities.checkListed(),
				checkDocsRegistered(nothingRegistered),
			},
		},
		{
			name:       "registers everything from every page",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(paginated)},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				paginated.checkListed(
					"tools/list", "tools/list", "tools/list", "tools/list",
					"resources/list", "resources/list",
					"resources/templates/list", "resources/templates/list",
					"prompts/list", "prompts/list",
				),
				checkDocsRegistered(docsRegistration{
					tools:     everythingRegistered.tools,
					resources: everythingRegistered.resources,
					templates: everythingRegistered.templates,
					prompts:   everythingRegistered.prompts,
				}),
			},
		},
		{
			name:       "skips entries without a name or URI",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(unnamed)},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				checkDocsRegistered(docsRegistration{
					tools:     everythingRegistered.tools,
					resources: everythingRegistered.resources,
					templates: everythingRegistered.templates,
					prompts:   everythingRegistered.prompts,
				}),
			},
		},
		{
			name:        "registers the rest when listing tools fails",
			tool:        "search_docs",
			args:        map[string]any{},
			opts:        []runOption{withDocsServer(failingLists("tools/list"))},
			wantCallErr: `calling "tools/call": unknown tool "search_docs"`,
			checks: []checkFunc{
				checkStartupLogs("level=ERROR msg=\"Failed to register tools from docs MCP server\" error=\"failed to list tools from remote server: calling \\\"tools/list\\\": docs server failure\"\n"),
				checkDocsRegistered(docsRegistration{
					resources: everythingRegistered.resources,
					templates: everythingRegistered.templates,
					prompts:   everythingRegistered.prompts,
				}),
			},
		},
		{
			name:       "registers the rest when listing resources fails",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(failingLists("resources/list"))},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				checkStartupLogs("level=ERROR msg=\"Failed to register resources from docs MCP server\" error=\"failed to list resources from remote server: calling \\\"resources/list\\\": docs server failure\"\n"),
				checkDocsRegistered(docsRegistration{
					tools:     everythingRegistered.tools,
					templates: everythingRegistered.templates,
					prompts:   everythingRegistered.prompts,
				}),
			},
		},
		{
			name:       "registers the rest when listing resource templates fails",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(failingLists("resources/templates/list"))},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				checkStartupLogs("level=ERROR msg=\"Failed to register resource templates from docs MCP server\" error=\"failed to list resource templates from remote server: calling \\\"resources/templates/list\\\": docs server failure\"\n"),
				checkDocsRegistered(docsRegistration{
					tools:     everythingRegistered.tools,
					resources: everythingRegistered.resources,
					prompts:   everythingRegistered.prompts,
				}),
			},
		},
		{
			name:       "registers the rest when listing prompts fails",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(failingLists("prompts/list"))},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				checkStartupLogs("level=ERROR msg=\"Failed to register prompts from docs MCP server\" error=\"failed to list prompts from remote server: calling \\\"prompts/list\\\": docs server failure\"\n"),
				checkDocsRegistered(docsRegistration{
					tools:     everythingRegistered.tools,
					resources: everythingRegistered.resources,
					templates: everythingRegistered.templates,
				}),
			},
		},

		// Forwarding requests
		{
			name:       "forwards prompts, resources, and completions",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(full)},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				checkGetPrompt("setup", "setup: "+proxyIdentityText),
				checkReadResource("docs://guide", "docs://guide: "+proxyIdentityText),
				checkReadResource("docs://pages/intro", "docs://pages/intro: "+proxyIdentityText),
				checkCompletion(completeSetup, mcp.CompletionResultDetails{Values: []string{"setup topic=hyper: " + proxyIdentityText}}),
			},
		},
		{
			name:        "reports the docs server's errors",
			tool:        "search_docs",
			args:        map[string]any{},
			opts:        []runOption{withDocsServer(failingCalls)},
			wantCallErr: `calling "tools/call": remote tool call failed: calling "tools/call": docs server failure`,
			checks: []checkFunc{
				checkGetPromptErr("setup", `calling "prompts/get": remote prompt request failed: calling "prompts/get": docs server failure`),
				checkReadResourceErr("docs://guide", `calling "resources/read": remote resource read failed: calling "resources/read": docs server failure`),
				checkCompletionErr(completeSetup, `calling "completion/complete": remote completion failed: calling "completion/complete": docs server failure`),
			},
		},
		{
			name:       "calls the docs server as itself",
			tool:       "whoami",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(full), answerBlue},
			wantOutput: proxyIdentity("2026-07-28"),
		},
		{
			name:       "calls the docs server as itself for agents on older protocol versions",
			tool:       "whoami",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(full), withClientProtocolVersion("2025-11-25"), answerBlue},
			wantOutput: proxyIdentity("2026-07-28"),
		},
		{
			name:       "calls tools on a docs server that only speaks older protocol versions",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(legacy)},
			wantOutput: map[string]any{"results": "docs"},
		},
		{
			name:       "calls a docs server that only speaks older protocol versions as itself",
			tool:       "whoami",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(legacy), answerBlue},
			wantOutput: proxyIdentity("2025-11-25"),
		},
		{
			name:       "passes the docs server's input requests through to the agent",
			tool:       "ask",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(full), answerBlue},
			wantPrompt: &mcp.ElicitParams{Mode: "form", Message: "Favorite color?", RequestedSchema: askSchema},
			wantOutput: map[string]any{"color": "blue", "request_state": "asked"},
			checks: []checkFunc{
				checkGetPrompt("ask", "ask: color=blue request_state=asked"),
				checkReadResource("docs://ask", "docs://ask: color=blue request_state=asked"),
			},
		},

		// Shutting down
		{
			name:       "closes the docs server connection",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(full)},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				checkClose(),
				checkCallToolErr("search_docs", `calling "tools/call": remote tool call failed: connection closed: calling "tools/call": client is closing`),
			},
		},
		{
			name:        "closes with the proxy disabled",
			tool:        "search_docs",
			args:        map[string]any{},
			wantCallErr: `calling "tools/call": unknown tool "search_docs"`,
			checks:      []checkFunc{checkClose()},
		},
	})
}

// docsFeatures picks what a fake docs MCP server exposes, and therefore which
// capabilities it advertises: tools, resources (with resource templates),
// prompts, and completions. Each kind includes an entry that reports the
// client it sees and one that asks the client for input before it answers.
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

// askSchema is the form a fake docs server requests when it asks for input.
var askSchema = map[string]any{
	"type":       "object",
	"properties": map[string]any{"color": map[string]any{"type": "string"}},
}

// docsServer is a fake remote docs MCP server that records the list methods
// it receives, so a test can assert which ones the proxy requested.
type docsServer struct {
	server *mcp.Server
	legacy bool

	mu     sync.Mutex
	listed []string
}

func newDocsServer(features docsFeatures) *docsServer {
	opts := &mcp.ServerOptions{PageSize: features.pageSize}
	if features.completions {
		opts.CompletionHandler = func(_ context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			value := fmt.Sprintf("%s %s=%s: %s", req.Params.Ref.Name, req.Params.Argument.Name, req.Params.Argument.Value, clientIdentity(req))
			return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{value}}}, nil
		}
	}
	d := &docsServer{
		server: mcp.NewServer(&mcp.Implementation{Name: "docs"}, opts),
		legacy: features.legacy,
	}

	if features.tools {
		docsResult := func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: `{"results":"docs"}`}},
				StructuredContent: map[string]any{"results": "docs"},
			}, nil
		}
		whoami := func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				StructuredContent: map[string]any{
					"client":           req.ClientInfo().Name,
					"protocol_version": req.ProtocolVersion(),
					// Empty when the client advertises no capabilities at all.
					"capabilities_diff": cmp.Diff(&mcp.ClientCapabilities{}, req.Session.InitializeParams().Capabilities),
				},
			}, nil
		}
		ask := func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			color, ok := askedColor(req.Params.InputResponses)
			if !ok {
				return &mcp.CallToolResult{InputRequests: askForColor(), RequestState: "asked"}, nil
			}
			return &mcp.CallToolResult{
				StructuredContent: map[string]any{"color": color, "request_state": req.Params.RequestState},
			}, nil
		}
		handlers := map[string]mcp.ToolHandler{"search_docs": docsResult, "view_skill": docsResult, "whoami": whoami, "ask": ask}
		if features.unnamed {
			handlers[""] = docsResult
		}
		for name, handler := range handlers {
			d.server.AddTool(&mcp.Tool{Name: name, Title: docsTitle, InputSchema: map[string]any{"type": "object"}}, handler)
		}
	}

	if features.resources {
		read := func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			text := req.Params.URI + ": " + clientIdentity(req)
			if strings.HasSuffix(req.Params.URI, "/ask") {
				color, ok := askedColor(req.Params.InputResponses)
				if !ok {
					return &mcp.ReadResourceResult{InputRequests: askForColor(), RequestState: "asked"}, nil
				}
				text = fmt.Sprintf("%s: color=%s request_state=%s", req.Params.URI, color, req.Params.RequestState)
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: text}}}, nil
		}
		resources := []string{"docs://guide", "docs://ask"}
		templates := []string{"docs://pages/{page}", "docs://chapters/{chapter}"}
		if features.unnamed {
			resources = append(resources, "")
			templates = append(templates, "")
		}
		for _, uri := range resources {
			d.server.AddResource(&mcp.Resource{Name: uri, URI: uri}, read)
		}
		for _, uriTemplate := range templates {
			d.server.AddResourceTemplate(&mcp.ResourceTemplate{Name: uriTemplate, URITemplate: uriTemplate}, read)
		}
	}

	if features.prompts {
		get := func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			text := req.Params.Name + ": " + clientIdentity(req)
			if req.Params.Name == "ask" {
				color, ok := askedColor(req.Params.InputResponses)
				if !ok {
					return &mcp.GetPromptResult{InputRequests: askForColor(), RequestState: "asked"}, nil
				}
				text = fmt.Sprintf("ask: color=%s request_state=%s", color, req.Params.RequestState)
			}
			return &mcp.GetPromptResult{
				Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}},
			}, nil
		}
		names := []string{"setup", "ask"}
		if features.unnamed {
			names = append(names, "")
		}
		for _, name := range names {
			d.server.AddPrompt(&mcp.Prompt{Name: name}, get)
		}
	}

	d.server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if strings.HasSuffix(method, "/list") {
				d.mu.Lock()
				d.listed = append(d.listed, method)
				d.mu.Unlock()
			}
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
	return d
}

// clientIdentity describes the client a fake docs server sees making req.
func clientIdentity[P mcp.Params](req *mcp.ServerRequest[P]) string {
	return fmt.Sprintf("client=%s protocol_version=%s capabilities_diff=%q",
		req.ClientInfo().Name,
		req.ProtocolVersion(),
		cmp.Diff(&mcp.ClientCapabilities{}, req.Session.InitializeParams().Capabilities),
	)
}

// askForColor is the input request a fake docs server makes before it
// answers.
func askForColor() mcp.InputRequestMap {
	return mcp.InputRequestMap{"color": &mcp.ElicitParams{Message: "Favorite color?", RequestedSchema: askSchema}}
}

// askedColor returns the color the client answered askForColor with, if it
// has.
func askedColor(responses mcp.InputResponseMap) (any, bool) {
	answer, ok := responses["color"].(*mcp.ElicitResult)
	if !ok {
		return nil, false
	}
	return answer.Content["color"], true
}

// withDocsServer serves docs over streamable HTTP and enables the docs proxy
// against it. The SDK serves protocol versions from 2026-07-28 on only from a
// stateless server, so a legacy docs server is a stateful one.
func withDocsServer(docs *docsServer) runOption {
	return withSetup(func(t *testing.T) {
		srv := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return docs.server
		}, &mcp.StreamableHTTPOptions{Stateless: !docs.legacy}))
		t.Cleanup(func() {
			srv.CloseClientConnections()
			srv.Close()
		})
		t.Setenv("TIGER_DOCS_MCP", "true")
		t.Setenv("TIGER_DOCS_MCP_URL", srv.URL)
	})
}

// withUnreachableDocsServer enables the docs proxy against a URL nothing
// listens on.
func withUnreachableDocsServer() runOption {
	return withSetup(func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		t.Setenv("TIGER_DOCS_MCP", "true")
		t.Setenv("TIGER_DOCS_MCP_URL", srv.URL)
	})
}

// checkListed returns a check asserting the list methods the proxy requested
// from the docs server, in order.
func (d *docsServer) checkListed(want ...string) checkFunc {
	return func(t *testing.T, _ toolResult) {
		t.Helper()
		d.mu.Lock()
		defer d.mu.Unlock()
		if diff := cmp.Diff(want, d.listed, cmpEmptyEqualsNil); diff != "" {
			t.Errorf("listed methods mismatch (-want +got):\n%s", diff)
		}
	}
}

// cmpEmptyEqualsNil treats an empty slice as equal to a nil one.
var cmpEmptyEqualsNil = cmp.FilterValues(func(x, y []string) bool {
	return len(x) == 0 && len(y) == 0
}, cmp.Comparer(func(_, _ []string) bool { return true }))

// docsRegistration lists the docs server's features as an agent sees them
// through Tiger MCP: tools by name, resources by URI, resource templates by URI
// template, and prompts by name, each sorted.
type docsRegistration struct {
	tools     []string
	resources []string
	templates []string
	prompts   []string
}

// checkDocsRegistered returns a check asserting which of the docs server's
// features Tiger MCP offers the agent.
func checkDocsRegistered(want docsRegistration) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		ctx := context.Background()
		var got docsRegistration
		for tool, err := range result.session.Tools(ctx, nil) {
			if err != nil {
				t.Fatalf("list tools: %v", err)
			}
			if tool.Title == docsTitle {
				got.tools = append(got.tools, tool.Name)
			}
		}
		for resource, err := range result.session.Resources(ctx, nil) {
			if err != nil {
				t.Fatalf("list resources: %v", err)
			}
			got.resources = append(got.resources, resource.URI)
		}
		for template, err := range result.session.ResourceTemplates(ctx, nil) {
			if err != nil {
				t.Fatalf("list resource templates: %v", err)
			}
			got.templates = append(got.templates, template.URITemplate)
		}
		for prompt, err := range result.session.Prompts(ctx, nil) {
			if err != nil {
				t.Fatalf("list prompts: %v", err)
			}
			got.prompts = append(got.prompts, prompt.Name)
		}
		for _, names := range [][]string{got.tools, got.resources, got.templates, got.prompts} {
			slices.Sort(names)
		}
		if diff := cmp.Diff(want, got, cmp.AllowUnexported(docsRegistration{}), cmpEmptyEqualsNil); diff != "" {
			t.Errorf("registered docs features mismatch (-want +got):\n%s", diff)
		}
	}
}

// checkStartupLogs returns a check asserting what the server logged while it
// started.
func checkStartupLogs(want string) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		if diff := cmp.Diff(want, result.startupLogs); diff != "" {
			t.Errorf("startup logs mismatch (-want +got):\n%s", diff)
		}
	}
}

// checkStartupLogsPrefix returns a check asserting how what the server logged
// while it started begins, for logs that can't be matched exactly.
func checkStartupLogsPrefix(want string) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		if !strings.HasPrefix(result.startupLogs, want) {
			t.Errorf("startup logs = %q, want prefix %q", result.startupLogs, want)
		}
	}
}

// checkAdvertisesCompletions returns a check asserting whether the server
// advertises completion support to the client.
func checkAdvertisesCompletions(want bool) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		if got := result.session.InitializeResult().Capabilities.Completions != nil; got != want {
			t.Errorf("advertises completions = %v, want %v", got, want)
		}
	}
}

// checkCallToolErr returns a check asserting the error calling the named tool
// again returns.
func checkCallToolErr(name, wantErr string) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		_, err := result.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		checkErr(t, err, wantErr)
	}
}

// checkGetPrompt returns a check asserting the text of the single message the
// named prompt returns.
func checkGetPrompt(name, want string) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		res, err := result.session.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: name})
		if err != nil {
			t.Fatalf("get prompt: %v", err)
		}
		var got []string
		for _, message := range res.Messages {
			got = append(got, message.Content.(*mcp.TextContent).Text)
		}
		if diff := cmp.Diff([]string{want}, got); diff != "" {
			t.Errorf("prompt messages mismatch (-want +got):\n%s", diff)
		}
	}
}

// checkGetPromptErr returns a check asserting the error getting the named
// prompt returns.
func checkGetPromptErr(name, wantErr string) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		_, err := result.session.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: name})
		checkErr(t, err, wantErr)
	}
}

// checkReadResource returns a check asserting the text of the single content
// reading uri returns.
func checkReadResource(uri, want string) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		res, err := result.session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatalf("read resource: %v", err)
		}
		var got []string
		for _, contents := range res.Contents {
			got = append(got, contents.Text)
		}
		if diff := cmp.Diff([]string{want}, got); diff != "" {
			t.Errorf("resource contents mismatch (-want +got):\n%s", diff)
		}
	}
}

// checkReadResourceErr returns a check asserting the error reading uri
// returns.
func checkReadResourceErr(uri, wantErr string) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		_, err := result.session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
		checkErr(t, err, wantErr)
	}
}

// checkCompletion returns a check asserting the completion of params.
func checkCompletion(params *mcp.CompleteParams, want mcp.CompletionResultDetails) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		got, err := result.session.Complete(context.Background(), params)
		if err != nil {
			t.Fatalf("complete: %v", err)
		}
		if diff := cmp.Diff(want, got.Completion); diff != "" {
			t.Errorf("completion mismatch (-want +got):\n%s", diff)
		}
	}
}

// checkCompletionErr returns a check asserting the error completing params
// returns.
func checkCompletionErr(params *mcp.CompleteParams, wantErr string) checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		_, err := result.session.Complete(context.Background(), params)
		checkErr(t, err, wantErr)
	}
}

// checkClose returns a check that closes the server, asserting it closes
// cleanly.
func checkClose() checkFunc {
	return func(t *testing.T, result toolResult) {
		t.Helper()
		if err := result.server.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}
}

// checkErr asserts err has the text wantErr.
func checkErr(t *testing.T, err error, wantErr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %q, got none", wantErr)
	}
	if got := err.Error(); got != wantErr {
		t.Errorf("error = %q, want %q", got, wantErr)
	}
}
