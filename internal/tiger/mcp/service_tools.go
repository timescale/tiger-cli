package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/password"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// Service type constants matching OpenAPI spec (uppercase)
const (
	serviceTypeTimescaleDB = "TIMESCALEDB"
	serviceTypePostgres    = "POSTGRES"
	serviceTypeVector      = "VECTOR"
)

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
	schema.Properties["service_id"].Description = "The unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs."
	schema.Properties["service_id"].Examples = []any{"e6ue9697jf", "u8me885b93"}
	schema.Properties["service_id"].Pattern = "^[a-z0-9]{10}$"
}

// setServiceIDSchemaProperties sets common with_password schema properties
func setWithPasswordSchemaProperties(schema *jsonschema.Schema) {
	schema.Properties["with_password"].Description = "Whether to include the password in the response and in the returned connection string."
	schema.Properties["with_password"].Default = util.Must(json.Marshal(false))
	schema.Properties["with_password"].Examples = []any{false, true}
}

// ServiceShowInput represents input for service_show
type ServiceShowInput struct {
	ServiceID    string `json:"service_id"`
	WithPassword bool   `json:"with_password,omitempty"`
}

func (ServiceShowInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceShowInput](nil))
	setServiceIDSchemaProperties(schema)
	setWithPasswordSchemaProperties(schema)

	return schema
}

// ServiceShowOutput represents output for service_show
type ServiceShowOutput struct {
	Service ServiceDetail `json:"service"`
}

func (ServiceShowOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceShowOutput](nil))
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
	Paused           bool          `json:"paused"`
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
	Name           string   `json:"name,omitempty"`
	Addons         []string `json:"addons,omitempty"`
	Region         string   `json:"region,omitempty"`
	CPUMemory      string   `json:"cpu_memory,omitempty"`
	Replicas       int      `json:"replicas,omitempty"`
	Free           bool     `json:"free,omitempty"`
	Wait           bool     `json:"wait,omitempty"`
	TimeoutMinutes *int     `json:"timeout_minutes,omitempty"`
	SetDefault     bool     `json:"set_default,omitempty"`
	WithPassword   bool     `json:"with_password,omitempty"`
}

func (ServiceCreateInput) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceCreateInput](nil))

	schema.Properties["name"].Description = "Human-readable name for the service (auto-generated if not provided)"
	schema.Properties["name"].MaxLength = util.Ptr(128) // Matches backend validation
	schema.Properties["name"].Examples = []any{"my-production-db", "analytics-service", "user-store"}

	schema.Properties["addons"].Description = "Array of addons to enable for the service. 'time-series' enables TimescaleDB, 'ai' enables AI/vector extensions. Use empty array for PostgreSQL-only. Defaults to time-series."
	schema.Properties["addons"].Items.Enum = []any{util.AddonTimeSeries, util.AddonAI}
	schema.Properties["addons"].UniqueItems = true

	schema.Properties["region"].Description = "AWS region where the service will be deployed. Choose the region closest to your users for optimal performance. Defaults to us-east-1."
	schema.Properties["region"].Examples = []any{"us-east-1", "us-west-2", "eu-west-1", "eu-central-1", "ap-southeast-1"}

	cpuMemoryCombinations := util.GetAllowedCPUMemoryConfigs().Strings()
	schema.Properties["cpu_memory"].Description = fmt.Sprintf("CPU and memory allocation combination. Choose from the available configurations. Defaults to %s", cpuMemoryCombinations[0])
	schema.Properties["cpu_memory"].Enum = util.AnySlice(cpuMemoryCombinations)

	schema.Properties["replicas"].Description = "Number of high-availability replicas for fault tolerance. Higher replica counts increase cost but improve availability."
	schema.Properties["replicas"].Minimum = util.Ptr(0.0)
	schema.Properties["replicas"].Maximum = util.Ptr(5.0)
	schema.Properties["replicas"].Default = util.Must(json.Marshal(0))
	schema.Properties["replicas"].Examples = []any{0, 1, 2}

	schema.Properties["free"].Description = "Create a free service. When true, addons, region, cpu_memory, and replicas cannot be specified."
	schema.Properties["free"].Default = util.Must(json.Marshal(false))
	schema.Properties["free"].Examples = []any{false, true}

	schema.Properties["wait"].Description = "Whether to wait for the service to be fully ready before returning. Default is false and is recommended because service creation can take a few minutes and it's usually better to return immediately. ONLY set to true if the user explicitly needs to use the service immediately to continue the same conversation."
	schema.Properties["wait"].Default = util.Must(json.Marshal(false))
	schema.Properties["wait"].Examples = []any{false, true}

	schema.Properties["timeout_minutes"].Description = "Timeout in minutes when waiting for service to be ready. Only used when 'wait' is true."
	schema.Properties["timeout_minutes"].Minimum = util.Ptr(0.0)
	schema.Properties["timeout_minutes"].Default = util.Must(json.Marshal(30))
	schema.Properties["timeout_minutes"].Examples = []any{15, 30, 60}

	schema.Properties["set_default"].Description = "Whether to set the newly created service as the default service. When true, the service will be set as the default for future commands."
	schema.Properties["set_default"].Default = util.Must(json.Marshal(true))
	schema.Properties["set_default"].Examples = []any{true, false}

	setWithPasswordSchemaProperties(schema)

	return schema
}

// ServiceCreateOutput represents output for service_create
type ServiceCreateOutput struct {
	Service         ServiceDetail                   `json:"service"`
	Message         string                          `json:"message"`
	PasswordStorage *password.PasswordStorageResult `json:"password_storage,omitempty"`
}

func (ServiceCreateOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceCreateOutput](nil))
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
	Message         string                          `json:"message"`
	PasswordStorage *password.PasswordStorageResult `json:"password_storage,omitempty"`
}

func (ServiceUpdatePasswordOutput) Schema() *jsonschema.Schema {
	return util.Must(jsonschema.For[ServiceUpdatePasswordOutput](nil))
}

// registerServiceTools registers service management tools with comprehensive schemas and descriptions
func (s *Server) registerServiceTools() {
	// service_list
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_list",
		Title: "List Database Services",
		Description: "List all database services in current TigerData project. " +
			"Returns services with status, type, region, and resource allocation.",
		InputSchema:  ServiceListInput{}.Schema(),
		OutputSchema: ServiceListOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
			Title:        "List Database Services",
		},
	}, s.handleServiceList)

	// service_show
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_show",
		Title: "Show Service Details",
		Description: "Get detailed information for a specific database service. " +
			"Returns connection endpoints, replica configuration, resource allocation, creation time, and status.",
		InputSchema:  ServiceShowInput{}.Schema(),
		OutputSchema: ServiceShowOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
			Title:        "Show Service Details",
		},
	}, s.handleServiceShow)

	// service_create
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:  "service_create",
		Title: "Create Database Service",
		Description: `Create a new database service in TigerData Cloud with specified type, compute resources, region, and HA options.

Default behavior: Returns immediately while service provisions in background (recommended).
Setting wait=true will block for a few minutes until ready - only use if user explicitly needs immediate access.
timeout_minutes: Wait duration in minutes (only relevant with wait=true).

WARNING: Creates billable resources.`,
		InputSchema:  ServiceCreateInput{}.Schema(),
		OutputSchema: ServiceCreateOutput{}.Schema(),
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: util.Ptr(false), // Creates resources but doesn't modify existing
			IdempotentHint:  false,           // Creating with same name would fail
			Title:           "Create Database Service",
		},
	}, s.handleServiceCreate)

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
}

// handleServiceList handles the service_list MCP tool
func (s *Server) handleServiceList(ctx context.Context, req *mcp.CallToolRequest, input ServiceListInput) (*mcp.CallToolResult, ServiceListOutput, error) {
	// Load config and validate project ID
	cfg, err := s.loadConfigWithProjectID()
	if err != nil {
		return nil, ServiceListOutput{}, err
	}

	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return nil, ServiceListOutput{}, err
	}

	logging.Debug("MCP: Listing services", zap.String("project_id", cfg.ProjectID))

	// Make API call to list services
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := apiClient.GetProjectsProjectIdServicesWithResponse(ctx, cfg.ProjectID)
	if err != nil {
		return nil, ServiceListOutput{}, fmt.Errorf("failed to list services: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200:
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

	case 401:
		return nil, ServiceListOutput{}, fmt.Errorf("authentication failed: invalid API key")
	case 403:
		return nil, ServiceListOutput{}, fmt.Errorf("permission denied: insufficient access to project")
	default:
		return nil, ServiceListOutput{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}

// handleServiceShow handles the service_show MCP tool
func (s *Server) handleServiceShow(ctx context.Context, req *mcp.CallToolRequest, input ServiceShowInput) (*mcp.CallToolResult, ServiceShowOutput, error) {
	// Load config and validate project ID
	cfg, err := s.loadConfigWithProjectID()
	if err != nil {
		return nil, ServiceShowOutput{}, err
	}

	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return nil, ServiceShowOutput{}, err
	}

	logging.Debug("MCP: Showing service details",
		zap.String("project_id", cfg.ProjectID),
		zap.String("service_id", input.ServiceID))

	// Make API call to get service details
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := apiClient.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, cfg.ProjectID, input.ServiceID)
	if err != nil {
		return nil, ServiceShowOutput{}, fmt.Errorf("failed to get service details: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200:
		if resp.JSON200 == nil {
			return nil, ServiceShowOutput{}, fmt.Errorf("empty response from API")
		}

		service := *resp.JSON200
		output := ServiceShowOutput{
			Service: s.convertToServiceDetail(service),
		}

		// Include password in ServiceDetail if requested
		// Note: service_show doesn't have access to InitialPassword, so we fetch from storage
		if input.WithPassword {
			if passwd, err := password.GetPassword(service); err != nil {
				logging.Debug("MCP: Failed to retrieve password from storage", zap.Error(err))
			} else {
				output.Service.Password = passwd
			}
		}

		// Always include connection string in ServiceDetail
		// Password is embedded in connection string only if with_password=true
		// Note: InitialPassword is empty string here since service_show doesn't have it
		if connectionString, err := password.BuildConnectionString(service, password.ConnectionStringOptions{
			Pooled:       false,
			Role:         "tsdbadmin",
			PasswordMode: password.GetPasswordMode(input.WithPassword),
		}); err != nil {
			logging.Debug("MCP: Failed to build connection string", zap.Error(err))
		} else {
			output.Service.ConnectionString = connectionString
		}

		return nil, output, nil

	case 401:
		return nil, ServiceShowOutput{}, fmt.Errorf("authentication failed: invalid API key")
	case 403:
		return nil, ServiceShowOutput{}, fmt.Errorf("permission denied: insufficient access to service")
	case 404:
		return nil, ServiceShowOutput{}, fmt.Errorf("service '%s' not found in project '%s'", input.ServiceID, cfg.ProjectID)
	default:
		return nil, ServiceShowOutput{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}

// handleServiceCreate handles the service_create MCP tool
func (s *Server) handleServiceCreate(ctx context.Context, req *mcp.CallToolRequest, input ServiceCreateInput) (*mcp.CallToolResult, ServiceCreateOutput, error) {
	// Load config and validate project ID
	cfg, err := s.loadConfigWithProjectID()
	if err != nil {
		return nil, ServiceCreateOutput{}, err
	}

	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
	if err != nil {
		return nil, ServiceCreateOutput{}, err
	}

	// Auto-generate service name if not provided
	if input.Name == "" {
		input.Name = util.GenerateServiceName()
	}

	var cpuMillis, memoryGbs int
	if input.Free {
		// Validate free service restrictions
		if input.Addons != nil {
			return nil, ServiceCreateOutput{}, fmt.Errorf("addons cannot be specified for free services")
		}
		if input.Region != "" {
			return nil, ServiceCreateOutput{}, fmt.Errorf("region cannot be specified for free services")
		}
		if input.CPUMemory != "" {
			return nil, ServiceCreateOutput{}, fmt.Errorf("cpu_memory cannot be specified for free services")
		}
		if input.Replicas != 0 {
			return nil, ServiceCreateOutput{}, fmt.Errorf("replicas cannot be specified for free services")
		}

		// Set free service defaults (in the future, we might geolocate
		// the user to the closest free region here)
		input.Region = "us-east-1"
	} else {
		// Set default addons if not provided
		if input.Addons == nil {
			input.Addons = []string{util.AddonTimeSeries}
		} else {
			// Validate addons for non-free services
			for _, addon := range input.Addons {
				if !util.IsValidAddon(addon) {
					return nil, ServiceCreateOutput{}, fmt.Errorf("invalid addon '%s'. Valid addons: %s", addon, strings.Join(util.ValidAddons(), ", "))
				}
			}
		}

		// Set default region if not provided
		if input.Region == "" {
			input.Region = "us-east-1"
		}

		// Set default CPU/Memory if not provided
		if input.CPUMemory == "" {
			config := util.GetAllowedCPUMemoryConfigs()[0] // Default to smallest config (0.5 CPU/2GB)
			cpuMillis, memoryGbs = config.CPUMillis, config.MemoryGbs
		} else {
			// Validate cpu/memory for non-free services
			cpuMillis, memoryGbs, err = util.ParseCPUMemory(input.CPUMemory)
			if err != nil {
				return nil, ServiceCreateOutput{}, fmt.Errorf("invalid CPU/Memory specification: %w", err)
			}
		}

	}

	logging.Debug("MCP: Creating service",
		zap.String("project_id", cfg.ProjectID),
		zap.String("name", input.Name),
		zap.Strings("addons", input.Addons),
		zap.String("region", input.Region),
		zap.Int("cpu", cpuMillis),
		zap.Int("memory", memoryGbs),
		zap.Int("replicas", input.Replicas),
		zap.Bool("free", input.Free),
	)

	// Prepare service creation request
	serviceCreateReq := api.ServiceCreate{
		Name:         input.Name,
		Addons:       util.ConvertStringSlice[api.ServiceCreateAddons](input.Addons),
		RegionCode:   input.Region,
		ReplicaCount: input.Replicas,
		CpuMillis:    cpuMillis,
		MemoryGbs:    memoryGbs,
	}

	// Set free flag if specified
	if input.Free {
		serviceCreateReq.Free = &input.Free
	}

	// Make API call to create service
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := apiClient.PostProjectsProjectIdServicesWithResponse(ctx, cfg.ProjectID, serviceCreateReq)
	if err != nil {
		return nil, ServiceCreateOutput{}, fmt.Errorf("failed to create service: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 202:
		if resp.JSON202 == nil {
			return nil, ServiceCreateOutput{}, fmt.Errorf("service creation request accepted but no response data received")
		}

		service := *resp.JSON202
		serviceID := util.Deref(service.ServiceId)
		serviceStatus := util.DerefStr(service.Status)

		output := ServiceCreateOutput{
			Service: s.convertToServiceDetail(service),
			Message: "Service creation request accepted. The service may still be provisioning.",
		}

		// Save password immediately after service creation, before any waiting
		// This ensures the password is stored even if the wait fails or is interrupted
		if service.InitialPassword != nil {
			result, err := password.SavePasswordWithResult(api.Service(service), *service.InitialPassword)
			output.PasswordStorage = &result
			if err != nil {
				logging.Debug("MCP: Password storage failed", zap.Error(err))
			} else {
				logging.Debug("MCP: Password saved successfully", zap.String("method", result.Method))
			}
		}

		// Include password in ServiceDetail if requested
		if input.WithPassword {
			output.Service.Password = util.Deref(service.InitialPassword)
		}

		// Always include connection string in ServiceDetail
		// Password is embedded in connection string only if with_password=true
		if connectionString, err := password.BuildConnectionString(api.Service(service), password.ConnectionStringOptions{
			Pooled:          false,
			Role:            "tsdbadmin",
			PasswordMode:    password.GetPasswordMode(input.WithPassword),
			InitialPassword: util.Deref(service.InitialPassword),
		}); err != nil {
			logging.Debug("MCP: Failed to build connection string", zap.Error(err))
		} else {
			output.Service.ConnectionString = connectionString
		}

		// Set as default service if requested (defaults to true)
		if input.SetDefault {
			if err := cfg.Set("service_id", serviceID); err != nil {
				// Log warning but don't fail the service creation
				logging.Debug("MCP: Failed to set service as default", zap.Error(err))
			} else {
				logging.Debug("MCP: Set service as default", zap.String("service_id", serviceID))
			}
		}

		// If wait is explicitly requested, wait for service to be ready
		if input.Wait {
			timeout := time.Duration(*input.TimeoutMinutes) * time.Minute

			output.Service, err = s.waitForServiceReady(apiClient, cfg.ProjectID, serviceID, timeout, serviceStatus)
			if err != nil {
				output.Message = fmt.Sprintf("Error: %s", err.Error())
			} else {
				output.Message = "Service created successfully and is ready!"
			}
		}

		return nil, output, nil
	case 400:
		return nil, ServiceCreateOutput{}, fmt.Errorf("invalid request parameters")
	case 401:
		return nil, ServiceCreateOutput{}, fmt.Errorf("authentication failed: invalid API key")
	case 403:
		return nil, ServiceCreateOutput{}, fmt.Errorf("permission denied: insufficient access to create services")
	default:
		return nil, ServiceCreateOutput{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}

// handleServiceUpdatePassword handles the service_update_password MCP tool
func (s *Server) handleServiceUpdatePassword(ctx context.Context, req *mcp.CallToolRequest, input ServiceUpdatePasswordInput) (*mcp.CallToolResult, ServiceUpdatePasswordOutput, error) {
	// Load config and validate project ID
	cfg, err := s.loadConfigWithProjectID()
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, err
	}

	// Create fresh API client with current credentials
	apiClient, err := s.createAPIClient()
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

	resp, err := apiClient.PostProjectsProjectIdServicesServiceIdUpdatePasswordWithResponse(ctx, cfg.ProjectID, input.ServiceID, updateReq)
	if err != nil {
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("failed to update service password: %w", err)
	}

	// Handle API response
	switch resp.StatusCode() {
	case 200, 204:
		output := ServiceUpdatePasswordOutput{
			Message: "Master password for 'tsdbadmin' user updated successfully",
		}

		// Get service details for password storage (similar to CLI implementation)
		serviceResp, err := apiClient.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, cfg.ProjectID, input.ServiceID)
		if err == nil && serviceResp.StatusCode() == 200 && serviceResp.JSON200 != nil {
			// Save the new password using the shared util function
			result, err := password.SavePasswordWithResult(api.Service(*serviceResp.JSON200), input.Password)
			output.PasswordStorage = &result
			if err != nil {
				logging.Debug("MCP: Password storage failed", zap.Error(err))
			} else {
				logging.Debug("MCP: Password saved successfully", zap.String("method", result.Method))
			}
		}

		return nil, output, nil

	case 401:
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("authentication failed: invalid API key")
	case 403:
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("permission denied: insufficient access to update service password")
	case 404:
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("service '%s' not found in project '%s'", input.ServiceID, cfg.ProjectID)
	case 400:
		var errorMsg string
		if resp.JSON400 != nil && resp.JSON400.Message != nil {
			errorMsg = *resp.JSON400.Message
		} else {
			errorMsg = "Invalid password"
		}
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("invalid password: %s", errorMsg)

	default:
		return nil, ServiceUpdatePasswordOutput{}, fmt.Errorf("API request failed with status %d", resp.StatusCode())
	}
}
