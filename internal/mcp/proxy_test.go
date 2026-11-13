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
	})
}

// docsFeatures picks what a fake docs MCP server exposes, and therefore which
// capabilities it advertises. A nonzero pageSize splits its lists into pages.
type docsFeatures struct {
	tools     bool
	resources bool
	prompts   bool
	pageSize  int
}

// docsServer is a fake remote docs MCP server that records the list methods
// it receives, so a test can assert which ones the proxy requested.
type docsServer struct {
	server *mcp.Server

	mu     sync.Mutex
	listed []string
}

func newDocsServer(features docsFeatures) *docsServer {
	d := &docsServer{
		server: mcp.NewServer(&mcp.Implementation{Name: "docs"}, &mcp.ServerOptions{PageSize: features.pageSize}),
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

// withDocsServer serves docs over streamable HTTP and enables the docs proxy
// against it. The SDK only accepts the latest protocol version on a stateless
// server, which is what the proxy client negotiates by default.
func withDocsServer(docs *docsServer) runOption {
	return withSetup(func(t *testing.T) {
		srv := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return docs.server
		}, &mcp.StreamableHTTPOptions{Stateless: true}))
		t.Cleanup(func() {
			srv.CloseClientConnections()
			srv.Close()
		})
		t.Setenv("TIGER_DOCS_MCP", "true")
		t.Setenv("TIGER_DOCS_MCP_URL", srv.URL)
	})
}
