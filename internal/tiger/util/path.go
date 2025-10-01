package util

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands environment variables and tilde in file paths.
// It handles:
// - Empty paths (returns empty string)
// - Environment variable expansion (e.g., $HOME/config)
// - Home directory expansion (e.g., ~/config or ~)
// - Path normalization for cross-platform compatibility
func ExpandPath(path string) string {
	if path == "" {
		return ""
	}

	// Expand environment variables
	expanded := os.ExpandEnv(path)

	// Expand home directory
	if expanded == "~" {
		homeDir, _ := os.UserHomeDir()
		return homeDir
	}

	if strings.HasPrefix(expanded, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(homeDir, expanded[2:])
		}
	}

	return filepath.Clean(expanded)
}
