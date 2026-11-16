package util

import (
	"fmt"
	"strings"
)

// storageUnits are the units FormatStorageMB scales through, each 1024 times
// the last.
var storageUnits = []string{"MB", "GB", "TB", "PB"}

// FormatStorageMB formats a size in mebibytes, as the Tiger Cloud API reports
// it, in the largest unit that keeps the value at least 1, to one decimal
// place ("512 MB", "1.2 GB", "843.5 GB").
func FormatStorageMB(mb int) string {
	value := float64(mb)
	unit := 0
	for value >= 1024 && unit < len(storageUnits)-1 {
		value /= 1024
		unit++
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", value), ".0") + " " + storageUnits[unit]
}
