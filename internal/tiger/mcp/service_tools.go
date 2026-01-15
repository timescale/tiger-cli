package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// Service type constants matching OpenAPI spec (uppercase)
const (
	serviceTypeTimescaleDB = "TIMESCALEDB"
	serviceTypePostgres    = "POSTGRES"
	serviceTypeVector      = "VECTOR"
)

// Wait timeout for MCP tool operations
const waitTimeout = 10 * time.Minute

// validServiceTypes returns a slice of all valid service type values
func validServiceTypes() []string {
	return []string{
		serviceTypeTimescaleDB,
		serviceTypePostgres,
		serviceTypeVector,
	}
}

// ServiceListInput represents input for service_list
type ServiceListInput struct{}

func (ServiceListInput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceListInput](nil))
}

// ServiceListOutput represents output for service_list
type ServiceListOutput struct {
	Services []ServiceInfo `json:"services"`
}

func (ServiceListOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceListOutput](nil))
}

// ServiceInfo represents simplified service information for MCP output
type ServiceInfo struct {
	ServiceID string        `json:"id" jsonschema:"Service identifier (10-character alphanumeric string)"`
	Name      string        `json:"name"`
	Status    string        `json:"status" jsonschema:"Service status (e.g., READY, PAUSED, CONFIGURING, UPGRADING)"`
	Type      string        `json:"type"`
	Region    string        `json:"region"`
	Created   string        `json:"created,omitempty"`
	Resources *ResourceInfo `json:"resources,omitempty"`
}

func (ServiceInfo) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceInfo](nil))
	schema.Properties["type"].Enum = util.AnySlice(validServiceTypes())
	return schema
}

// ResourceInfo represents resource allocation information
type ResourceInfo struct {
	CPU    string `json:"cpu,omitempty" jsonschema:"CPU allocation (e.g., '0.5 cores', '1 core')"`
	Memory string `json:"memory,omitempty" jsonschema:"Memory allocation (e.g., '2 GB', '4 GB')"`
}

// setServiceIDSchemaProperties sets common service_id schema properties
func setServiceIDSchemaProperties(schema *jsonschema.Schema) {
	schema.Properties["service_id"].Description = "Unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs."
	schema.Properties["service_id"].Examples = []any{"e6ue9697jf", "u8me885b93"}
	schema.Properties["service_id"].Pattern = "^[a-z0-9]{10}$"
}

// setWithPasswordSchemaProperties sets common with_password schema properties
func setWithPasswordSchemaProperties(schema *jsonschema.Schema) {
	schema.Properties["with_password"].Description = "Whether to include the password in the response and connection string. NEVER set to true unless the user explicitly asks for the password."
	schema.Properties["with_password"].Default = util.Must(json.Marshal(false))
	schema.Properties["with_password"].Examples = []any{false, true}
}

// ServiceGetInput represents input for service_get
type ServiceGetInput struct {
	ServiceID    string `json:"service_id"`
	WithPassword bool   `json:"with_password,omitempty"`
}

func (ServiceGetInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceGetInput](nil))
	setServiceIDSchemaProperties(schema)
	setWithPasswordSchemaProperties(schema)

	return schema
}

// ServiceGetOutput represents output for service_get
type ServiceGetOutput struct {
	Service ServiceDetail `json:"service"`
}

func (ServiceGetOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceGetOutput](nil))
}

// ServiceDetail represents detailed service information
type ServiceDetail struct {
	ServiceID        string        `json:"id" jsonschema:"Service identifier (10-character alphanumeric string)"`
	Name             string        `json:"name"`
	Status           string        `json:"status" jsonschema:"Service status (e.g., READY, PAUSED, CONFIGURING, UPGRADING)"`
	Type             string        `json:"type"`
	Region           string        `json:"region"`
	Created          string        `json:"created,omitempty"`
	Resources        *ResourceInfo `json:"resources,omitempty"`
	Replicas         int           `json:"replicas" jsonschema:"Number of HA replicas (0=single node/no HA, 1+=HA enabled)"`
	DirectEndpoint   string        `json:"direct_endpoint,omitempty" jsonschema:"Direct database connection endpoint"`
	PoolerEndpoint   string        `json:"pooler_endpoint,omitempty" jsonschema:"Connection pooler endpoint"`
	Password         string        `json:"password,omitempty" jsonschema:"Password for tsdbadmin user (only included if with_password=true)"`
	ConnectionString string        `json:"connection_string" jsonschema:"PostgreSQL connection string (password embedded only if with_password=true)"`
}

func (ServiceDetail) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceDetail](nil))
	schema.Properties["type"].Enum = util.AnySlice(validServiceTypes())
	return schema
}

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

// ServiceUpdatePasswordInput represents input for service_update_password
type ServiceUpdatePasswordInput struct {
	ServiceID string `json:"service_id"`
	Password  string `json:"password"`
}

func (ServiceUpdatePasswordInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceUpdatePasswordInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["password"].Description = "The new password for the 'tsdbadmin' user. Must be strong and secure."
	schema.Properties["password"].Examples = []any{"MySecurePassword123!"}

	return schema
}

// ServiceUpdatePasswordOutput represents output for service_update_password
type ServiceUpdatePasswordOutput struct {
	Message         string                        `json:"message"`
	PasswordStorage *common.PasswordStorageResult `json:"password_storage,omitempty"`
}

func (ServiceUpdatePasswordOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceUpdatePasswordOutput](nil))
}

// ServiceStartInput represents input for service_start
type ServiceStartInput struct {
	ServiceID string `json:"service_id"`
	Wait      bool   `json:"wait,omitempty"`
}

func (ServiceStartInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceStartInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["wait"].Description = "Whether to wait for the service to be fully started before returning. Default is false (recommended). Only set to true if your next steps require connecting to or querying this database. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	return schema
}

// ServiceStartOutput represents output for service_start
type ServiceStartOutput struct {
	Status  string `json:"status" jsonschema:"Current service status after start operation"`
	Message string `json:"message"`
}

func (ServiceStartOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceStartOutput](nil))
}

// ServiceStopInput represents input for service_stop
type ServiceStopInput struct {
	ServiceID string `json:"service_id"`
	Wait      bool   `json:"wait,omitempty"`
}

func (ServiceStopInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceStopInput](nil))

	setServiceIDSchemaProperties(schema)

	schema.Properties["wait"].Description = "Whether to wait for the service to be fully stopped before returning. Default is false (recommended). Only set to true if your next steps require confirmation that the service is stopped. When true, waits up to 10 minutes."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	return schema
}

// ServiceStopOutput represents output for service_stop
type ServiceStopOutput struct {
	Status  string `json:"status" jsonschema:"Current service status after stop operation"`
	Message string `json:"message"`
}

func (ServiceStopOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceStopOutput](nil))
}

// registerServiceTools registers service management tools with comprehensive schemas and descriptions
func (s *Server) registerServiceTools() {
	// service_list
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_list",
		Title: "List Database Services",
		Description: "List all database services in your Tiger Cloud project. " +
			"Returns services with status, type, region, and resource allocation.",
		InputSchema:  ServiceListInput{}.Schema(),
		OutputSchema: ServiceListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
			Title:        "List Database Services",
		},
	}, s.handleServiceList)

	// service_get
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_get",
		Title: "Get Service Details",
		Description: "Get detailed information for a specific database service. " +
			"Returns connection endpoints, replica configuration, resource allocation, creation time, and status.",
		InputSchema:  ServiceGetInput{}.Schema(),
		OutputSchema: ServiceGetOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
			Title:        "Get Service Details",
		},
	}, s.handleServiceGet)

	// service_create
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_create",
		Title: "Create Database Service",
		Description: `Create a new database service in Tiger Cloud with specified type, compute resources, region, and HA options.

The default type of service created depends on the user's plan:
- Free plan: Creates a service with shared CPU/memory and the 'time-series' and 'ai' add-ons
- Paid plans: Creates a service with 0.5 CPU / 2 GB memory and the 'time-series' add-on

WARNING: Creates billable resources.`,
		InputSchema:  ServiceCreateInput{}.Schema(),
		OutputSchema: ServiceCreateOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: util.Ptr(false), // Creates resources but doesn't modify existing
			IdempotentHint:  false,           // Creating with same name creates multiple services (name is not unique)
			Title:           "Create Database Service",
		},
	}, s.handleServiceCreate)

	// service_fork
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_fork",
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
			DestructiveHint: util.Ptr(false), // Creates resources but doesn't modify existing
			IdempotentHint:  false,           // Forking same service multiple times creates multiple forks
			Title:           "Fork Database Service",
		},
	}, s.handleServiceFork)

	// service_update_password
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_update_password",
		Title: "Update Service Password",
		Description: "Update master password for 'tsdbadmin' user of a database service. " +
			"Takes effect immediately. May terminate existing connections.",
		InputSchema:  ServiceUpdatePasswordInput{}.Schema(),
		OutputSchema: ServiceUpdatePasswordOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: util.Ptr(true), // Modifies authentication credentials
			IdempotentHint:  true,           // Same password can be set multiple times
			Title:           "Update Service Password",
		},
	}, s.handleServiceUpdatePassword)

	// service_start
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_start",
		Title: "Start Database Service",
		Description: `Start a stopped database service.

This operation starts a service that is currently in a stopped/paused state. The service will transition to a ready state and become available for connections.`,
		InputSchema:  ServiceStartInput{}.Schema(),
		OutputSchema: ServiceStartOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: util.Ptr(false), // Starting a service cannot really break anything
			IdempotentHint:  true,            // Starting an already-started service is safe (but returns an error)
			Title:           "Start Database Service",
		},
	}, s.handleServiceStart)

	// service_stop
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_stop",
		Title: "Stop Database Service",
		Description: `Stop a running database service.

This operation stops a service that is currently running. The service will transition to a stopped/paused state and will no longer accept connections.`,
		InputSchema:  ServiceStopInput{}.Schema(),
		OutputSchema: ServiceStopOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: util.Ptr(true), // Stopping a service breaks existing connections and could cause app downtime
			IdempotentHint:  true,           // Stopping an already-stopped service is safe (but returns an error)
			Title:           "Stop Database Service",
		},
	}, s.handleServiceStop)
}

// handleServiceList handles the service_list MCP tool
func (s *Server) handleServiceList(ctx context.Context, req *mcp.CallToolRequest, input ServiceListInput) (*mcp.CallToolResult, ServiceListOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceListOutput{}, err
	}

	logging.Debug("MCP: Listing services", zap.String("project_id", cfg.ProjectID))

	// Make API call to list services
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.GetProjectsProjectIdServicesWithResponse(ctx, cfg.ProjectID)
	if err != nil {
		return nil, ServiceListOutput{}, fmt.Errorf("failed to list services: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 200 {
		return nil, ServiceListOutput{}, resp.JSON4XX
	}

	if resp.JSON200 == nil {
		return nil, ServiceListOutput{Services: []ServiceInfo{}}, nil
	}

	services := *resp.JSON200
	output := ServiceListOutput{
		Services: make([]ServiceInfo, len(services)),
	}

	for i, service := range services {
		output.Services[i] = s.convertToServiceInfo(service)
	}

	return nil, output, nil
}

// handleServiceGet handles the service_get MCP tool
func (s *Server) handleServiceGet(ctx context.Context, req *mcp.CallToolRequest, input ServiceGetInput) (*mcp.CallToolResult, ServiceGetOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceGetOutput{}, err
	}

	logging.Debug("MCP: Getting service details",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID))

	// Make API call to get service details
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, cfg.ProjectID, input.ServiceID)
	if err != nil {
		return nil, ServiceGetOutput{}, fmt.Errorf("failed to get service details: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 200 {
		return nil, ServiceGetOutput{}, resp.JSON4XX
	}

	output := ServiceGetOutput{
		Service: s.convertToServiceDetail(*resp.JSON200, input.WithPassword),
	}

	// Check if password was requested but not available
	if input.WithPassword && output.Service.Password == "" {
		return nil, ServiceGetOutput{}, fmt.Errorf("requested password but password not available")
	}

	return nil, output, nil
}

// handleServiceCreate handles the service_create MCP tool
func (s *Server) handleServiceCreate(ctx context.Context, req *mcp.CallToolRequest, input ServiceCreateInput) (*mcp.CallToolResult, ServiceCreateOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
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

	logging.Debug("MCP: Creating service",
		zap.String("project_id", cfg.ProjectID),
		zap.String("name", input.Name),
		zap.Strings("addons", input.Addons),
		zap.Stringp("region", input.Region),
		zap.Stringp("cpu", cpuMillis),
		zap.Stringp("memory", memoryGBs),
		zap.Int("replicas", input.Replicas),
	)

	// Prepare service creation request
	serviceCreateReq := api.ServiceCreate{
		Name:         input.Name,
		Addons:       util.ConvertStringSlicePtr[api.ServiceCreateAddons](input.Addons),
		RegionCode:   input.Region,
		ReplicaCount: &input.Replicas,
		CpuMillis:    cpuMillis,
		MemoryGbs:    memoryGBs,
	}

	// Make API call to create service
	createCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.PostProjectsProjectIdServicesWithResponse(createCtx, cfg.ProjectID, serviceCreateReq)
	if err != nil {
		return nil, ServiceCreateOutput{}, fmt.Errorf("failed to create service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 202 {
		return nil, ServiceCreateOutput{}, resp.JSON4XX
	}

	service := *resp.JSON202
	serviceID := util.Deref(service.ServiceId)

	// Set as default service if requested (defaults to true)
	if input.SetDefault {
		if err := cfg.Set("service_id", serviceID); err != nil {
			// Log warning but don't fail the service creation
			logging.Debug("MCP: Failed to set service as default", zap.Error(err))
		} else {
			logging.Debug("MCP: Set service as default", zap.String("service_id", serviceID))
		}
	}

	// Save password immediately after service creation, before any waiting
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

	// If wait is explicitly requested, wait for service to be ready
	message := "Service creation request accepted. The service may still be provisioning."
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
			message = "Service created successfully and is ready!"
		}
	}

	// Convert service to output format (after wait so status is accurate)
	output := ServiceCreateOutput{
		Service:         s.convertToServiceDetail(service, input.WithPassword),
		Message:         message,
		PasswordStorage: passwordStorage,
	}

	return nil, output, nil
}

// handleServiceFork handles the service_fork MCP tool
func (s *Server) handleServiceFork(ctx context.Context, req *mcp.CallToolRequest, input ServiceForkInput) (*mcp.CallToolResult, ServiceForkOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
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

	resp, err := cfg.Client.PostProjectsProjectIdServicesServiceIdForkServiceWithResponse(forkCtx, cfg.ProjectID, input.ServiceID, forkReq)
	if err != nil {
		return nil, ServiceForkOutput{}, fmt.Errorf("failed to fork service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 202 {
		return nil, ServiceForkOutput{}, resp.JSON4XX
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

// handleServiceUpdatePassword handles the service_update_password MCP tool
func (s *Server) handleServiceUpdatePassword(ctx context.Context, req *mcp.CallToolRequest, input ServiceUpdatePasswordInput) (*mcp.CallToolResult, ServiceUpdatePasswordOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	logging.Debug("MCP: Updating service password",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID))

	// Prepare password update request
	updateReq := api.UpdatePasswordInput{
		Password: input.Password,
	}

	// Make API call to update password
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.PostProjectsProjectIdServicesServiceIdUpdatePasswordWithResponse(ctx, cfg.ProjectID, input.ServiceID, updateReq)
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("failed to update service password: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 200 && resp.StatusCode() != 204 {
		return nil, ServiceUpdatePasswordOutput{}, resp.JSON4XX
	}

	// Get service details for password storage (similar to CLI implementation)
	var passwordStorage *common.PasswordStorageResult
	serviceResp, err := cfg.Client.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, cfg.ProjectID, input.ServiceID)
	if err == nil && serviceResp.StatusCode() == 200 && serviceResp.JSON200 != nil {
		// Save the new password using the shared util function
		result, err := common.SavePasswordWithResult(api.Service(*serviceResp.JSON200), input.Password, "tsdbadmin")
		passwordStorage = &result
		if err != nil {
			logging.Debug("MCP: Password storage failed", zap.Error(err))
		} else {
			logging.Debug("MCP: Password saved successfully", zap.String("method", result.Method))
		}
	}

	output := ServiceUpdatePasswordOutput{
		Message:         "Master password for 'tsdbadmin' user updated successfully",
		PasswordStorage: passwordStorage,
	}

	return nil, output, nil
}

// handleServiceStart handles the service_start MCP tool
func (s *Server) handleServiceStart(ctx context.Context, req *mcp.CallToolRequest, input ServiceStartInput) (*mcp.CallToolResult, ServiceStartOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceStartOutput{}, err
	}

	logging.Debug("MCP: Starting service",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID))

	// Make API call to start service
	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.PostProjectsProjectIdServicesServiceIdStartWithResponse(startCtx, cfg.ProjectID, input.ServiceID)
	if err != nil {
		return nil, ServiceStartOutput{}, fmt.Errorf("failed to start service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 202 {
		return nil, ServiceStartOutput{}, resp.JSON4XX
	}

	service := *resp.JSON202

	// If wait is explicitly requested, wait for service to be ready
	message := "Service start request accepted. The service may still be starting."
	if input.Wait {
		if err := common.WaitForService(ctx, common.WaitForServiceArgs{
			Client:    cfg.Client,
			ProjectID: cfg.ProjectID,
			ServiceID: input.ServiceID,
			Handler: &common.StatusWaitHandler{
				TargetStatus: "READY",
				Service:      &service,
			},
			Timeout:    waitTimeout,
			TimeoutMsg: "service may still be starting",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Service started successfully and is ready!"
		}
	}

	// Return status and message (after wait so status is accurate)
	output := ServiceStartOutput{
		Status:  util.DerefStr(service.Status),
		Message: message,
	}

	return nil, output, nil
}

// handleServiceStop handles the service_stop MCP tool
func (s *Server) handleServiceStop(ctx context.Context, req *mcp.CallToolRequest, input ServiceStopInput) (*mcp.CallToolResult, ServiceStopOutput, error) {
	// Load config and API client
	cfg, err := common.LoadConfig(ctx)
	if err != nil {
		return nil, ServiceStopOutput{}, err
	}

	logging.Debug("MCP: Stopping service",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID))

	// Make API call to stop service
	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cfg.Client.PostProjectsProjectIdServicesServiceIdStopWithResponse(stopCtx, cfg.ProjectID, input.ServiceID)
	if err != nil {
		return nil, ServiceStopOutput{}, fmt.Errorf("failed to stop service: %w", err)
	}

	// Handle API response
	if resp.StatusCode() != 202 {
		return nil, ServiceStopOutput{}, resp.JSON4XX
	}

	service := *resp.JSON202

	// If wait is explicitly requested, wait for service to be paused
	message := "Service stop request accepted. The service may still be stopping."
	if input.Wait {
		if err := common.WaitForService(ctx, common.WaitForServiceArgs{
			Client:    cfg.Client,
			ProjectID: cfg.ProjectID,
			ServiceID: input.ServiceID,
			Handler: &common.StatusWaitHandler{
				TargetStatus: "PAUSED",
				Service:      &service,
			},
			Timeout:    waitTimeout,
			TimeoutMsg: "service may still be stopping",
		}); err != nil {
			message = fmt.Sprintf("Error: %s", err.Error())
		} else {
			message = "Service stopped successfully!"
		}
	}

	// Return status and message (after wait so status is accurate)
	output := ServiceStopOutput{
		Status:  util.DerefStr(service.Status),
		Message: message,
	}

	return nil, output, nil
}
