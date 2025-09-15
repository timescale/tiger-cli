package mcp

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

const (
	serviceName = "tiger-cli"
	username    = "user"
)

// getAPIKey retrieves the API key from keyring or file fallback
// This is a copy of the function from cmd/auth.go to avoid circular imports
func (s *Server) getAPIKey() (string, error) {
	// Try keyring first
	apiKey, err := keyring.Get(serviceName, username)
	if err == nil && apiKey != "" {
		return apiKey, nil
	}

	// Fall back to file-based storage
	return s.getAPIKeyFromFile()
}

// getAPIKeyFromFile retrieves API key from ~/.config/tiger/api-key
func (s *Server) getAPIKeyFromFile() (string, error) {
	configDir := config.GetConfigDir()
	apiKeyFile := filepath.Join(configDir, "api-key")

	apiKeyBytes, err := os.ReadFile(apiKeyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no API key found. Please run 'tiger auth login'")
		}
		return "", fmt.Errorf("failed to read API key file: %w", err)
	}

	apiKey := string(apiKeyBytes)
	if apiKey == "" {
		return "", fmt.Errorf("API key file is empty. Please run 'tiger auth login'")
	}

	return apiKey, nil
}
