package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/analytics"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
)

const (
	ServerName  = "tiger"
	serverTitle = "Tiger MCP"
)

// Server wraps the MCP server with Tiger-specific functionality
type Server struct {
	mcpServer       *mcp.Server
	docsProxyClient *ProxyClient
}

// buildServerInstructions returns the `instructions` string the MCP SDK
// sends to clients at initialize.
//
// Instructions are evaluated once at server start; toggling read_only
// mid-session leaves the warning stale until the MCP client restarts. The
// gate itself stays correct because handlers reload config per call.
func buildServerInstructions(cfg *config.Config) string {
	base := "Tiger MCP provides tools for creating, managing, and querying Tiger Cloud database services (managed TimescaleDB/PostgreSQL). " +
		"Use it to provision and fork services, start/stop/resize instances, rotate credentials, fetch service logs, execute SQL queries, and search Tiger documentation."

	if cfg == nil || !cfg.ReadOnly {
		return base
	}
	return base + " " +
		"READ-ONLY MODE IS ENABLED. The following Tiger MCP tools will refuse to run: " +
		strings.Join(readOnlyGatedTools, ", ") + ". " +
		"Before asking the user to provide inputs for any of these operations, tell them read-only mode is on."
}

// NewServer creates a new Tiger MCP server instance. The caller-supplied cfg
// is used only to render the read-only warning in server instructions.
func NewServer(ctx context.Context, cfg *config.Config) (*Server, error) {
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Title:   serverTitle,
		Version: config.Version,
	}, &mcp.ServerOptions{Instructions: buildServerInstructions(cfg)})

	server := &Server{
		mcpServer: mcpServer,
	}

	// Register all tools (including proxied docs tools)
	server.registerTools(ctx)

	// Add analytics tracking middleware
	server.mcpServer.AddReceivingMiddleware(server.analyticsMiddleware)

	return server, nil
}

// StartStdio starts the MCP server with the stdio transport
func (s *Server) StartStdio(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

// Returns an HTTP handler that implements the http transport
func (s *Server) HTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return s.mcpServer
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
	})
}

// registerTools registers all available MCP tools
func (s *Server) registerTools(ctx context.Context) {
	// Service management tools
	s.registerServiceTools()

	// Billing tools
	s.registerBillingTools()

	// Database operation tools
	s.registerDatabaseTools()

	// TODO: Register more tool groups

	// Register remote docs MCP server proxy
	s.registerDocsProxy(ctx)

	logging.Info("MCP tools registered successfully")
}

// analyticsMiddleware tracks analytics for all MCP requests
func (s *Server) analyticsMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, runErr error) {
		start := time.Now()

		// Load config for analytics
		cfg, err := config.Load()
		if err != nil {
			// If we can't load config, just skip analytics and continue
			return next(ctx, method, req)
		}

		client, projectID, _ := common.NewAPIClient(ctx, cfg)
		a := analytics.New(cfg, client, projectID)

		switch r := req.(type) {
		case *mcp.CallToolRequest:
			// Extract arguments from the tool call
			var args map[string]any
			if len(r.Params.Arguments) > 0 {
				if err := json.Unmarshal(r.Params.Arguments, &args); err != nil {
					logging.Error("Error unmarshaling tool call arguments", zap.Error(err))
				}
			}

			defer func() {
				toolErr := runErr
				if callToolResult, ok := result.(*mcp.CallToolResult); ok && callToolResult != nil && callToolResult.IsError && len(callToolResult.Content) > 0 {
					if textContent, ok := callToolResult.Content[0].(*mcp.TextContent); ok && textContent != nil {
						toolErr = errors.New(textContent.Text)
					}
				}

				a.Track(fmt.Sprintf("Call %s tool", r.Params.Name),
					analytics.Map(args),
					analytics.Property("elapsed_seconds", time.Since(start).Seconds()),
					analytics.Error(toolErr),
				)
			}()
		case *mcp.ReadResourceRequest:
			defer func() {
				a.Track("Read proxied resource",
					analytics.Property("resource_uri", r.Params.URI),
					analytics.Property("elapsed_seconds", time.Since(start).Seconds()),
					analytics.Error(runErr),
				)
			}()
		case *mcp.GetPromptRequest:
			defer func() {
				a.Track(fmt.Sprintf("Get %s prompt", r.Params.Name),
					analytics.Property("elapsed_seconds", time.Since(start).Seconds()),
					analytics.Error(runErr),
				)
			}()
		}

		// Execute the actual handler
		return next(ctx, method, req)
	}
}

// Close gracefully shuts down the MCP server and all proxy connections
func (s *Server) Close() error {
	logging.Debug("Closing MCP server and proxy connections")

	// Close docs proxy connection
	if err := s.docsProxyClient.Close(); err != nil {
		return fmt.Errorf("failed to close docs proxy client: %w", err)
	}

	return nil
}
