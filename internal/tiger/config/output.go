package config

import (
	"fmt"
	"strings"
)

var ValidOutputFormats = []string{"json", "yaml", "env", "table"}

func ValidateOutputFormat(format string) error {
	for _, valid := range ValidOutputFormats {
		if format == valid {
			return nil
		}
	}
	return fmt.Errorf("invalid output format: %s (must be one of: %s)", format, strings.Join(ValidOutputFormats, ", "))
}
