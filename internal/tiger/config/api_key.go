package config

import (
	"encoding/json"
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
	keyringUsername    = "credentials"
)

// storedCredentials represents the JSON structure for stored credentials
type storedCredentials struct {
	APIKey    string `json:"api_key"`
	ProjectID string `json:"project_id"`
}

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

// StoreCredentials stores the API key (public:secret) and project ID together
// The credentials are stored as JSON with api_key and project_id fields
func StoreCredentials(apiKey, projectID string) error {
	creds := storedCredentials{
		APIKey:    apiKey,
		ProjectID: projectID,
	}

	credentialsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Try keyring first
	if err := storeToKeyring(string(credentialsJSON)); err == nil {
		return nil
	}

	// Fallback to file storage
	return storeToFile(string(credentialsJSON))
}

// StoreCredentialsToFile stores credentials to file (test helper)
func StoreCredentialsToFile(apiKey, projectID string) error {
	creds := storedCredentials{
		APIKey:    apiKey,
		ProjectID: projectID,
	}

	credentialsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	return storeToFile(string(credentialsJSON))
}

func storeToKeyring(credentials string) error {
	return keyring.Set(GetServiceName(), keyringUsername, credentials)
}

// storeToFile stores credentials to ~/.config/tiger/credentials with restricted permissions
func storeToFile(credentials string) error {
	configDir := GetConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	credentialsFile := fmt.Sprintf("%s/credentials", configDir)
	file, err := os.OpenFile(credentialsFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create credentials file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(credentials); err != nil {
		return fmt.Errorf("failed to write credentials to file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	return nil
}

var ErrNotLoggedIn = errors.New("not logged in")

// GetCredentials retrieves the API key and project ID from storage
// Returns (apiKey, projectID, error) where apiKey is in "publicKey:secretKey" format
func GetCredentials() (string, string, error) {
	// Try keyring first
	if apiKey, projectId, err := getCredentialsFromKeyring(); err == nil {
		return apiKey, projectId, nil
	}

	// Fallback to file storage
	return getCredentialsFromFile()
}

// getCredentialsFromKeyring gets credentials from keyring.
func getCredentialsFromKeyring() (string, string, error) {
	combined, err := keyring.Get(GetServiceName(), keyringUsername)
	if err != nil {
		return "", "", err
	}
	return parseCredentials(combined)
}

// getCredentialsFromFile retrieves credentials from file
func getCredentialsFromFile() (string, string, error) {
	configDir := GetConfigDir()
	credentialsFile := fmt.Sprintf("%s/credentials", configDir)

	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", ErrNotLoggedIn
		}
		return "", "", fmt.Errorf("failed to read credentials file: %w", err)
	}

	credentials := strings.TrimSpace(string(data))
	if credentials == "" {
		return "", "", ErrNotLoggedIn
	}

	return parseCredentials(credentials)
}

// parseCredentials parses the stored credentials from JSON format
// Returns (apiKey, projectID, error) where apiKey is in "publicKey:secretKey" format
func parseCredentials(combined string) (string, string, error) {
	var creds storedCredentials
	if err := json.Unmarshal([]byte(combined), &creds); err != nil {
		return "", "", fmt.Errorf("failed to parse credentials: %w", err)
	}

	if creds.APIKey == "" {
		return "", "", fmt.Errorf("API key not found in stored credentials")
	}
	if creds.ProjectID == "" {
		return "", "", fmt.Errorf("project ID not found in stored credentials")
	}

	return creds.APIKey, creds.ProjectID, nil
}

// RemoveCredentials removes stored credentials from keyring and file fallback
func RemoveCredentials() error {
	// Remove from keyring (ignore errors as it might not exist)
	removeCredentialsFromKeyring()
	return removeCredentialsFile()
}

// removeCredentialsFromKeyring removes credentials from keyring (test helper)
func removeCredentialsFromKeyring() {
	keyring.Delete(GetServiceName(), keyringUsername)
}

// removeCredentialsFile removes credentials file
func removeCredentialsFile() error {
	configDir := GetConfigDir()
	credentialsFile := fmt.Sprintf("%s/credentials", configDir)
	if err := os.Remove(credentialsFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove credentials file: %w", err)
	}
	return nil
}
