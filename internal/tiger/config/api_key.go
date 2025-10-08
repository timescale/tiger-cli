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
	keyringServiceName = "tiger-cli"
	keyringUsername    = "api-key"
)

// testServiceNameOverride allows tests to override the service name for isolation
var testServiceNameOverride string

// GetServiceName returns the appropriate service name for keyring operations
func GetServiceName() string {
	// Tests should set a unique service name to avoid conflicts
	if testServiceNameOverride != "" {
		return testServiceNameOverride
	}

	// In test mode without an override, panic to catch missing test setup
	if testing.Testing() {
		panic("test must call SetTestServiceName() to set a unique keyring service name")
	}

	return keyringServiceName
}

// SetTestServiceName sets a unique service name for testing based on the test name
// This allows tests to use unique service names to avoid conflicts when running in parallel
// The cleanup is automatically registered with t.Cleanup()
func SetTestServiceName(t *testing.T) {
	testServiceNameOverride = "tiger-test-" + t.Name()

	// Automatically clean up when the test finishes
	t.Cleanup(func() {
		testServiceNameOverride = ""
	})
}

// storeAPIKey stores the API key using keyring with file fallback
func StoreAPIKey(apiKey string) error {
	// Try keyring first
	if err := StoreAPIKeyToKeyring(apiKey); err == nil {
		return nil
	}

	// Fallback to file storage
	return StoreAPIKeyToFile(apiKey)
}

func StoreAPIKeyToKeyring(apiKey string) error {
	return keyring.Set(GetServiceName(), keyringUsername, apiKey)
}

// StoreAPIKeyToFile stores API key to ~/.config/tiger/api-key with restricted permissions
func StoreAPIKeyToFile(apiKey string) error {
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

var ErrNotLoggedIn = errors.New("not logged in")

// GetAPIKey retrieves the API key from keyring or file fallback
func GetAPIKey() (string, error) {
	// Try keyring first
	apiKey, err := GetAPIKeyFromKeyring()
	if err == nil && apiKey != "" {
		return apiKey, nil
	}

	// Fallback to file storage
	return GetAPIKeyFromFile()
}

func GetAPIKeyFromKeyring() (string, error) {
	return keyring.Get(GetServiceName(), keyringUsername)
}

// GetAPIKeyFromFile retrieves API key from ~/.config/tiger/api-key
func GetAPIKeyFromFile() (string, error) {
	configDir := GetConfigDir()
	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)

	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		// If the file does not exist, treat as not logged in
		if os.IsNotExist(err) {
			return "", ErrNotLoggedIn
		}
		return "", fmt.Errorf("failed to read API key file: %w", err)
	}

	apiKey := strings.TrimSpace(string(data))

	// If file exists but is empty, treat as not logged in
	if apiKey == "" {
		return "", ErrNotLoggedIn
	}

	return apiKey, nil
}

// RemoveAPIKey removes the API key from keyring and file fallback
func RemoveAPIKey() error {
	RemoveAPIKeyFromKeyring()

	// Remove from file fallback
	return RemoveAPIKeyFromFile()
}

func RemoveAPIKeyFromKeyring() error {
	// Try to remove from keyring (ignore errors as it might not exist)
	return keyring.Delete(GetServiceName(), keyringUsername)
}

// RemoveAPIKeyFromFile removes the API key file
func RemoveAPIKeyFromFile() error {
	configDir := GetConfigDir()
	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)

	err := os.Remove(apiKeyFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove API key file: %w", err)
	}

	return nil
}
