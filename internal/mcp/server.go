package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/analytics"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/logging"
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
	toolDBExecuteQuery          = "db_execute_query"
)

// Server wraps the MCP server with Tiger-specific functionality
type Server struct {
	mcpServer       *mcp.Server
	docsProxyClient *ProxyClient

	// flags is the flag set of the command that started the server. Tool
	// handlers pass it to [common.LoadConfig] on every call so the flags given
	// to `tiger mcp start` (e.g. --config-dir, --service-id) keep taking
	// precedence over the config file, which is re-read per call.
	flags *pflag.FlagSet
}

// addTool registers an MCP tool, skipping readOnlyGatedTools in read-only mode.
func addTool[In, Out any](s *Server, readOnly bool, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	if readOnly && slices.Contains(readOnlyGatedTools, t.Name) {
		logging.Debug("Skipping write tool in read-only mode", zap.String("tool", t.Name))
		return
	}
	mcp.AddTool(s.mcpServer, t, h)
}

// buildServerInstructions returns the `instructions` string the MCP SDK sends
// to clients at initialize. Evaluated once at server start, like tool registration.
func buildServerInstructions(cfg *config.Config) string {
	intro := "Tiger MCP provides tools for managing and querying Tiger Cloud database services (managed TimescaleDB/PostgreSQL). "

	if cfg == nil || !cfg.ReadOnly {
		return intro +
			"Use it to provision and fork services, start/stop/resize instances, rotate credentials, fetch service logs, execute SQL queries, and search Tiger documentation."
	}
	// Read-only mode: announce the mode and the blocked operations so the model
	// won't attempt them.
	return intro +
		"READ-ONLY MODE IS ENABLED. Service-mutating tools are not registered, so do not offer to create, fork, start, stop, resize, or modify services. " +
		"db_execute_query connects read-only, so writes and DDL are rejected by the server."
}

// NewServer creates a new Tiger MCP server instance. The caller-supplied cfg
// is used only to render the read-only warning in server instructions and to
// gate tool registration; flags is retained so each tool call reloads the
// config with the same flag precedence (see Server.flags).
func NewServer(ctx context.Context, cfg *config.Config, flags *pflag.FlagSet) (*Server, error) {
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Title:   serverTitle,
		Version: config.Version,
	}, &mcp.ServerOptions{Instructions: buildServerInstructions(cfg)})

	server := &Server{
		mcpServer: mcpServer,
		flags:     flags,
	}

	// Register all tools (including proxied docs tools). readOnly and
	// experimental are captured here and threaded through registration only.
	// experimental follows the ghost pattern — env-var only, undocumented; see
	// CLAUDE.md's "Experimental Feature Gating".
	readOnly := cfg != nil && cfg.ReadOnly
	experimental, _ := strconv.ParseBool(os.Getenv("TIGER_EXPERIMENTAL"))
	server.registerTools(ctx, readOnly, experimental)

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
func (s *Server) registerTools(ctx context.Context, readOnly, experimental bool) {
	// Service management tools
	s.registerServiceTools(readOnly, experimental)

	// Database operation tools
	s.registerDatabaseTools(readOnly)

	// TODO: Register more tool groups

	// Register remote docs MCP server proxy
	s.registerDocsProxy(ctx)

	logging.Info("MCP tools registered successfully")
}

// registerServiceTools registers service management tools with comprehensive schemas and descriptions
func (s *Server) registerServiceTools(readOnly, experimental bool) {
	addTool(s, readOnly, newServiceListTool(), s.handleServiceList)
	addTool(s, readOnly, newServiceGetTool(), s.handleServiceGet)
	addTool(s, readOnly, newServiceCreateTool(), s.handleServiceCreate)
	addTool(s, readOnly, newServiceForkTool(), s.handleServiceFork)
	addTool(s, readOnly, newServiceUpdatePasswordTool(), s.handleServiceUpdatePassword)
	addTool(s, readOnly, newServiceStartTool(), s.handleServiceStart)
	addTool(s, readOnly, newServiceStopTool(), s.handleServiceStop)
	addTool(s, readOnly, newServiceResizeTool(), s.handleServiceResize)
	addTool(s, readOnly, newServiceLogsTool(), s.handleServiceLogs)

	// Metrics tools target gateway endpoints marked `x-preview: true`. They
	// are registered only when the experimental gate is on at server startup;
	// the user must restart the MCP server after toggling the gate. Handler
	// bodies re-check the gate defensively in case config changes mid-session.
	if experimental {
		addTool(s, readOnly, newServiceMetricsAvailableTool(), s.handleServiceMetricsAvailable)
		addTool(s, readOnly, newServiceMetricsSeriesTool(), s.handleServiceMetricsSeries)
	}
}

// registerDatabaseTools registers database operation tools with comprehensive schemas and descriptions
func (s *Server) registerDatabaseTools(readOnly bool) {
	addTool(s, readOnly, newDBExecuteQueryTool(), s.handleDBExecuteQuery)

	mcp.AddTool(s.mcpServer, newDBSchemaTool(), s.handleDBSchema)
}

// analyticsMiddleware tracks analytics for all MCP requests
func (s *Server) analyticsMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, runErr error) {
		start := time.Now()

		// Load config for analytics
		cfg, err := config.Load(s.flags)
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
