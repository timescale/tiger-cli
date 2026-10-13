package common

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/timescale/tiger-cli/internal/api"
)

// CPUMemoryConfig represents an allowed CPU/Memory configuration
type CPUMemoryConfig struct {
	Shared    bool // Shared CPU/Memory
	CPUMillis int  // CPU in millicores
	MemoryGBs int  // Memory in GB
}

func (c *CPUMemoryConfig) Matches(cpuMillis, memoryGBs string) bool {
	if c.Shared {
		if (cpuMillis == "shared" && memoryGBs == "shared") ||
			(cpuMillis == "shared" && memoryGBs == "") ||
			(cpuMillis == "" && memoryGBs == "shared") {
			return true
		}
	}

	cpuMillisStr := strconv.Itoa(c.CPUMillis)
	memoryGBsStr := strconv.Itoa(c.MemoryGBs)

	if (cpuMillis == cpuMillisStr && memoryGBs == memoryGBsStr) ||
		(cpuMillis == cpuMillisStr && memoryGBs == "") ||
		(cpuMillis == "" && memoryGBs == memoryGBsStr) {
		return true
	}

	return false
}

func (c *CPUMemoryConfig) CPUMillisString() *string {
	if c == nil {
		return nil
	}

	str := "shared"
	if !c.Shared {
		str = strconv.Itoa(c.CPUMillis)
	}
	return &str
}

func (c *CPUMemoryConfig) MemoryGBsString() *string {
	if c == nil {
		return nil
	}

	str := "shared"
	if !c.Shared {
		str = strconv.Itoa(c.MemoryGBs)
	}
	return &str
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

// StandardCPUMemoryConfigs returns the allowed CPU/Memory configurations for
// services on the standard platform.
func StandardCPUMemoryConfigs() CPUMemoryConfigs {
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

// FluidCPUMemoryConfigs returns the allowed CPU/Memory configurations for
// services on the fluid platform: 8GB of memory per CPU, with no shared option.
func FluidCPUMemoryConfigs() CPUMemoryConfigs {
	return CPUMemoryConfigs{
		{CPUMillis: 500, MemoryGBs: 4},     // 0.5 CPU, 4GB
		{CPUMillis: 1000, MemoryGBs: 8},    // 1 CPU, 8GB
		{CPUMillis: 2000, MemoryGBs: 16},   // 2 CPU, 16GB
		{CPUMillis: 4000, MemoryGBs: 32},   // 4 CPU, 32GB
		{CPUMillis: 8000, MemoryGBs: 64},   // 8 CPU, 64GB
		{CPUMillis: 16000, MemoryGBs: 128}, // 16 CPU, 128GB
		{CPUMillis: 32000, MemoryGBs: 256}, // 32 CPU, 256GB
	}
}

// CPUMemoryConfigsFor returns the allowed CPU/Memory configurations for a
// platform. An empty platform is standard, as the API defaults it to.
func CPUMemoryConfigsFor(platform api.ServicePlatform) CPUMemoryConfigs {
	if platform == api.ServicePlatformFLUID {
		return FluidCPUMemoryConfigs()
	}
	return StandardCPUMemoryConfigs()
}

// WithoutShared returns the configurations other than shared/shared, which a
// service can't be resized onto.
func (c CPUMemoryConfigs) WithoutShared() CPUMemoryConfigs {
	filtered := make(CPUMemoryConfigs, 0, len(c))
	for _, config := range c {
		if !config.Shared {
			filtered = append(filtered, config)
		}
	}
	return filtered
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

// ValidateAndNormalize validates CPU/Memory values against the configurations
// and fills in whichever one was omitted from the matching configuration.
func (c CPUMemoryConfigs) ValidateAndNormalize(cpuMillis, memoryGBs string) (*CPUMemoryConfig, error) {
	// Return nil for omitted CPU/memory so that values are omitted from the API request
	if cpuMillis == "" && memoryGBs == "" {
		return nil, nil
	}

	for _, config := range c {
		if config.Matches(cpuMillis, memoryGBs) {
			return &config, nil
		}
	}

	// If no match, provide helpful error
	return nil, fmt.Errorf("invalid CPU/Memory combination. Allowed combinations: %s", c)
}

// Parse parses a CPU/memory combination string (e.g., "2 CPU/8 GB") from the
// configurations and returns millicores and GB. If "shared" is given, returns
// "shared" for both CPU and memory.
func (c CPUMemoryConfigs) Parse(cpuMemoryStr string) (string, string, error) {
	for _, config := range c {
		if config.String() == cpuMemoryStr {
			if config.Shared {
				return "shared", "shared", nil
			}
			return strconv.Itoa(config.CPUMillis), strconv.Itoa(config.MemoryGBs), nil
		}
	}

	// If no match found, return error with valid options
	return "", "", fmt.Errorf("invalid CPU/Memory combination '%s'. Valid options: %s", cpuMemoryStr, c.String())
}
