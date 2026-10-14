package mcp

import (
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

// setIOPSSchemaProperties sets a tool's iops input property. IOPS are a
// preview parameter, so without the experimental gate the property is removed,
// and the schema rejects it as an unknown property. defaultDescription, if
// any, says what happens when iops is omitted. The API validates the value, since the
// allowed range depends on the plan, which the tool can't see.
func setIOPSSchemaProperties(schema *jsonschema.Schema, experimental bool, defaultDescription string) {
	if !experimental {
		delete(schema.Properties, "iops")
		return
	}
	property := schema.Properties["iops"]
	property.Description = strings.TrimSpace(fmt.Sprintf("Provisioned IOPS for the service's storage, in increments of 1000. "+
		"Only valid for services on the fluid platform. The allowed range depends on the user's plan and the service's CPU allocation. "+
		"Across all plans, it spans: %s. %s", common.FluidIOPSRangesString(), defaultDescription))
	property.Minimum = new(0.0)
	property.Examples = []any{6000, 12000}
}

// ResourceInfo represents resource allocation information
type ResourceInfo struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	IOPS   int    `json:"iops,omitempty"`
}

// setResourceInfoSchemaProperties enhances the schema of a ResourceInfo
// property nested inside a service output schema. IOPS are a preview field, so
// without the experimental gate the property is removed.
func setResourceInfoSchemaProperties(schema *jsonschema.Schema, experimental bool) {
	schema.Properties["cpu"].Description = "CPU allocation"
	schema.Properties["cpu"].Examples = []any{"0.5 cores", "1 core", "4 cores"}

	schema.Properties["memory"].Description = "Memory allocation"
	schema.Properties["memory"].Examples = []any{"2 GB", "4 GB", "16 GB"}

	if experimental {
		schema.Properties["iops"].Description = "Provisioned IOPS for the service's storage"
		schema.Properties["iops"].Examples = []any{3000, 12000}
	} else {
		delete(schema.Properties, "iops")
	}
}

// convertToResourceInfo converts a service's resource allocation to its MCP
// output form, or nil if the API didn't return one. The IOPS are a preview
// field, returned only if experimental is set.
func convertToResourceInfo(service api.Service, experimental bool) *ResourceInfo {
	if len(service.Resources) == 0 || service.Resources[0].Spec == nil {
		return nil
	}
	spec := service.Resources[0].Spec
	info := &ResourceInfo{}

	if spec.CPUMillis != nil {
		info.CPU = common.FormatCPUCores(*spec.CPUMillis) + " cores"
	} else {
		// CPU is null - this indicates a free tier service
		info.CPU = "shared"
	}

	if spec.MemoryGbs != nil {
		info.Memory = fmt.Sprintf("%d GB", *spec.MemoryGbs)
	} else {
		// Memory is null - this indicates a free tier service
		info.Memory = "shared"
	}

	if experimental {
		info.IOPS = util.Deref(spec.Iops)
	}

	return info
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

	setResourceInfoSchemaProperties(schema.Properties["resources"], experimental)

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

	detail.Resources = convertToResourceInfo(service, s.app.Experimental)

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
