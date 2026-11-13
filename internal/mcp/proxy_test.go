package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDocsProxy(t *testing.T) {
	toolsAndPrompts := newDocsServer(docsFeatures{tools: true, prompts: true})
	toolsAndResources := newDocsServer(docsFeatures{tools: true, resources: true})
	promptsOnly := newDocsServer(docsFeatures{prompts: true})
	paginated := newDocsServer(docsFeatures{tools: true, pageSize: 1})
	whoami := newDocsServer(docsFeatures{whoami: true})
	legacy := newDocsServer(docsFeatures{tools: true, whoami: true, legacy: true})
	asks := newDocsServer(docsFeatures{asks: true})
	completions := newDocsServer(docsFeatures{tools: true, prompts: true, completions: true})

	// The proxy calls the docs server as a client in its own right, whatever
	// the agent sends or supports.
	proxyIdentity := func(protocolVersion string) map[string]any {
		return map[string]any{
			"client":            "tiger-mcp-proxy-client",
			"protocol_version":  protocolVersion,
			"capabilities_diff": "",
		}
	}

	runToolTests(t, []toolTest{
		{
			name:       "skips resources the server doesn't advertise",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(toolsAndPrompts)},
			wantOutput: map[string]any{"results": "docs"},
			checks:     []checkFunc{toolsAndPrompts.checkListed("tools/list", "prompts/list")},
		},
		{
			name:       "skips prompts the server doesn't advertise",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(toolsAndResources)},
			wantOutput: map[string]any{"results": "docs"},
			checks:     []checkFunc{toolsAndResources.checkListed("tools/list", "resources/list", "resources/templates/list")},
		},
		{
			name:        "skips tools the server doesn't advertise",
			tool:        "search_docs",
			args:        map[string]any{},
			opts:        []runOption{withDocsServer(promptsOnly)},
			wantCallErr: `calling "tools/call": unknown tool "search_docs"`,
			checks:      []checkFunc{promptsOnly.checkListed("prompts/list")},
		},
		{
			name:       "registers tools from every page",
			tool:       "view_skill",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(paginated)},
			wantOutput: map[string]any{"results": "docs"},
			checks:     []checkFunc{paginated.checkListed("tools/list", "tools/list")},
		},
		{
			name:       "calls tools on a docs server that only speaks older protocol versions",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(legacy)},
			wantOutput: map[string]any{"results": "docs"},
		},
		{
			name:       "calls the docs server as itself on newer protocol versions",
			tool:       "whoami",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(whoami), withElicitation(&mcp.ElicitResult{Action: "accept"})},
			wantOutput: proxyIdentity("2026-07-28"),
		},
		{
			name:       "calls the docs server as itself on older protocol versions",
			tool:       "whoami",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(legacy), withElicitation(&mcp.ElicitResult{Action: "accept"})},
			wantOutput: proxyIdentity("2025-11-25"),
		},
		{
			name:       "passes the docs server's input requests through to the agent",
			tool:       "ask",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(asks), withElicitation(&mcp.ElicitResult{Action: "accept", Content: map[string]any{"color": "blue"}})},
			wantPrompt: &mcp.ElicitParams{Mode: "form", Message: "Favorite color?", RequestedSchema: askSchema},
			wantOutput: map[string]any{"color": "blue", "request_state": "asked"},
		},
		{
			name:       "proxies completions when the server advertises them",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(completions)},
			wantOutput: map[string]any{"results": "docs"},
			checks: []checkFunc{
				checkAdvertisesCompletions(true),
				checkCompletion(
					&mcp.CompleteParams{
						Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: "setup"},
						Argument: mcp.CompleteParamsArgument{Name: "topic", Value: "hyper"},
					},
					mcp.CompletionResultDetails{Values: []string{"setup topic hyper"}},
				),
			},
		},
		{
			name:       "skips completions the server doesn't advertise",
			tool:       "search_docs",
			args:       map[string]any{},
			opts:       []runOption{withDocsServer(toolsAndPrompts)},
			wantOutput: map[string]any{"results": "docs"},
			checks:     []checkFunc{checkAdvertisesCompletions(false)},
		},
	})
}

// docsFeatures picks what a fake docs MCP server exposes, and therefore which
// capabilities it advertises. A nonzero pageSize splits its lists into pages,
// and completions completes any argument with its prompt, name, and value.
// whoami adds a tool reporting the client the server sees, and asks one that
// asks the client for input before it answers. legacy serves only
// protocol versions before 2026-07-28, as pg-aiguide did until it upgraded its
// SDK.
type docsFeatures struct {
	tools       bool
	resources   bool
	prompts     bool
	whoami      bool
	asks        bool
	completions bool
	legacy      bool
	pageSize    int
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
			value := strings.Join([]string{req.Params.Ref.Name, req.Params.Argument.Name, req.Params.Argument.Value}, " ")
			return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{value}}}, nil
		}
	}
	d := &docsServer{
		server: mcp.NewServer(&mcp.Implementation{Name: "docs"}, opts),
		legacy: features.legacy,
	}
	if features.tools {
		for _, name := range []string{"search_docs", "view_skill"} {
			d.server.AddTool(&mcp.Tool{
				Name:        name,
				InputSchema: map[string]any{"type": "object"},
			}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{
					Content:           []mcp.Content{&mcp.TextContent{Text: `{"results":"docs"}`}},
					StructuredContent: map[string]any{"results": "docs"},
				}, nil
			})
		}
	}
	if features.whoami {
		d.server.AddTool(&mcp.Tool{
			Name:        "whoami",
			InputSchema: map[string]any{"type": "object"},
		}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				StructuredContent: map[string]any{
					"client":           req.ClientInfo().Name,
					"protocol_version": req.ProtocolVersion(),
					// Empty when the client advertises no capabilities at all.
					"capabilities_diff": cmp.Diff(&mcp.ClientCapabilities{}, req.Session.InitializeParams().Capabilities),
				},
			}, nil
		})
	}
	if features.asks {
		d.server.AddTool(&mcp.Tool{
			Name:        "ask",
			InputSchema: map[string]any{"type": "object"},
		}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			answer, ok := req.Params.InputResponses["color"].(*mcp.ElicitResult)
			if !ok {
				return &mcp.CallToolResult{
					InputRequests: mcp.InputRequestMap{
						"color": &mcp.ElicitParams{Message: "Favorite color?", RequestedSchema: askSchema},
					},
					RequestState: "asked",
				}, nil
			}
			return &mcp.CallToolResult{
				StructuredContent: map[string]any{
					"color":         answer.Content["color"],
					"request_state": req.Params.RequestState,
				},
			}, nil
		})
	}
	if features.resources {
		readResource := func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{}, nil
		}
		d.server.AddResource(&mcp.Resource{Name: "guide", URI: "docs://guide"}, readResource)
		d.server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "page", URITemplate: "docs://pages/{page}"}, readResource)
	}
	if features.prompts {
		d.server.AddPrompt(&mcp.Prompt{Name: "setup"}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{}, nil
		})
	}
	d.server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if strings.HasSuffix(method, "/list") {
				d.mu.Lock()
				d.listed = append(d.listed, method)
				d.mu.Unlock()
			}
			return next(ctx, method, req)
		}
	})
	return d
}

// checkListed returns a check asserting the list methods the proxy requested
// from the docs server, in order.
func (d *docsServer) checkListed(want ...string) checkFunc {
	return func(t *testing.T, _ toolResult) {
		t.Helper()
		d.mu.Lock()
		defer d.mu.Unlock()
		if diff := cmp.Diff(want, d.listed); diff != "" {
			t.Errorf("listed methods mismatch (-want +got):\n%s", diff)
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

// askSchema is the form the docs server's ask tool requests.
var askSchema = map[string]any{
	"type":       "object",
	"properties": map[string]any{"color": map[string]any{"type": "string"}},
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
