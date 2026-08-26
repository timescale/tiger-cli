package config

import (
	"fmt"
	"slices"
	"strings"
)

// validOutputFormats are the formats every command supports. Commands that
// accept an extra format (`env`, `bare`) pass it to ValidateOutputFormat.
var validOutputFormats = []string{"json", "yaml", "table"}

// ValidOutputFormats returns the universally supported output formats plus
// any command-specific extras (`env`, `bare`).
func ValidOutputFormats(extra ...string) []string {
	return append(slices.Clone(validOutputFormats), extra...)
}

// ValidateOutputFormat checks format against the universally supported formats
// plus any command-specific extras.
func ValidateOutputFormat(format string, extra ...string) error {
	formats := ValidOutputFormats(extra...)
	if slices.Contains(formats, format) {
		return nil
	}
	return fmt.Errorf("invalid output format: %s (must be one of: %s)", format, strings.Join(formats, ", "))
}
