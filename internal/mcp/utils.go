package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
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

// setServiceIDSchemaProperties sets common service_id schema properties.
//
// Intentional divergence from the CLI: every CLI command that takes a service
// ID also takes a service name, but the tools stay IDs-only. Agents have
// service_list, and a name it guessed or remembered may since have moved to a
// different service through a rename, or through a delete and recreate — not a
// risk worth taking when the tool on the other end is destructive. Only the
// ref filter on the API's service list takes a name, and the tools never use
// it: they pass the ID straight to the ID-only endpoints, and the pattern
// below rejects anything else before it gets that far.
func setServiceIDSchemaProperties(schema *jsonschema.Schema) {
	schema.Properties["service_id"].Description = "Unique identifier of the service (10-character alphanumeric string). Use service_list to find service IDs."
	schema.Properties["service_id"].Examples = []any{"e6ue9697jf", "u8me885b93"}
	schema.Properties["service_id"].Pattern = "^[a-z0-9]{10}$"
}

// setWithPasswordSchemaProperties sets common with_password schema properties
func setWithPasswordSchemaProperties(schema *jsonschema.Schema) {
	schema.Properties["with_password"].Description = "Whether to include the password in the response. NEVER set to true unless the user explicitly asks for the password."
	schema.Properties["with_password"].Default = util.Must(json.Marshal(false))
	schema.Properties["with_password"].Examples = []any{false, true}
}

// setCPUMemorySchemaProperties sets a tool's cpu_memory property. shared
// includes shared/shared, which a service can't be resized onto. Without the
// experimental gate, only the standard platform's combinations are offered.
// Behind it, the enum spans both platforms, so description is followed by
// platformDescription, which says how the platform is decided, and then by
// which combinations go with which: an enum can't express that, and every
// tool's description must stand alone, since clients that search tools may
// load one without the others.
func setCPUMemorySchemaProperties(schema *jsonschema.Schema, shared, experimental bool, description, platformDescription string) {
	property := schema.Properties["cpu_memory"]
	property.Enum = util.AnySlice(common.AllowedCPUMemoryConfigs(shared, experimental).Strings())
	if !experimental {
		property.Description = description
		return
	}

	standard := common.StandardCPUMemoryConfigs()
	if !shared {
		standard = standard.WithoutShared()
	}
	property.Description = fmt.Sprintf("%s %s Standard platform: %s. Fluid platform: %s.",
		description, platformDescription, standard, common.FluidCPUMemoryConfigs())
}

// setPlatformSchemaProperties sets the platform property shared by the service
// output types. The platform is a preview field, so without the experimental
// gate the property is removed and clients aren't told about it.
func setPlatformSchemaProperties(schema *jsonschema.Schema, experimental bool) {
	if !experimental {
		delete(schema.Properties, "platform")
		return
	}
	schema.Properties["platform"].Description = "Storage platform the service runs on"
	schema.Properties["platform"].Enum = []any{api.ServicePlatformSTANDARD, api.ServicePlatformFLUID}
}

// ResourceInfo represents resource allocation information
type ResourceInfo struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// setResourceInfoSchemaProperties enhances the schema of a ResourceInfo
// property nested inside a service output schema.
func setResourceInfoSchemaProperties(schema *jsonschema.Schema) {
	schema.Properties["cpu"].Description = "CPU allocation"
	schema.Properties["cpu"].Examples = []any{"0.5 cores", "1 core", "4 cores"}

	schema.Properties["memory"].Description = "Memory allocation"
	schema.Properties["memory"].Examples = []any{"2 GB", "4 GB", "16 GB"}
}

// ServiceDetail represents detailed service information
type ServiceDetail struct {
	ServiceID        string        `json:"id"`
	Name             string        `json:"name"`
	Status           string        `json:"status"`
	Platform         string        `json:"platform,omitempty"`
	Type             string        `json:"type"`
	Region           string        `json:"region"`
	Created          string        `json:"created,omitempty"`
	Environment      string        `json:"environment"`
	Resources        *ResourceInfo `json:"resources,omitempty"`
	Replicas         int           `json:"replicas"`
	DirectEndpoint   string        `json:"direct_endpoint,omitempty"`
	PoolerEndpoint   string        `json:"pooler_endpoint,omitempty"`
	Password         string        `json:"password,omitempty"`
	ConnectionString string        `json:"connection_string"`
}

func (ServiceDetail) Schema(experimental bool) *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceDetail](nil))

	schema.Properties["id"].Description = "Service identifier (10-character alphanumeric string)"

	schema.Properties["status"].Description = "Service status"
	schema.Properties["status"].Examples = []any{"READY", "PAUSED", "CONFIGURING", "UPGRADING"}

	setPlatformSchemaProperties(schema, experimental)

	schema.Properties["type"].Enum = util.AnySlice(validServiceTypes())

	schema.Properties["environment"].Description = "Environment tag (DEV or PROD). Under read_only=prod, services tagged PROD cannot be modified."

	setResourceInfoSchemaProperties(schema.Properties["resources"])

	schema.Properties["replicas"].Description = "Number of HA replicas (0=single node/no HA, 1+=HA enabled)"
	schema.Properties["direct_endpoint"].Description = "Direct database connection endpoint"
	schema.Properties["pooler_endpoint"].Description = "Connection pooler endpoint"
	schema.Properties["password"].Description = "Password for tsdbadmin user (only included if with_password=true)"
	schema.Properties["connection_string"].Description = "PostgreSQL connection string (password embedded only if with_password=true)"

	return schema
}

// convertToServiceDetail converts an API Service to MCP ServiceDetail
func (s *Server) convertToServiceDetail(cfg *config.Config, service api.Service, withPassword bool) ServiceDetail {
	detail := ServiceDetail{
		ServiceID: service.ServiceID,
		Name:      service.Name,
		Status:    string(service.Status),
		Type:      string(service.ServiceType),
		Region:    service.RegionCode,
		Created:   service.Created.Format("2006-01-02T15:04:05Z"),
		// The read_only=prod instructions tell the model to check this first.
		Environment: string(common.ServiceEnvironmentTag(service)),
	}

	// The platform is a preview field, returned only behind the gate.
	if s.app.Experimental {
		detail.Platform = string(util.Deref(service.Platform))
	}

	// Add resource information if available
	if len(service.Resources) > 0 {
		resource := service.Resources[0]
		if resource.Spec != nil {
			detail.Resources = &ResourceInfo{}

			if resource.Spec.CPUMillis != nil {
				detail.Resources.CPU = common.FormatCPUCores(*resource.Spec.CPUMillis) + " cores"
			} else {
				// CPU is null - this indicates a free tier service
				detail.Resources.CPU = "shared"
			}

			if resource.Spec.MemoryGbs != nil {
				detail.Resources.Memory = fmt.Sprintf("%d GB", *resource.Spec.MemoryGbs)
			} else {
				// Memory is null - this indicates a free tier service
				detail.Resources.Memory = "shared"
			}
		}
	}

	// Add replica information
	if service.HaReplicas != nil && service.HaReplicas.ReplicaCount != nil {
		detail.Replicas = *service.HaReplicas.ReplicaCount
	}

	// Add endpoint information
	if service.Endpoint != nil && service.Endpoint.Host != nil {
		port := "5432"
		if service.Endpoint.Port != nil {
			port = fmt.Sprintf("%d", *service.Endpoint.Port)
		}
		detail.DirectEndpoint = fmt.Sprintf("%s:%s", *service.Endpoint.Host, port)
	}

	// Add connection pooler endpoint
	if service.ConnectionPooler != nil && service.ConnectionPooler.Endpoint != nil && service.ConnectionPooler.Endpoint.Host != nil {
		port := "6432"
		if service.ConnectionPooler.Endpoint.Port != nil {
			port = fmt.Sprintf("%d", *service.ConnectionPooler.Endpoint.Port)
		}
		detail.PoolerEndpoint = fmt.Sprintf("%s:%s", *service.ConnectionPooler.Endpoint.Host, port)
	}

	// Include password in ServiceDetail if requested. Setting it here ensures
	// it's always set, even if GetConnectionDetails returns an error.
	if withPassword {
		// NOTE: This is a no-op if service.InitialPassword is nil or empty
		detail.Password = util.Deref(service.InitialPassword)
	}

	// Always include connection string in ServiceDetail
	// Password is embedded in connection string only if with_password=true
	if details, err := common.GetConnectionDetails(cfg, service, common.ConnectionDetailsOptions{
		Role:            "tsdbadmin",
		WithPassword:    withPassword,
		InitialPassword: util.Deref(service.InitialPassword),
		ReadOnly:        common.CheckReadOnly(cfg, common.ServiceEnvironmentTag(service)) != nil,
	}); err != nil {
		s.logger.Error("MCP: Failed to build connection string", slog.Any("error", err))
	} else {
		if withPassword && details.Password == "" {
			s.logger.Error("MCP: Requested password but password not available")
		}
		detail.ConnectionString = details.String()
		detail.Password = details.Password
	}

	return detail
}

// branchIDHelp is the sentence a db tool's service_id description gains for
// branches, saying what passing a branch ID does. Branches are a preview, so
// it is empty without the experimental gate.
func branchIDHelp(experimental bool, does string) string {
	if !experimental {
		return ""
	}
	return " A branch ID is also accepted — passing one " + does + " instead. Use branch_list to find branch IDs."
}

// setBranchIDSchemaProperties sets common branch_id schema properties. Like
// service_id, branch_id is IDs-only (see setServiceIDSchemaProperties), which
// resolveBranchByID enforces.
//
// TODO: confirm the branch ID format with the API. The pattern assumes branch
// IDs look like service IDs, which branches are on the platform side.
func setBranchIDSchemaProperties(schema *jsonschema.Schema) {
	schema.Properties["branch_id"].Description = "Unique identifier of the branch (10-character alphanumeric string). Use branch_list to find branch IDs."
	schema.Properties["branch_id"].Examples = []any{"br4nch1d9x", "k2m9x2sd3a"}
	schema.Properties["branch_id"].Pattern = "^[a-z0-9]{10}$"
}

// setBranchCPUMemorySchemaProperties sets a branch tool's cpu_memory property,
// offering only the combinations a branch can have.
func setBranchCPUMemorySchemaProperties(schema *jsonschema.Schema, description string) {
	schema.Properties["cpu_memory"].Description = description
	schema.Properties["cpu_memory"].Enum = util.AnySlice(common.BranchCPUMemoryConfigs().Strings())
}

// resolveBranchByID looks up the branch with the given ID, through the API's
// resolve endpoint, which also matches names. A match by name is reported as
// not found, keeping the tools IDs-only. The branch carries its parent's ID,
// which every other branch endpoint needs.
func resolveBranchByID(ctx context.Context, client api.ClientWithResponsesInterface, projectID, id string) (*api.Branch, error) {
	branch, err := common.ResolveBranch(ctx, client, projectID, id)
	if err != nil {
		return nil, err
	}
	if branch.ID != id {
		return nil, common.ExitWithCode(common.ExitServiceNotFound, fmt.Errorf("branch '%s' not found", id))
	}
	return branch, nil
}

// setBranchIOPSSchemaProperties sets a branch tool's iops input property.
// defaultDescription, if any, says what happens when iops is omitted. As for
// services, the allowed values aren't listed: they depend on too much to
// describe, and would go stale. The API validates the value.
func setBranchIOPSSchemaProperties(schema *jsonschema.Schema, defaultDescription string) {
	property := schema.Properties["iops"]
	property.Description = strings.TrimSpace("Provisioned IOPS for the branch's storage. " +
		"Check the Tiger Data documentation for the allowed values, which depend on the user's plan and the branch's CPU/memory allocation. " +
		defaultDescription)
	property.Minimum = new(0.0)
	property.Examples = []any{16000, 32000}
}

// BranchDetail represents detailed branch information
type BranchDetail struct {
	BranchID         string        `json:"id"`
	Name             string        `json:"name"`
	ServiceID        string        `json:"service_id"`
	Status           string        `json:"status"`
	Created          string        `json:"created,omitempty"`
	Resources        *ResourceInfo `json:"resources,omitempty"`
	IOPS             *int          `json:"iops,omitempty"`
	DirectEndpoint   string        `json:"direct_endpoint,omitempty"`
	ConnectionString string        `json:"connection_string,omitempty"`
}

func (BranchDetail) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[BranchDetail](nil))

	schema.Properties["id"].Description = "Branch identifier (10-character alphanumeric string)"
	schema.Properties["name"].Description = "Branch name, unique within its parent service"
	schema.Properties["service_id"].Description = "ID of the parent service the branch was created from"

	schema.Properties["status"].Description = "Branch status"
	schema.Properties["status"].Examples = []any{"READY", "PAUSED", "QUEUED", "CONFIGURING"}

	setResourceInfoSchemaProperties(schema.Properties["resources"])

	schema.Properties["iops"].Description = "Provisioned IOPS for the branch's storage"

	schema.Properties["direct_endpoint"].Description = "Direct database connection endpoint (absent while the branch is still provisioning)"
	schema.Properties["connection_string"].Description = "PostgreSQL connection string, without a password"

	return schema
}

// convertToBranchDetail converts an API Branch to MCP BranchDetail
func (s *Server) convertToBranchDetail(cfg *config.Config, branch api.Branch) BranchDetail {
	detail := BranchDetail{
		BranchID:  branch.ID,
		Name:      branch.Name,
		ServiceID: branch.ServiceID,
		Status:    string(branch.Status),
		Created:   branch.Created.Format("2006-01-02T15:04:05Z"),
		Resources: &ResourceInfo{
			CPU:    common.FormatCPUCores(branch.CPUMillis) + " cores",
			Memory: fmt.Sprintf("%d GB", branch.MemoryGbs),
		},
		IOPS: branch.Iops,
	}

	// A branch that's still provisioning may have no endpoint yet.
	if branch.Endpoint == nil {
		return detail
	}
	if branch.Endpoint.Host != nil {
		port := "5432"
		if branch.Endpoint.Port != nil {
			port = fmt.Sprintf("%d", *branch.Endpoint.Port)
		}
		detail.DirectEndpoint = fmt.Sprintf("%s:%s", *branch.Endpoint.Host, port)
	}
	if details, err := common.GetConnectionDetails(cfg, common.BranchAsService(branch), common.ConnectionDetailsOptions{
		Role:     "tsdbadmin",
		ReadOnly: common.CheckReadOnly(cfg, common.BranchEnvironmentTag) != nil,
	}); err != nil {
		s.logger.Error("MCP: Failed to build connection string", slog.Any("error", err))
	} else {
		detail.ConnectionString = details.String()
	}

	return detail
}

// parseBranchCPUMemory returns the branch configuration a cpu_memory value
// names. The schema's enum already rejects anything else, so an error here
// means the two have drifted apart.
func parseBranchCPUMemory(cpuMemory string) (common.CPUMemoryConfig, error) {
	for _, c := range common.BranchCPUMemoryConfigs() {
		if c.String() == cpuMemory {
			return c, nil
		}
	}
	return common.CPUMemoryConfig{}, fmt.Errorf("invalid CPU/Memory specification '%s'. Valid options: %s", cpuMemory, common.BranchCPUMemoryConfigs())
}
