package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/util"
)

// ServiceCreateInput represents input for service_create
type ServiceCreateInput struct {
	Name         string   `json:"name,omitempty"`
	Addons       []string `json:"addons,omitempty"`
	Region       *string  `json:"region,omitempty"`
	CPUMemory    string   `json:"cpu_memory,omitempty"`
	Replicas     int      `json:"replicas,omitempty"`
	Wait         bool     `json:"wait,omitempty"`
	SetDefault   bool     `json:"set_default,omitempty"`
	WithPassword bool     `json:"with_password,omitempty"`
}

func (ServiceCreateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceCreateInput](nil))

	schema.Properties["name"].Description = "Human-readable name for the service (auto-generated if not provided)"
	schema.Properties["name"].Examples = []any{"my-production-db", "analytics-service", "user-store"}

	schema.Properties["addons"].Description = "Array of addons to enable for the service. 'time-series' enables TimescaleDB, 'ai' enables AI/vector extensions. Use empty array for PostgreSQL-only."
	schema.Properties["addons"].Items.Enum = []any{common.AddonTimeSeries, common.AddonAI}
	schema.Properties["addons"].UniqueItems = true

	schema.Properties["region"].Description = "AWS region where the service will be deployed. Choose the region closest to your users for optimal performance."
	schema.Properties["region"].Examples = []any{"us-east-1", "us-west-2", "eu-west-1", "eu-central-1", "ap-southeast-1"}

	schema.Properties["cpu_memory"].Description = "CPU and memory allocation combination. Choose from the available configurations."
	schema.Properties["cpu_memory"].Enum = util.AnySlice(common.GetAllowedCPUMemoryConfigs().Strings())

	schema.Properties["replicas"].Description = "Number of high-availability replicas for fault tolerance. Higher replica counts increase cost but improve availability."
	schema.Properties["replicas"].Minimum = util.Ptr(0.0)
	schema.Properties["replicas"].Maximum = util.Ptr(5.0)
	schema.Properties["replicas"].Default = util.Must(json.Marshal(0))
	schema.Properties["replicas"].Examples = []any{0, 1, 2}

	schema.Properties["wait"].Description = "Whether to wait for the service to be fully ready before returning. Default is false (recommended). Only set to true if your next steps require connecting to or querying this database. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	schema.Properties["set_default"].Description = "Whether to set the newly created service as the default service. When true, the service will be set as the default for future commands."
	schema.Properties["set_default"].Default = util.Must(json.Marshal(true))
	schema.Properties["set_default"].Examples = []any{true, false}

	setWithPasswordSchemaProperties(schema)

	return schema
}

// ServiceCreateOutput represents output for service_create
type ServiceCreateOutput struct {
	Service         ServiceDetail                 `json:"service"`
	Message         string                        `json:"message"`
	PasswordStorage *common.PasswordStorageResult `json:"password_storage,omitempty"`
}

func (ServiceCreateOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceCreateOutput](nil))
}

func newServiceCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  toolServiceCreate,
		Title: "Create Database Service",
		Description: `Create a new database service in Tiger Cloud with specified type, compute resources, region, and HA options.

The default type of service created depends on the user's plan:
- Free plan: Creates a service with shared CPU/memory and the 'time-series' and 'ai' add-ons
- Paid plans: Creates a service with 0.5 CPU / 2 GB memory and the 'time-series' add-on

WARNING: Creates billable resources.`,
		InputSchema:  ServiceCreateInput{}.Schema(),
		OutputSchema: ServiceCreateOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: util.Ptr(false), // Creates resources but doesn't modify existing
			IdempotentHint:  false,           // Creating with same name creates multiple services (name is not unique)
			OpenWorldHint:   util.Ptr(true),
			Title:           "Create Database Service",
		},
	}
}

// handleServiceCreate handles the service_create MCP tool
func (s *Server) handleServiceCreate(ctx context.Context, req *mcp.CallToolRequest, input ServiceCreateInput) (*mcp.CallToolResult, ServiceCreateOutput, error) {
	cfg, client, projectID, err := s.app.GetAll()
	if err != nil {
		return nil, ServiceCreateOutput{}, err
	}

	if err := common.CheckReadOnly(cfg); err != nil {
		return nil, ServiceCreateOutput{}, err
	}

	// Auto-generate service name if not provided
	if input.Name == "" {
		input.Name = common.GenerateServiceName()
	}

	var cpuMillis, memoryGBs *string
	if input.CPUMemory != "" {
		cpuMillisStr, memoryGBsStr, err := common.ParseCPUMemory(input.CPUMemory)
		if err != nil {
			return nil, ServiceCreateOutput{}, fmt.Errorf("invalid CPU/Memory specification: %w", err)
		}
		cpuMillis, memoryGBs = &cpuMillisStr, &memoryGBsStr
	}

	s.logger.Info("MCP: Creating service",
		slog.String("project_id", projectID),
		slog.String("name", input.Name),
		slog.Any("addons", input.Addons),
		slog.Any("region", input.Region),
		slog.Any("cpu", cpuMillis),
		slog.Any("memory", memoryGBs),
		slog.Int("replicas", input.Replicas),
	)

	// Prepare service creation request
	serviceCreateReq := api.ServiceCreate{
		Name:         input.Name,
		Addons:       util.ConvertStringSlicePtr[api.ServiceCreateAddons](input.Addons),
		RegionCode:   input.Region,
		ReplicaCount: &input.Replicas,
		CPUMillis:    cpuMillis,
		MemoryGbs:    memoryGBs,
	}

	// Make API call to create service
	resp, err := client.CreateServiceWithResponse(ctx, projectID, serviceCreateReq)
	if err != nil {
		return nil, ServiceCreateOutput{}, fmt.Errorf("failed to create service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != http.StatusAccepted {
		return nil, ServiceCreateOutput{}, common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
	}

	if resp.JSON202 == nil {
		return nil, ServiceCreateOutput{}, fmt.Errorf("empty response from API")
	}

	service := *resp.JSON202
	serviceID := service.ServiceID

	// Set as default service if requested (defaults to true)
	if input.SetDefault {
		if err := cfg.Set("service_id", serviceID); err != nil {
			// Log warning but don't fail the service creation
			s.logger.Warn("MCP: Failed to set service as default", slog.Any("error", err))
		} else {
			s.logger.Info("MCP: Set service as default", slog.String("service_id", serviceID))
		}
	}

	// Save password immediately after service creation, before any waiting
	// This ensures the password is stored even if the wait fails or is interrupted
	var passwordStorage *common.PasswordStorageResult
	if service.InitialPassword != nil {
		result, err := common.SavePasswordWithResult(cfg, api.Service(service), *service.InitialPassword, "tsdbadmin")
		passwordStorage = &result
		if err != nil {
			s.logger.Warn("MCP: Password storage failed", slog.Any("error", err))
		} else {
			s.logger.Info("MCP: Password saved successfully", slog.String("method", result.Method))
		}
	}

	// If wait is explicitly requested, wait for service to be ready
	message := "Service creation request accepted. The service may still be provisioning."
	if input.Wait {
		if err := common.WaitForService(ctx, common.WaitForServiceArgs{
			Client:    client,
			ProjectID: projectID,
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
			message = "Service created successfully and is ready!"
		}
	}

	// Convert service to output format (after wait so status is accurate)
	output := ServiceCreateOutput{
		Service:         s.convertToServiceDetail(cfg, service, input.WithPassword),
		Message:         message,
		PasswordStorage: passwordStorage,
	}

	return nil, output, nil
}
