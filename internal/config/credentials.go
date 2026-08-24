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

func (c *Config) credentialsFileName() string {
	return fmt.Sprintf("%s/credentials", c.ConfigDir)
}

// StoreCredentials stores a PAT credential.
func (c *Config) StoreCredentials(apiKey, projectID string) error {
	return c.storeCredentials(storedCredentials{
		APIKey:    apiKey,
		ProjectID: projectID,
	})
}

// StoreOAuthCredentials stores an OAuth token (access + refresh + expiry) and
// project ID. Use this for the PKCE login path; use StoreCredentials for PAT.
func (c *Config) StoreOAuthCredentials(token *oauth2.Token, projectID string) error {
	if token == nil {
		return fmt.Errorf("oauth token must not be nil")
	}
	return c.storeCredentials(storedCredentials{
		OAuth:     token,
		ProjectID: projectID,
	})
}

// SwitchProject repoints the stored login at projectID, keeping its OAuth
// session — read and write sit together so a token refreshed in between isn't
// lost. An API key is scoped to one project, so this refuses one.
func (c *Config) SwitchProject(projectID string) error {
	creds, err := c.GetStoredCredentials()
	if err != nil {
		return err
	}
	if creds.OAuth == nil {
		return fmt.Errorf("stored credentials are an API key, which is scoped to a single project")
	}
	return c.StoreOAuthCredentials(creds.OAuth, projectID)
}

// StoreCredentialsToFile stores credentials to file (test helper)
func (c *Config) StoreCredentialsToFile(apiKey, projectID string) error {
	creds := storedCredentials{
		APIKey:    apiKey,
		ProjectID: projectID,
	}

	credentialsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	return c.storeToFile(string(credentialsJSON))
}

func (c *Config) storeCredentials(creds storedCredentials) error {
	credentialsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	keyringErr := storeToKeyring(string(credentialsJSON))
	if keyringErr == nil {
		return nil
	}
	if err := c.storeToFile(string(credentialsJSON)); err != nil {
		return err
	}
	// A readable-but-unwritable keyring entry would shadow the file just
	// written; deleted only now so a failure never destroys the sole copy.
	if err := removeCredentialsFromKeyring(); err != nil {
		// Don't leave the new credentials latent behind the surviving entry:
		// they'd silently take over whenever the keyring stops being readable.
		return errors.Join(
			fmt.Errorf("failed to store credentials in keyring: %w", keyringErr),
			err, c.removeCredentialsFile())
	}
	return nil
}

func storeToKeyring(credentials string) error {
	return keyring.Set(GetServiceName(), keyringUsername, credentials)
}

// storeToFile stores credentials to ~/.config/tiger/credentials with restricted permissions
func (c *Config) storeToFile(credentials string) error {
	credentialsFile := c.credentialsFileName()
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

func (c *Config) GetStoredCredentials() (*Credentials, error) {
	raw, err := c.loadCredentialsBlob()
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
func (c *Config) loadCredentialsBlob() (string, error) {
	if blob, err := keyring.Get(GetServiceName(), keyringUsername); err == nil {
		if blob == "" {
			return "", ErrNotLoggedIn
		}
		return blob, nil
	}

	credentialsFile := c.credentialsFileName()
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

// RemoveCredentials removes stored credentials from keyring and file fallback.
// The file is removed even when the keyring delete fails.
func (c *Config) RemoveCredentials() error {
	return errors.Join(removeCredentialsFromKeyring(), c.removeCredentialsFile())
}

// removeCredentialsFromKeyring deletes the keyring entry, erroring only when
// an entry provably survives (still readable — it would keep authenticating).
// An unreadable backend can't serve the entry either, and treating it as
// fatal would break logout on every keyring-less system.
func removeCredentialsFromKeyring() error {
	err := keyring.Delete(GetServiceName(), keyringUsername)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if _, getErr := keyring.Get(GetServiceName(), keyringUsername); getErr == nil {
		return fmt.Errorf("failed to remove credentials from keyring: %w", err)
	}
	return nil
}

// removeCredentialsFile removes credentials file
func (c *Config) removeCredentialsFile() error {
	credentialsFile := c.credentialsFileName()
	if err := os.Remove(credentialsFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove credentials file: %w", err)
	}
	return nil
}
