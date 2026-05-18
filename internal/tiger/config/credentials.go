package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// Keyring parameters
const (
	keyringServiceName = "tiger-cli"
	keyringUsername    = "credentials"
)

// storedCredentials represents the JSON structure for stored credentials.
// Exactly one of APIKey (PAT) or OAuth (PKCE) is populated per login.
type storedCredentials struct {
	APIKey    string        `json:"api_key,omitempty"`
	OAuth     *oauth2.Token `json:"oauth,omitempty"`
	ProjectID string        `json:"project_id"`
}

// Credentials is the resolved form of what's in keyring/file. Exactly one of
// APIKey (PAT) or OAuth (PKCE) is populated.
type Credentials struct {
	APIKey    string
	OAuth     *oauth2.Token
	ProjectID string
}

// testServiceNameOverride allows tests to override the service name for isolation
var testServiceNameOverride string

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

func getCredentialsFileName() string {
	configDir := GetConfigDir()
	return fmt.Sprintf("%s/credentials", configDir)
}

// StoreCredentials stores a PAT credential.
func StoreCredentials(apiKey, projectID string) error {
	return storeCredentials(storedCredentials{
		APIKey:    apiKey,
		ProjectID: projectID,
	})
}

// StoreOAuthCredentials stores an OAuth token (access + refresh + expiry) and
// project ID. Use this for the PKCE login path; use StoreCredentials for PAT.
func StoreOAuthCredentials(token *oauth2.Token, projectID string) error {
	if token == nil {
		return fmt.Errorf("oauth token must not be nil")
	}
	return storeCredentials(storedCredentials{
		OAuth:     token,
		ProjectID: projectID,
	})
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

func storeCredentials(creds storedCredentials) error {
	credentialsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := storeToKeyring(string(credentialsJSON)); err == nil {
		return nil
	}
	return storeToFile(string(credentialsJSON))
}

func storeToKeyring(credentials string) error {
	return keyring.Set(GetServiceName(), keyringUsername, credentials)
}

// storeToFile stores credentials to ~/.config/tiger/credentials with restricted permissions
func storeToFile(credentials string) error {
	credentialsFile := getCredentialsFileName()
	if err := os.MkdirAll(filepath.Dir(credentialsFile), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

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

// GetCredentials retrieves a PAT API key and project ID from storage. Returns
// an error if the stored credentials are OAuth rather than PAT — OAuth-aware
// callers should use GetStoredCredentials.
func GetCredentials() (string, string, error) {
	creds, err := GetStoredCredentials()
	if err != nil {
		return "", "", err
	}
	if creds.APIKey == "" {
		return "", "", fmt.Errorf("stored credentials are not a PAT")
	}
	return creds.APIKey, creds.ProjectID, nil
}

func GetStoredCredentials() (*Credentials, error) {
	raw, err := loadCredentialsBlob()
	if err != nil {
		return nil, err
	}

	var stored storedCredentials
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}
	if stored.ProjectID == "" {
		return nil, fmt.Errorf("project ID not found in stored credentials")
	}

	switch {
	case stored.OAuth != nil && stored.OAuth.AccessToken != "":
		return &Credentials{OAuth: stored.OAuth, ProjectID: stored.ProjectID}, nil
	case stored.APIKey != "":
		return &Credentials{APIKey: stored.APIKey, ProjectID: stored.ProjectID}, nil
	default:
		return nil, fmt.Errorf("stored credentials have neither API key nor OAuth token")
	}
}

// loadCredentialsBlob returns the raw JSON blob from keyring or file fallback.
func loadCredentialsBlob() (string, error) {
	if blob, err := keyring.Get(GetServiceName(), keyringUsername); err == nil {
		if blob == "" {
			return "", ErrNotLoggedIn
		}
		return blob, nil
	}

	credentialsFile := getCredentialsFileName()
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotLoggedIn
		}
		return "", fmt.Errorf("failed to read credentials file: %w", err)
	}
	if len(data) == 0 {
		return "", ErrNotLoggedIn
	}
	return string(data), nil
}

func RemoveCredentials() error {
	// Remove from keyring (ignore errors as it might not exist)
	removeCredentialsFromKeyring()
	return removeCredentialsFile()
}

func removeCredentialsFromKeyring() {
	keyring.Delete(GetServiceName(), keyringUsername)
}

func removeCredentialsFile() error {
	credentialsFile := getCredentialsFileName()
	if err := os.Remove(credentialsFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove credentials file: %w", err)
	}
	return nil
}
