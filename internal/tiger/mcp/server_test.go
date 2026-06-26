package mcp

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/tiger/config"
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
func registeredToolNames(t *testing.T, readOnly bool) []string {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	s := &Server{
		mcpServer: mcp.NewServer(&mcp.Implementation{
			Name:    ServerName,
			Title:   serverTitle,
			Version: config.Version,
		}, nil),
	}
	s.registerServiceTools(readOnly)
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
		readOnly         bool
		wantGatedPresent bool
	}{
		{"read-write registers all tools", false, true},
		{"read-only skips gated tools", true, false},
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
