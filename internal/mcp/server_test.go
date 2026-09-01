package mcp

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/config"
)

// alwaysRegisteredTools must be available regardless of read-only mode.
var alwaysRegisteredTools = []string{
	toolServiceList,
	toolServiceGet,
	toolServiceLogs,
	toolDBExecuteQuery,
}

// registeredToolNames returns the tool names a server advertises over a real
// client/server session. registerDocsProxy is skipped: it connects to a remote
// server.
func registeredToolNames(t *testing.T, readOnly config.ReadOnlyMode) []string {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	s := &Server{
		mcpServer: mcp.NewServer(&mcp.Implementation{
			Name:    ServerName,
			Title:   serverTitle,
			Version: config.Version,
		}, nil),
		logger: ensureLogger(nil),
	}
	s.registerServiceTools(readOnly, false)
	s.registerDatabaseTools(readOnly)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	serverSession, err := s.mcpServer.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	names := make([]string, len(res.Tools))
	for i, tool := range res.Tools {
		names[i] = tool.Name
	}
	return names
}

func TestReadOnlyToolRegistration(t *testing.T) {
	for _, tt := range []struct {
		name             string
		readOnly         config.ReadOnlyMode
		wantGatedPresent bool
	}{
		{"read-write registers all tools", config.ReadOnlyOff, true},
		{"read-only skips gated tools", config.ReadOnlyAll, false},
		{"prod mode keeps gated tools registered", config.ReadOnlyProd, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			names := registeredToolNames(t, tt.readOnly)

			// Read tools and the read-only-safe query tool are always present.
			for _, name := range alwaysRegisteredTools {
				if !slices.Contains(names, name) {
					t.Errorf("expected tool %q to be registered, got %v", name, names)
				}
			}
			// Service-mutating tools are present only in read-write mode.
			for _, name := range readOnlyGatedTools {
				if got := slices.Contains(names, name); got != tt.wantGatedPresent {
					t.Errorf("gated tool %q registered = %v, want %v (got %v)", name, got, tt.wantGatedPresent, names)
				}
			}
		})
	}
}

func TestBuildServerInstructions(t *testing.T) {
	const capabilitiesMarker = "Tiger MCP provides tools"

	readWrite := buildServerInstructions(&config.Config{ReadOnly: config.ReadOnlyOff})
	readOnly := buildServerInstructions(&config.Config{ReadOnly: config.ReadOnlyAll})
	prodOnly := buildServerInstructions(&config.Config{ReadOnly: config.ReadOnlyProd})

	// The capabilities blurb is always present, whatever the mode.
	for _, got := range []string{readWrite, readOnly, prodOnly} {
		if !strings.Contains(got, capabilitiesMarker) {
			t.Errorf("instructions missing capabilities blurb: %q", got)
		}
	}

	// The read-only banner appears only when a read-only mode is set.
	const banner = "READ-ONLY MODE IS ENABLED"
	for _, got := range []string{readOnly, prodOnly} {
		if !strings.Contains(got, banner) {
			t.Errorf("read-only instructions missing banner: %q", got)
		}
	}
	if strings.Contains(readWrite, banner) {
		t.Errorf("read-write instructions should not contain banner: %q", readWrite)
	}

	// Read-only instructions never name the gated tools, since they aren't
	// registered; prod-mode ones must explain what the registered tools refuse.
	for _, tool := range readOnlyGatedTools {
		if strings.Contains(readOnly, tool) {
			t.Errorf("read-only instructions should not name gated tool %q: %q", tool, readOnly)
		}
	}
	if !strings.Contains(prodOnly, "PROD") {
		t.Errorf("prod-mode instructions should explain that PROD services are refused: %q", prodOnly)
	}
}
