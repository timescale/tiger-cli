package config

import (
	"fmt"
	"slices"
	"strings"
)

// validOutputFormats are the formats every command supports. Commands that
// accept an extra format (`env`, `bare`) pass it to ValidateOutputFormat.
var validOutputFormats = []string{"json", "yaml", "table"}

// ValidateOutputFormat checks format against the universally supported formats
// plus any command-specific extras.
func ValidateOutputFormat(format string, extra ...string) error {
	formats := append(slices.Clone(validOutputFormats), extra...)
	if slices.Contains(formats, format) {
		return nil
	}
	return fmt.Errorf("invalid output format: %s (must be one of: %s)", format, strings.Join(formats, ", "))
}
