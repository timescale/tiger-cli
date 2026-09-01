package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/analytics"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

const (
	ServerName  = "tiger"
	serverTitle = "Tiger MCP"
)

// MCP tool names. Centralized so the read-only gate (see errors.go) and the
// tool registrations share a single source of truth.
const (
	toolServiceList             = "service_list"
	toolServiceGet              = "service_get"
	toolServiceCreate           = "service_create"
	toolServiceFork             = "service_fork"
	toolServiceStart            = "service_start"
	toolServiceStop             = "service_stop"
	toolServiceResize           = "service_resize"
	toolServiceUpdatePassword   = "service_update_password"
	toolServiceLogs             = "service_logs"
	toolServiceMetricsAvailable = "service_metrics_available"
	toolServiceMetricsSeries    = "service_metrics_series"
	toolServiceBackups          = "service_backups"
	toolDBExecuteQuery          = "db_execute_query"
)

// Server wraps the MCP server with Tiger-specific functionality
type Server struct {
	mcpServer       *mcp.Server
	docsProxyClient *ProxyClient
	logger          *slog.Logger

	// app holds the config and API client. The analytics middleware reloads it
	// once per request, so config changes and logins made while the session is
	// open take effect on the next request; handlers then read that state via
	// s.app.GetAll and friends.
	app *common.App
}

// addTool registers an MCP tool, skipping readOnlyGatedTools under read_only=all.
// Under prod they stay registered — they still work on DEV services — and refuse
// per call in the handler, once the target is known.
func addTool[In, Out any](s *Server, mode config.ReadOnlyMode, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	if mode.BlocksAll() && slices.Contains(readOnlyGatedTools, t.Name) {
		s.logger.Info("Skipping write tool in read-only mode", slog.String("tool", t.Name))
		return
	}
	mcp.AddTool(s.mcpServer, t, h)
}

// buildServerInstructions returns the `instructions` string the MCP SDK sends
// to clients at initialize. Evaluated once at server start, like tool registration.
func buildServerInstructions(cfg *config.Config) string {
	const (
		intro        = "Tiger MCP provides tools for managing and querying Tiger Cloud database services (managed TimescaleDB/PostgreSQL). "
		capabilities = "Use it to provision and fork services, start/stop/resize instances, rotate credentials, fetch service logs, execute SQL queries, and search Tiger documentation."
	)

	switch cfg.ReadOnly {
	case config.ReadOnlyAll:
		// The write tools aren't registered, so announce the mode instead of
		// advertising them.
		return intro +
			"READ-ONLY MODE IS ENABLED. Service-mutating tools are not registered, so do not offer to create, fork, start, stop, resize, or modify services. " +
			"db_execute_query connects read-only, so writes and DDL are rejected by the server."
	case config.ReadOnlyProd:
		// The write tools are registered, so keep advertising them but explain
		// the refusals — otherwise one looks like a bug.
		return intro + capabilities + " " +
			"READ-ONLY MODE IS ENABLED FOR PRODUCTION SERVICES. Services tagged PROD cannot be modified: the service-mutating tools refuse them, and db_execute_query connects to them read-only, so writes and DDL are rejected by the server. " +
			"Services tagged DEV are unaffected. Check a service's environment field (from service_get or service_list) before offering to modify it."
	default:
		return intro + capabilities
	}
}

// NewServer creates a new Tiger MCP server instance. The app must already be
// loaded: its config renders the read-only warning in the server instructions
// and gates which tools are registered, both evaluated once here at startup.
// A nil logger discards the server's log output.
func NewServer(ctx context.Context, app *common.App, logger *slog.Logger) (*Server, error) {
	cfg := app.GetConfig()
	logger = ensureLogger(logger)

	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Title:   serverTitle,
		Version: config.Version,
	}, &mcp.ServerOptions{
		Instructions: buildServerInstructions(cfg),
		Logger:       logger,
	})

	server := &Server{
		mcpServer: mcpServer,
		logger:    logger,
		app:       app,
	}

	// Register all tools (including proxied docs tools). The read-only mode and
	// experimental gate are captured here and threaded through registration only.
	// experimental follows the ghost pattern — env-var only, undocumented; see
	// CLAUDE.md's "Experimental Feature Gating".
	server.registerTools(ctx, cfg.ReadOnly, app.Experimental)

	// Add analytics tracking middleware
	server.mcpServer.AddReceivingMiddleware(server.analyticsMiddleware)

	return server, nil
}

func ensureLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.DiscardHandler)
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
func (s *Server) registerTools(ctx context.Context, mode config.ReadOnlyMode, experimental bool) {
	// Service management tools
	s.registerServiceTools(mode, experimental)

	// Database operation tools
	s.registerDatabaseTools(mode)

	// TODO: Register more tool groups

	// Register remote docs MCP server proxy
	s.registerDocsProxy(ctx)
}

// registerServiceTools registers service management tools with comprehensive schemas and descriptions
func (s *Server) registerServiceTools(mode config.ReadOnlyMode, experimental bool) {
	addTool(s, mode, newServiceListTool(), s.handleServiceList)
	addTool(s, mode, newServiceGetTool(), s.handleServiceGet)
	addTool(s, mode, newServiceCreateTool(), s.handleServiceCreate)
	addTool(s, mode, newServiceForkTool(), s.handleServiceFork)
	addTool(s, mode, newServiceUpdatePasswordTool(), s.handleServiceUpdatePassword)
	addTool(s, mode, newServiceStartTool(), s.handleServiceStart)
	addTool(s, mode, newServiceStopTool(), s.handleServiceStop)
	addTool(s, mode, newServiceResizeTool(), s.handleServiceResize)
	addTool(s, mode, newServiceLogsTool(), s.handleServiceLogs)

	// Metrics tools target gateway endpoints marked `x-tigerdata-preview: true`. They
	// are registered only when the experimental gate is on at server startup;
	// the user must restart the MCP server after toggling the gate. Handler
	// bodies re-check the gate defensively in case config changes mid-session.
	if experimental {
		addTool(s, mode, newServiceMetricsAvailableTool(), s.handleServiceMetricsAvailable)
		addTool(s, mode, newServiceMetricsSeriesTool(), s.handleServiceMetricsSeries)
		addTool(s, mode, newServiceBackupsTool(), s.handleServiceBackups)
	}
}

// registerDatabaseTools registers database operation tools with comprehensive schemas and descriptions
func (s *Server) registerDatabaseTools(mode config.ReadOnlyMode) {
	addTool(s, mode, newDBExecuteQueryTool(), s.handleDBExecuteQuery)

	mcp.AddTool(s.mcpServer, newDBSchemaTool(), s.handleDBSchema)
}

// analyticsMiddleware tracks analytics for all MCP requests
func (s *Server) analyticsMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, runErr error) {
		start := time.Now()

		// Reload the config and API client for this request, so config changes
		// and logins/logouts made while the session is open take effect. Handlers
		// read the result via s.app.
		cfg, client, projectID, err := s.app.Load(ctx)
		if err != nil {
			// If we can't load config, just skip analytics and continue
			return next(ctx, method, req)
		}

		a := analytics.New(cfg, client, projectID)

		switch r := req.(type) {
		case *mcp.CallToolRequest:
			// Extract arguments from the tool call
			var args map[string]any
			if len(r.Params.Arguments) > 0 {
				if err := json.Unmarshal(r.Params.Arguments, &args); err != nil {
					s.logger.Error("Error unmarshaling tool call arguments", slog.Any("error", err))
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
	// Close docs proxy connection
	if err := s.docsProxyClient.Close(); err != nil {
		return fmt.Errorf("failed to close docs proxy client: %w", err)
	}

	return nil
}
