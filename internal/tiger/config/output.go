package config

import (
	"fmt"
	"strings"
)

var validOutputFormats = []string{"json", "yaml", "table"}
var validOutputFormatsWithEnv = append(append([]string(nil), validOutputFormats...), "env")

func ValidateOutputFormat(format string, allowEnv bool) error {
	formats := validOutputFormats
	if allowEnv {
		formats = validOutputFormatsWithEnv
	}
	for _, valid := range formats {
		if format == valid {
			return nil
		}
	}
	return fmt.Errorf("invalid output format: %s (must be one of: %s)", format, strings.Join(formats, ", "))
}
