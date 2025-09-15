package mcp

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
)

// derefString safely dereferences a string pointer, returning empty string if nil
func (s *Server) derefString(str *string) string {
	if str == nil {
		return ""
	}
	return *str
}

// derefInt safely dereferences an int pointer, returning 0 if nil
func (s *Server) derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

// derefBool safely dereferences a bool pointer, returning false if nil
func (s *Server) derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

// convertToServiceInfo converts an API Service to MCP ServiceInfo
func (s *Server) convertToServiceInfo(service api.Service) ServiceInfo {
	info := ServiceInfo{
		ServiceID: s.derefString(service.ServiceId),
		Name:      s.derefString(service.Name),
		Status:    s.formatDeployStatus(service.Status),
		Type:      s.formatServiceType(service.ServiceType),
		Region:    s.derefString(service.RegionCode),
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
		ServiceID: s.derefString(service.ServiceId),
		Name:      s.derefString(service.Name),
		Status:    s.formatDeployStatus(service.Status),
		Type:      s.formatServiceType(service.ServiceType),
		Region:    s.derefString(service.RegionCode),
		Paused:    s.derefBool(service.Paused),
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

// formatDeployStatus formats a DeployStatus pointer, returning empty string if nil
func (s *Server) formatDeployStatus(status *api.DeployStatus) string {
	if status == nil {
		return ""
	}
	return string(*status)
}

// formatServiceType formats a ServiceType pointer, returning empty string if nil
func (s *Server) formatServiceType(serviceType *api.ServiceType) string {
	if serviceType == nil {
		return ""
	}
	return string(*serviceType)
}

// CPUMemoryConfig represents an allowed CPU/Memory configuration
type CPUMemoryConfig struct {
	CPUMillis int     // CPU in millicores
	MemoryGbs float64 // Memory in GB
}

// getAllowedCPUMemoryConfigs returns the allowed CPU/Memory configurations from the spec
func (s *Server) getAllowedCPUMemoryConfigs() []CPUMemoryConfig {
	return []CPUMemoryConfig{
		{CPUMillis: 500, MemoryGbs: 2},     // 0.5 CPU, 2GB
		{CPUMillis: 1000, MemoryGbs: 4},    // 1 CPU, 4GB
		{CPUMillis: 2000, MemoryGbs: 8},    // 2 CPU, 8GB
		{CPUMillis: 4000, MemoryGbs: 16},   // 4 CPU, 16GB
		{CPUMillis: 8000, MemoryGbs: 32},   // 8 CPU, 32GB
		{CPUMillis: 16000, MemoryGbs: 64},  // 16 CPU, 64GB
		{CPUMillis: 32000, MemoryGbs: 128}, // 32 CPU, 128GB
	}
}

// parseCPUMemory parses CPU and memory specifications and returns normalized values
func (s *Server) parseCPUMemory(cpuStr, memoryStr string) (int, float64, error) {
	// Parse CPU
	cpuMillis, err := s.parseCPU(cpuStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid CPU specification '%s': %w", cpuStr, err)
	}

	// Parse Memory
	memoryGbs, err := s.parseMemory(memoryStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid memory specification '%s': %w", memoryStr, err)
	}

	// Validate the combination is allowed
	configs := s.getAllowedCPUMemoryConfigs()
	for _, config := range configs {
		if config.CPUMillis == cpuMillis && config.MemoryGbs == memoryGbs {
			return cpuMillis, memoryGbs, nil
		}
	}

	return 0, 0, fmt.Errorf("invalid CPU/Memory combination: %dm CPU and %.0fGB memory. Allowed combinations: %s",
		cpuMillis, memoryGbs, s.formatAllowedCombinations(configs))
}

// parseCPU parses a CPU specification (e.g., "2", "2000m", "0.5")
func (s *Server) parseCPU(cpuStr string) (int, error) {
	cpuStr = strings.TrimSpace(cpuStr)

	// Handle millicores (e.g., "2000m")
	if strings.HasSuffix(cpuStr, "m") {
		milliStr := strings.TrimSuffix(cpuStr, "m")
		return strconv.Atoi(milliStr)
	}

	// Handle cores (e.g., "2", "0.5")
	cores, err := strconv.ParseFloat(cpuStr, 64)
	if err != nil {
		return 0, err
	}

	return int(cores * 1000), nil
}

// parseMemory parses a memory specification (e.g., "8GB", "4096MB")
func (s *Server) parseMemory(memoryStr string) (float64, error) {
	memoryStr = strings.TrimSpace(strings.ToUpper(memoryStr))

	// Regular expression to parse memory with units
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)(GB|MB|G|M)?$`)
	matches := re.FindStringSubmatch(memoryStr)

	if len(matches) < 2 {
		return 0, fmt.Errorf("invalid format")
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}

	unit := matches[2]
	if unit == "" {
		unit = "GB" // Default to GB if no unit specified
	}

	switch unit {
	case "GB", "G":
		return value, nil
	case "MB", "M":
		return value / 1024, nil
	default:
		return 0, fmt.Errorf("unsupported unit: %s", unit)
	}
}

// formatAllowedCombinations returns a user-friendly string of allowed CPU/Memory combinations
func (s *Server) formatAllowedCombinations(configs []CPUMemoryConfig) string {
	var combinations []string
	for _, config := range configs {
		cpuCores := float64(config.CPUMillis) / 1000
		if cpuCores == float64(int(cpuCores)) {
			combinations = append(combinations, fmt.Sprintf("%.0f CPU/%.0fGB", cpuCores, config.MemoryGbs))
		} else {
			combinations = append(combinations, fmt.Sprintf("%.1f CPU/%.0fGB", cpuCores, config.MemoryGbs))
		}
	}
	return strings.Join(combinations, ", ")
}

// waitForServiceReady polls the service status until it's ready or timeout occurs
func (s *Server) waitForServiceReady(ctx context.Context, projectID, serviceID string, timeout time.Duration) error {
	logging.Debug("MCP: Waiting for service to be ready",
		zap.String("service_id", serviceID),
		zap.Duration("timeout", timeout))

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout reached after %v - service may still be provisioning", timeout)
		case <-ticker.C:
			resp, err := s.apiClient.GetProjectsProjectIdServicesServiceIdWithResponse(ctx, projectID, serviceID)
			if err != nil {
				logging.Warn("MCP: Error checking service status", zap.Error(err))
				continue
			}

			if resp.StatusCode() != 200 || resp.JSON200 == nil {
				logging.Warn("MCP: Service not found or error checking status", zap.Int("status_code", resp.StatusCode()))
				continue
			}

			service := *resp.JSON200
			status := s.formatDeployStatus(service.Status)

			switch status {
			case "READY":
				logging.Debug("MCP: Service is ready", zap.String("service_id", serviceID))
				return nil
			case "FAILED", "ERROR":
				return fmt.Errorf("service creation failed with status: %s", status)
			default:
				logging.Debug("MCP: Service status",
					zap.String("service_id", serviceID),
					zap.String("status", status))
			}
		}
	}
}
