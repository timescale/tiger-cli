package util

import (
	"fmt"
	"strings"
)

func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func Deref[T any](val *T) T {
	if val == nil {
		var res T
		return res
	}
	return *val
}

func DerefStr[T ~string](val *T) string {
	if val == nil {
		return ""
	}
	return string(*val)
}

// CPUMemoryConfig represents an allowed CPU/Memory configuration
type CPUMemoryConfig struct {
	CPUMillis int     // CPU in millicores
	MemoryGbs float64 // Memory in GB
}

type CPUMemoryConfigs []CPUMemoryConfig

// GetAllowedCPUMemoryConfigs returns the allowed CPU/Memory configurations from the spec
func GetAllowedCPUMemoryConfigs() CPUMemoryConfigs {
	return CPUMemoryConfigs{
		{CPUMillis: 500, MemoryGbs: 2},     // 0.5 CPU, 2GB
		{CPUMillis: 1000, MemoryGbs: 4},    // 1 CPU, 4GB
		{CPUMillis: 2000, MemoryGbs: 8},    // 2 CPU, 8GB
		{CPUMillis: 4000, MemoryGbs: 16},   // 4 CPU, 16GB
		{CPUMillis: 8000, MemoryGbs: 32},   // 8 CPU, 32GB
		{CPUMillis: 16000, MemoryGbs: 64},  // 16 CPU, 64GB
		{CPUMillis: 32000, MemoryGbs: 128}, // 32 CPU, 128GB
	}
}

// String returns a user-friendly string of allowed CPU/Memory combinations
func (c CPUMemoryConfigs) String() string {
	combinations := make([]string, 0, len(c))
	for _, config := range c {
		cpuCores := float64(config.CPUMillis) / 1000
		if cpuCores == float64(int(cpuCores)) {
			combinations = append(combinations, fmt.Sprintf("%.0f CPU/%.0fGB", cpuCores, config.MemoryGbs))
		} else {
			combinations = append(combinations, fmt.Sprintf("%.1f CPU/%.0fGB", cpuCores, config.MemoryGbs))
		}
	}
	return strings.Join(combinations, ", ")
}

// String returns a user-friendly string of allowed CPU values
func (c CPUMemoryConfigs) CPUString() string {
	var cpuValues []string
	for _, config := range c {
		cpuCores := float64(config.CPUMillis) / 1000
		if cpuCores == float64(int(cpuCores)) {
			cpuValues = append(cpuValues, fmt.Sprintf("%.0f (%.0fm)", cpuCores, float64(config.CPUMillis)))
		} else {
			cpuValues = append(cpuValues, fmt.Sprintf("%.1f (%.0fm)", cpuCores, float64(config.CPUMillis)))
		}
	}
	return strings.Join(cpuValues, ", ")
}

// String returns a user-friendly string of allowed memory values
func (c CPUMemoryConfigs) MemoryString() string {
	var memoryValues []string
	for _, config := range c {
		memoryValues = append(memoryValues, fmt.Sprintf("%.0fGB", config.MemoryGbs))
	}
	return strings.Join(memoryValues, ", ")
}

// ValidateAndNormalizeCPUMemory validates CPU/Memory values and applies auto-configuration logic
func ValidateAndNormalizeCPUMemory(cpuMillis int, memoryGbs float64, cpuFlagSet, memoryFlagSet bool) (int, float64, error) {
	configs := GetAllowedCPUMemoryConfigs()

	// If both CPU and memory were explicitly set, validate they match an allowed configuration
	if cpuFlagSet && memoryFlagSet {
		for _, config := range configs {
			if config.CPUMillis == cpuMillis && config.MemoryGbs == memoryGbs {
				return cpuMillis, memoryGbs, nil
			}
		}
		// If no exact match, provide helpful error
		return 0, 0, fmt.Errorf(
			"invalid CPU/Memory combination: %dm CPU and %.0fGB memory. Allowed combinations: %s",
			cpuMillis, memoryGbs, configs,
		)
	}

	// If only CPU was explicitly set, find matching memory and auto-configure
	if cpuFlagSet && !memoryFlagSet {
		for _, config := range configs {
			if config.CPUMillis == cpuMillis {
				return cpuMillis, config.MemoryGbs, nil
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
			if config.MemoryGbs == memoryGbs {
				return config.CPUMillis, memoryGbs, nil
			}
		}
		return 0, 0, fmt.Errorf(
			"invalid memory allocation: %.0fGB. Allowed memory values: %s",
			memoryGbs, configs.MemoryString(),
		)
	}

	// If neither flag was explicitly set, use default values (0.5 CPU, 2GB)
	return 500, 2, nil
}
