package mcp

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// convertToServiceInfo converts an API Service to MCP ServiceInfo
func (s *Server) convertToServiceInfo(service api.Service) ServiceInfo {
	info := ServiceInfo{
		ServiceID: util.Deref(service.ServiceId),
		Name:      util.Deref(service.Name),
		Status:    util.DerefStr(service.Status),
		Type:      util.DerefStr(service.ServiceType),
		Region:    util.Deref(service.RegionCode),
	}

	// Add creation time if available
	if service.Created != nil {
		info.Created = service.Created.Format("2006-01-02T15:04:05Z")
	}

	// Add resource information if available
	if service.Resources != nil && len(*service.Resources) > 0 {
		resource := (*service.Resources)[0]
		if resource.Spec != nil {
			info.Resources = &ResourceInfo{}

			if resource.Spec.CpuMillis != nil {
				cpuCores := float64(*resource.Spec.CpuMillis) / 1000
				if cpuCores == float64(int(cpuCores)) {
					info.Resources.CPU = fmt.Sprintf("%.0f cores", cpuCores)
				} else {
					info.Resources.CPU = fmt.Sprintf("%.1f cores", cpuCores)
				}
			} else {
				// CPU is null - this indicates a free tier service
				info.Resources.CPU = "shared"
			}

			if resource.Spec.MemoryGbs != nil {
				info.Resources.Memory = fmt.Sprintf("%d GB", *resource.Spec.MemoryGbs)
			} else {
				// Memory is null - this indicates a free tier service
				info.Resources.Memory = "shared"
			}
		}
	}

	return info
}

// convertToServiceDetail converts an API Service to MCP ServiceDetail
func (s *Server) convertToServiceDetail(service api.Service, withPassword bool) ServiceDetail {
	detail := ServiceDetail{
		ServiceID: util.Deref(service.ServiceId),
		Name:      util.Deref(service.Name),
		Status:    util.DerefStr(service.Status),
		Type:      util.DerefStr(service.ServiceType),
		Region:    util.Deref(service.RegionCode),
		Paused:    util.Deref(service.Paused),
	}

	// Add creation time if available
	if service.Created != nil {
		detail.Created = service.Created.Format("2006-01-02T15:04:05Z")
	}

	// Add resource information if available
	if service.Resources != nil && len(*service.Resources) > 0 {
		resource := (*service.Resources)[0]
		if resource.Spec != nil {
			detail.Resources = &ResourceInfo{}

			if resource.Spec.CpuMillis != nil {
				cpuCores := float64(*resource.Spec.CpuMillis) / 1000
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
	if details, err := common.GetConnectionDetails(service, common.ConnectionDetailsOptions{
		Role:            "tsdbadmin",
		WithPassword:    withPassword,
		InitialPassword: util.Deref(service.InitialPassword),
	}); err != nil {
		logging.Error("MCP: Failed to build connection string", zap.Error(err))
	} else {
		if withPassword && details.Password == "" {
			logging.Error("MCP: Requested password but password not available")
		}
		detail.ConnectionString = details.String()
		detail.Password = details.Password
	}

	return detail
}
