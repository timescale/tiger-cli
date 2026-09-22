package mcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
// service_list, and a name can be ambiguous or shadowed — not something to
// guess at when the tool on the other end is destructive. The API itself
// accepts either; the pattern below is what declines them here.
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

func (ServiceDetail) Schema() *jsonschema.Schema {
	schema := util.Must(jsonschema.For[ServiceDetail](nil))

	schema.Properties["id"].Description = "Service identifier (10-character alphanumeric string)"

	schema.Properties["status"].Description = "Service status"
	schema.Properties["status"].Examples = []any{"READY", "PAUSED", "CONFIGURING", "UPGRADING"}

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

	// Add resource information if available
	if len(service.Resources) > 0 {
		resource := service.Resources[0]
		if resource.Spec != nil {
			detail.Resources = &ResourceInfo{}

			if resource.Spec.CPUMillis != nil {
				cpuCores := float64(*resource.Spec.CPUMillis) / 1000
				if cpuCores == float64(int(cpuCores)) {
					detail.Resources.CPU = fmt.Sprintf("%.0f cores", cpuCores)
				} else {
					detail.Resources.CPU = fmt.Sprintf("%.1f cores", cpuCores)
				}
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
