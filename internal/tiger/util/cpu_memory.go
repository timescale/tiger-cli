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

func (c *CPUMemoryConfig) String() string {
	if c.Shared {
		return "shared"
	}

	cpuCores := float64(c.CPUMillis) / 1000
	if cpuCores == float64(int(cpuCores)) {
		return fmt.Sprintf("%.0f CPU/%s", cpuCores, c.MemoryString())
	}
	return fmt.Sprintf("%.1f CPU/%s", cpuCores, c.MemoryString())
}

func (c *CPUMemoryConfig) CPUString() string {
	if c.Shared {
		return "shared"
	}

	cpuCores := float64(c.CPUMillis) / 1000
	if cpuCores == float64(int(cpuCores)) {
		return fmt.Sprintf("%.0f (%.0fm)", cpuCores, float64(c.CPUMillis))
	}
	return fmt.Sprintf("%.1f (%.0fm)", cpuCores, float64(c.CPUMillis))
}

func (c *CPUMemoryConfig) MemoryString() string {
	if c.Shared {
		return "shared"
	}

	return fmt.Sprintf("%dGB", c.MemoryGBs)
}

type CPUMemoryConfigs []CPUMemoryConfig

// GetAllowedCPUMemoryConfigs returns the allowed CPU/Memory configurations from the spec
// TODO: It would be great if we could fetch these from the API instead of hard coding them.
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

// String returns a user-friendly string of allowed CPU values
func (c CPUMemoryConfigs) CPUString() string {
	cpuValues := make([]string, 0, len(c))
	for _, config := range c {
		cpuValues = append(cpuValues, config.CPUString())
	}
	return strings.Join(cpuValues, ", ")
}

// String returns a user-friendly string of allowed memory values
func (c CPUMemoryConfigs) MemoryString() string {
	memoryValues := make([]string, 0, len(c))
	for _, config := range c {
		memoryValues = append(memoryValues, config.MemoryString())
	}
	return strings.Join(memoryValues, ", ")
}

// ValidateAndNormalizeCPUMemory validates CPU/Memory values and applies auto-configuration logic
func ValidateAndNormalizeCPUMemory(cpuMillis int, memoryGBs int, cpuFlagSet, memoryFlagSet bool) (int, int, error) {
	configs := GetAllowedCPUMemoryConfigs()

	// If both CPU and memory were explicitly set, validate they match an allowed configuration
	if cpuFlagSet && memoryFlagSet {
		for _, config := range configs {
			if config.CPUMillis == cpuMillis && config.MemoryGBs == memoryGBs {
				return cpuMillis, memoryGBs, nil
			}
		}
		// If no exact match, provide helpful error
		return 0, 0, fmt.Errorf(
			"invalid CPU/Memory combination: %dm CPU and %dGB memory. Allowed combinations: %s",
			cpuMillis, memoryGBs, configs,
		)
	}

	// If only CPU was explicitly set, find matching memory and auto-configure
	if cpuFlagSet && !memoryFlagSet {
		for _, config := range configs {
			if config.CPUMillis == cpuMillis {
				return cpuMillis, config.MemoryGBs, nil
			}
		}
		return 0, 0, fmt.Errorf(
			"invalid CPU allocation: %dm. Allowed CPU values: %s",
			cpuMillis, configs.CPUString(),
		)
	}

	// If only memory was explicitly set, find matching CPU and auto-configure
	if !cpuFlagSet && memoryFlagSet {
		for _, config := range configs {
			if config.MemoryGBs == memoryGBs {
				return config.CPUMillis, memoryGBs, nil
			}
		}
		return 0, 0, fmt.Errorf(
			"invalid memory allocation: %dGB. Allowed memory values: %s",
			memoryGBs, configs.MemoryString(),
		)
	}

	// If neither flag was explicitly set, use default values (0.5 CPU, 2GB)
	return 500, 2, nil
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
