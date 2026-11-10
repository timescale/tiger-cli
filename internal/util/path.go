package util

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands environment variables and a leading tilde in a path, and
// cleans the result. An empty path stays empty.
func ExpandPath(path string) string {
	return ExpandHome(os.ExpandEnv(path))
}

// ExpandHome expands a leading tilde (~ or ~/...) to the home directory and
// cleans the result, leaving environment variables untouched. An empty path
// stays empty, and the tilde is left in place if the home directory is unknown.
func ExpandHome(path string) string {
	if path == "" {
		return ""
	}

	if path == "~" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return homeDir
		}
	}

	if strings.HasPrefix(path, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(homeDir, path[2:])
		}
	}

	return filepath.Clean(path)
}
