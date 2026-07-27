package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/logging"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceForkInput represents input for service_fork
type ServiceForkInput struct {
	ServiceID    string           `json:"service_id"`
	Name         string           `json:"name,omitempty"`
	ForkStrategy api.ForkStrategy `json:"fork_strategy"`
	TargetTime   *time.Time       `json:"target_time,omitempty"`
	CPUMemory    string           `json:"cpu_memory,omitempty"`
	Wait         bool             `json:"wait,omitempty"`
	SetDefault   bool             `json:"set_default,omitempty"`
	WithPassword bool             `json:"with_password,omitempty"`
}

func (ServiceForkInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceForkInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["name"].Description = "Human-readable name for the forked service (auto-generated if not provided)"
	schema.Properties["name"].Examples = []any{"my-forked-db", "prod-fork-test", "backup-db"}

	schema.Properties["fork_strategy"].Description = "Fork strategy: 'NOW' creates fork at current state, 'LAST_SNAPSHOT' uses last existing snapshot (faster), 'PITR' allows point-in-time recovery to specific timestamp (requires target_time parameter)"
	schema.Properties["fork_strategy"].Enum = []any{api.NOW, api.LASTSNAPSHOT, api.PITR}
	schema.Properties["fork_strategy"].Examples = []any{api.NOW, api.LASTSNAPSHOT}

	schema.Properties["target_time"].Description = "Target timestamp for point-in-time recovery (RFC3339 format, e.g., '2025-01-15T10:30:00Z'). Only used when fork_strategy is 'PITR'."
	schema.Properties["target_time"].Examples = []any{"2025-01-15T10:30:00Z", "2024-12-01T00:00:00Z"}

	schema.Properties["cpu_memory"].Description = "CPU and memory allocation combination. Choose from the available configurations. If not specified, inherits from source service."
	schema.Properties["cpu_memory"].Enum = util.AnySlice(common.GetAllowedCPUMemoryConfigs().Strings())

	schema.Properties["wait"].Description = "Whether to wait for the forked service to be fully ready before returning. Default is false (recommended). Only set to true if your next steps require connecting to or querying this database. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	schema.Properties["set_default"].Description = "Whether to set the newly forked service as the default service. When true, the forked service will be set as the default for future commands."
	schema.Properties["set_default"].Default = util.Must(json.Marshal(true))
	schema.Properties["set_default"].Examples = []any{true, false}

	setWithPasswordSchemaProperties(schema)

	return schema
}

// ServiceForkOutput represents output for service_fork
type ServiceForkOutput struct {
	Service         ServiceDetail                 `json:"service"`
	Message         string                        `json:"message"`
	PasswordStorage *common.PasswordStorageResult `json:"password_storage,omitempty"`
}

func (ServiceForkOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceForkOutput](nil))
}

func newServiceForkTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceFork,
		Title: "Fork Database Service",
		Description: `Fork an existing database service to create a new independent copy.

You must specify a fork strategy:
- 'NOW': Fork at the current database state (creates new snapshot or uses WAL replay)
- 'LAST_SNAPSHOT': Fork at the last existing snapshot (faster fork)
- 'PITR': Fork at a specific point in time (requires target_time parameter)

By default:
- Name will be auto-generated as '{source-service-name}-fork'
- CPU and memory will be inherited from the source service
- The forked service will be set as the default service

WARNING: Creates billable resources.`,
		InputSchema:  ServiceForkInput{}.Schema(),
		OutputSchema: ServiceForkOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: util.Ptr(false), // Creates resources but doesn't modify existing
			IdempotentHint:  false,           // Forking same service multiple times creates multiple forks
			OpenWorldHint:   util.Ptr(true),
			Title:           "Fork Database Service",
		},
	}
}

// handleServiceFork handles the service_fork MCP tool
func (s *Server) handleServiceFork(ctx context.Context, req *mcp.CallToolRequest, input ServiceForkInput) (*mcp.CallToolResult, ServiceForkOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceForkOutput{}, err
	}

	if err := common.CheckReadOnly(cfg.Config); err != nil {
		return nil, ServiceForkOutput{}, err
	}

	// Validate fork strategy and target_time relationship
	switch input.ForkStrategy {
	case api.PITR:
		if input.TargetTime == nil {
			return nil, ServiceForkOutput{}, fmt.Errorf("target_time is required when fork_strategy is 'PITR'")
		}
	default:
		if input.TargetTime != nil {
			return nil, ServiceForkOutput{}, fmt.Errorf("target_time cannot be specified when fork_strategy is not 'PITR'")
		}
	}

	// Parse CPU/Memory configuration if provided
	var cpuMillis, memoryGBs *string
	if input.CPUMemory != "" {
		cpuMillisStr, memoryGBsStr, err := common.ParseCPUMemory(input.CPUMemory)
		if err != nil {
			return nil, ServiceForkOutput{}, fmt.Errorf("invalid CPU/Memory specification: %w", err)
		}
		cpuMillis, memoryGBs = &cpuMillisStr, &memoryGBsStr
	}

	logging.Debug("MCP: Forking service",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID),
		zap.String("name", input.Name),
		zap.String("fork_strategy", string(input.ForkStrategy)),
		zap.Stringp("cpu", cpuMillis),
		zap.Stringp("memory", memoryGBs),
	)

	// Prepare service fork request
	forkReq := api.ForkServiceCreate{
		ForkStrategy: input.ForkStrategy,
		TargetTime:   input.TargetTime,
		CpuMillis:    cpuMillis,
		MemoryGbs:    memoryGBs,
	}

	// Only set name if provided
	if input.Name != "" {
		forkReq.Name = &input.Name
	}

	// Make API call to fork service
	forkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.ForkServiceWithResponse(forkCtx, cfg.ProjectID, input.ServiceID, forkReq)
	if err != nil {
		return nil, ServiceForkOutput{}, fmt.Errorf("failed to fork service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusAccepted {
		return nil, ServiceForkOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON202 == nil {
		return nil, ServiceForkOutput{}, fmt.Errorf("empty response from API")
	}

	service := *resp.JSON202
	serviceID := util.Deref(service.ServiceId)

	// Save password immediately after service fork, before any waiting
	// This ensures the password is stored even if the wait fails or is interrupted
	var passwordStorage *common.PasswordStorageResult
	if service.InitialPassword != nil {
		result, err := common.SavePasswordWithResult(api.Service(service), *service.InitialPassword, "tsdbadmin")
		passwordStorage = &result
		if err != nil {
			logging.Debug("MCP: Password storage failed", zap.Error(err))
		} else {
			logging.Debug("MCP: Password saved successfully", zap.String("method", result.Method))
		}
	}

	// Set as default service if requested (defaults to true)
	if input.SetDefault {
		if err := cfg.Set("service_id", serviceID); err != nil {
			// Log warning but don't fail the service fork
			logging.Debug("MCP: Failed to set service as default", zap.Error(err))
		} else {
			logging.Debug("MCP: Set service as default", zap.String("service_id", serviceID))
		}
	}

	// If wait is explicitly requested, wait for service to be ready
	message := "Service fork request accepted. The forked service may still be provisioning."
	if input.Wait {
		if err := common.WaitForService(ctx, common.WaitForServiceArgs{
			Client:    cfg.Client,
			ProjectID: cfg.ProjectID,
			ServiceID: serviceID,
			Handler: &common.StatusWaitHandler{
				TargetStatus: "READY",
				Service:      &service,
			},
			Timeout:    waitTimeout,
			TimeoutMsg: "service may still be provisioning",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Service forked successfully and is ready!"
		}
	}

	// Convert service to output format (after wait so status is accurate)
	output := ServiceForkOutput{
		Service:         s.convertToServiceDetail(service, input.WithPassword),
		Message:         message,
		PasswordStorage: passwordStorage,
	}

	return nil, output, nil
}
