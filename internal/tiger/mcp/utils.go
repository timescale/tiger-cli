package mcp

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
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
			}

			if resource.Spec.MemoryGbs != nil {
				info.Resources.Memory = fmt.Sprintf("%d GB", *resource.Spec.MemoryGbs)
			}
		}
	}

	return info
}

// convertToServiceDetail converts an API Service to MCP ServiceDetail
func (s *Server) convertToServiceDetail(service api.Service) ServiceDetail {
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
			}

			if resource.Spec.MemoryGbs != nil {
				detail.Resources.Memory = fmt.Sprintf("%d GB", *resource.Spec.MemoryGbs)
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

	return detail
}

// waitForServiceReady polls the service status until it's ready or timeout occurs
// Returns the final service status and any error that occurred
func (s *Server) waitForServiceReady(apiClient *api.ClientWithResponses, projectID, serviceID string, timeout time.Duration, initialStatus string) (string, error) {
	logging.Debug("MCP: Waiting for service to be ready",
		zap.String("service_id", serviceID),
		zap.Duration("timeout", timeout),
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	lastSeenStatus := initialStatus
	for {
		select {
		case <-ctx.Done():
			logging.Warn("MCP: Timed out while waiting for service to be ready", zap.Error(ctx.Err()))
			return lastSeenStatus, fmt.Errorf("timeout reached after %v - service may still be provisioning", timeout)
		case <-ticker.C:
			resp, err := apiClient.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
			if err != nil {
				logging.Warn("MCP: Error checking service status", zap.Error(err))
				continue
			}

			if resp.StatusCode() != 200 || resp.JSON200 == nil {
				logging.Warn("MCP: Service not found or error checking status", zap.Int("status_code", resp.StatusCode()))
				continue
			}

			service := *resp.JSON200
			status := util.DerefStr(service.Status)
			lastSeenStatus = status // Track the last status we saw

			switch status {
			case "READY":
				logging.Debug("MCP: Service is ready", zap.String("service_id", serviceID))
				return status, nil
			case "FAILED", "ERROR":
				return status, fmt.Errorf("service creation failed with status: %s", status)
			default:
				logging.Debug("MCP: Service status",
					zap.String("service_id", serviceID),
					zap.String("status", status),
				)
			}
		}
	}
}
