package util

import (
	"fmt"
	"strconv"
	"strings"
)

// CPUMemoryConfig represents an allowed CPU/Memory configuration
type CPUMemoryConfig struct {
	Shared    bool // Shared CPU/Memory
	CPUMillis int  // CPU in millicores
	MemoryGBs int  // Memory in GB
}

func (c *CPUMemoryConfig) Matches(cpuMillis, memoryGBs string) (string, string, bool) {
	if c.Shared {
		if (cpuMillis == "shared" && memoryGBs == "shared") ||
			(cpuMillis == "shared" && memoryGBs == "") ||
			(cpuMillis == "" && memoryGBs == "shared") {
			return "shared", "shared", true
		}
	}

	cpuMillisStr := strconv.Itoa(c.CPUMillis)
	memoryGBsStr := strconv.Itoa(c.MemoryGBs)

	if (cpuMillis == cpuMillisStr && memoryGBs == memoryGBsStr) ||
		(cpuMillis == cpuMillisStr && memoryGBs == "") ||
		(cpuMillis == "" && memoryGBs == memoryGBsStr) {
		return cpuMillisStr, memoryGBsStr, true
	}

	return "", "", false
}

func (c *CPUMemoryConfig) String() string {
	if c.Shared {
		return "shared/shared"
	}

	cpuCores := float64(c.CPUMillis) / 1000
	if cpuCores == float64(int(cpuCores)) {
		return fmt.Sprintf("%.0f CPU/%d GB", cpuCores, c.MemoryGBs)
	}
	return fmt.Sprintf("%.1f CPU/%d GB", cpuCores, c.MemoryGBs)
}

type CPUMemoryConfigs []CPUMemoryConfig

// GetAllowedCPUMemoryConfigs returns the allowed CPU/Memory configurations from the spec
func GetAllowedCPUMemoryConfigs() CPUMemoryConfigs {
	return CPUMemoryConfigs{
		{Shared: true},                     // shared CPU, shared memory
		{CPUMillis: 500, MemoryGBs: 2},     // 0.5 CPU, 2GB
		{CPUMillis: 1000, MemoryGBs: 4},    // 1 CPU, 4GB
		{CPUMillis: 2000, MemoryGBs: 8},    // 2 CPU, 8GB
		{CPUMillis: 4000, MemoryGBs: 16},   // 4 CPU, 16GB
		{CPUMillis: 8000, MemoryGBs: 32},   // 8 CPU, 32GB
		{CPUMillis: 16000, MemoryGBs: 64},  // 16 CPU, 64GB
		{CPUMillis: 32000, MemoryGBs: 128}, // 32 CPU, 128GB
	}
}

// Strings returns a slice of user-friendly strings of allowed CPU/Memory combinations
func (c CPUMemoryConfigs) Strings() []string {
	strs := make([]string, 0, len(c))
	for _, config := range c {
		strs = append(strs, config.String())
	}
	return strs
}

// String returns a user-friendly string of allowed CPU/Memory combinations
func (c CPUMemoryConfigs) String() string {
	return strings.Join(c.Strings(), ", ")
}

// ValidateAndNormalizeCPUMemory validates CPU/Memory values and applies auto-configuration logic
func ValidateAndNormalizeCPUMemory(cpuMillis, memoryGBs string) (*string, *string, error) {
	// Return nil for omitted CPU/memory so that values are omitted from the API request
	if cpuMillis == "" && memoryGBs == "" {
		return nil, nil, nil
	}

	configs := GetAllowedCPUMemoryConfigs()
	for _, config := range configs {
		if cpuStr, memoryStr, ok := config.Matches(cpuMillis, memoryGBs); ok {
			return &cpuStr, &memoryStr, nil
		}
	}

	// If no match, provide helpful error
	return nil, nil, fmt.Errorf("invalid CPU/Memory combination. Allowed combinations: %s", configs)
}

// ParseCPUMemory parses a CPU/memory combination string (e.g., "2 CPU/8GB")
// and returns millicores and GB. If "shared" is given, returns "shared" for
// both CPU and memory.
func ParseCPUMemory(cpuMemoryStr string) (string, string, error) {
	// Get allowed configurations
	configs := GetAllowedCPUMemoryConfigs()

	// Find matching configuration by comparing string representation
	for _, config := range configs {
		if config.String() == cpuMemoryStr {
			if config.Shared {
				return "shared", "shared", nil
			}
			return strconv.Itoa(config.CPUMillis), strconv.Itoa(config.MemoryGBs), nil
		}
	}

	// If no match found, return error with valid options
	return "", "", fmt.Errorf("invalid CPU/Memory combination '%s'. Valid options: %s", cpuMemoryStr, configs.String())
}
