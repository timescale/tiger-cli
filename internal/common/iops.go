package common

import (
	"fmt"
	"strings"
)

// IOPSIncrement is the step provisioned IOPS are configured in.
const IOPSIncrement = 1000

// IOPSRange is the range of provisioned IOPS a fluid service of a given CPU
// allocation can be configured with.
type IOPSRange struct {
	CPUMillis int
	MinIOPS   int
	MaxIOPS   int
}

// FluidIOPSRanges returns the provisioned IOPS range for each fluid CPU
// allocation, in the order of FluidCPUMemoryConfigs. The real range depends on
// the project's plan too, which the API doesn't expose, so each range spans
// every plan: from the lowest plan's baseline to the highest plan's maximum.
// They're used only for help text and completion; the API validates the value.
func FluidIOPSRanges() []IOPSRange {
	return []IOPSRange{
		{CPUMillis: 250, MinIOPS: 3000, MaxIOPS: 3000},
		{CPUMillis: 500, MinIOPS: 4000, MaxIOPS: 4000},
		{CPUMillis: 1000, MinIOPS: 6000, MaxIOPS: 12000},
		{CPUMillis: 2000, MinIOPS: 6000, MaxIOPS: 16000},
		{CPUMillis: 4000, MinIOPS: 6000, MaxIOPS: 48000},
		{CPUMillis: 8000, MinIOPS: 6000, MaxIOPS: 64000},
		{CPUMillis: 16000, MinIOPS: 24000, MaxIOPS: 128000},
		{CPUMillis: 32000, MinIOPS: 32000, MaxIOPS: 192000},
	}
}

// Values returns every value in the range, in increments of IOPSIncrement.
func (r IOPSRange) Values() []int {
	values := make([]int, 0, (r.MaxIOPS-r.MinIOPS)/IOPSIncrement+1)
	for v := r.MinIOPS; v <= r.MaxIOPS; v += IOPSIncrement {
		values = append(values, v)
	}
	return values
}

// String renders the range as "1 CPU: 6000-12000", or "0.25 CPU: 3000" when
// it holds a single value.
func (r IOPSRange) String() string {
	if r.MinIOPS == r.MaxIOPS {
		return fmt.Sprintf("%s CPU: %d", FormatCPUCores(r.CPUMillis), r.MinIOPS)
	}
	return fmt.Sprintf("%s CPU: %d-%d", FormatCPUCores(r.CPUMillis), r.MinIOPS, r.MaxIOPS)
}

// FluidIOPSRangesString renders every fluid IOPS range, comma-separated.
func FluidIOPSRangesString() string {
	ranges := FluidIOPSRanges()
	strs := make([]string, len(ranges))
	for i, r := range ranges {
		strs[i] = r.String()
	}
	return strings.Join(strs, ", ")
}
