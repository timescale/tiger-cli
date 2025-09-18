package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// Keyring parameters
const (
	serviceName     = "tiger-cli"
	testServiceName = "tiger-cli-test"
	username        = "api-key"
)

// getServiceName returns the appropriate service name for keyring operations
// Uses a test-specific service name when running in test mode to avoid polluting the real keyring
func getServiceName() string {
	// Use Go's built-in testing detection
	if testing.Testing() {
		return testServiceName
	}

	return serviceName
}

// GetServiceName returns the appropriate service name for keyring operations
// Public function for external packages
func GetServiceName() string {
	return getServiceName()
}

// storeAPIKey stores the API key using keyring with file fallback
func StoreAPIKey(apiKey string) error {
	// Try keyring first
	err := keyring.Set(getServiceName(), username, apiKey)
	if err == nil {
		return nil
	}

	// Fallback to file storage
	return storeAPIKeyToFile(apiKey)
}

// GetAPIKey retrieves the API key from keyring or file fallback
func GetAPIKey() (string, error) {
	// Try keyring first
	apiKey, err := keyring.Get(getServiceName(), username)
	if err == nil && apiKey != "" {
		return apiKey, nil
	}

	// Fallback to file storage
	return getAPIKeyFromFile()
}

// RemoveAPIKey removes the API key from keyring and file fallback
func RemoveAPIKey() error {
	// Try to remove from keyring (ignore errors as it might not exist)
	keyring.Delete(getServiceName(), username)

	// Remove from file fallback
	return removeAPIKeyFromFile()
}

// storeAPIKeyToFile stores API key to ~/.config/tiger/api-key with restricted permissions
func storeAPIKeyToFile(apiKey string) error {
	configDir := GetConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)
	file, err := os.OpenFile(apiKeyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create API key file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(apiKey); err != nil {
		return fmt.Errorf("failed to write API key to file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	return nil
}

var errNotLoggedIn = errors.New("not logged in")

// getAPIKeyFromFile retrieves API key from ~/.config/tiger/api-key
func getAPIKeyFromFile() (string, error) {
	configDir := GetConfigDir()
	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)

	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		// If the file does not exist, treat as not logged in
		if os.IsNotExist(err) {
			return "", errNotLoggedIn
		}
		return "", fmt.Errorf("failed to read API key file: %w", err)
	}

	apiKey := strings.TrimSpace(string(data))

	// If file exists but is empty, treat as not logged in
	if apiKey == "" {
		return "", errNotLoggedIn
	}

	return apiKey, nil
}

// removeAPIKeyFromFile removes the API key file
func removeAPIKeyFromFile() error {
	configDir := GetConfigDir()
	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)

	err := os.Remove(apiKeyFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove API key file: %w", err)
	}

	return nil
}

// The following functions are exported for testing purposes

// StoreAPIKeyToFile stores API key to file (for testing)
func StoreAPIKeyToFile(apiKey string) error {
	return storeAPIKeyToFile(apiKey)
}

// GetAPIKeyFromFile retrieves API key from file (for testing)
func GetAPIKeyFromFile() (string, error) {
	return getAPIKeyFromFile()
}

// RemoveAPIKeyFromFile removes the API key file (for testing)
func RemoveAPIKeyFromFile() error {
	return removeAPIKeyFromFile()
}

// ErrNotLoggedIn is the error returned when not logged in
var ErrNotLoggedIn = errNotLoggedIn
