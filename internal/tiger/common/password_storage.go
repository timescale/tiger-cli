package common

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/zalando/go-keyring"
)

// getPasswordServiceName returns the service name for password storage
// Uses the same service name as auth for consistency
func getPasswordServiceName() string {
	return config.GetServiceName()
}

// buildPasswordKeyringUsername creates a unique keyring username for service passwords
func buildPasswordKeyringUsername(service api.Service, role string) (string, error) {
	if service.ServiceId == nil {
		return "", fmt.Errorf("service ID is required")
	}
	if service.ProjectId == nil {
		return "", fmt.Errorf("project ID is required")
	}
	if role == "" {
		return "", fmt.Errorf("role is required")
	}

	return fmt.Sprintf("password-%s-%s-%s", *service.ProjectId, *service.ServiceId, role), nil
}

// sanitizeErrorMessage removes sensitive information from error messages
func sanitizeErrorMessage(err error, password string) string {
	if err == nil {
		return ""
	}
	errorMsg := err.Error()
	if password != "" && strings.Contains(errorMsg, password) {
		// Replace the actual password with asterisks
		errorMsg = strings.ReplaceAll(errorMsg, password, "***")
	}
	return errorMsg
}

// PasswordStorageResult contains the result of password storage operations
type PasswordStorageResult struct {
	Success bool   `json:"success"`
	Method  string `json:"method"`  // "keyring", "pgpass", or "none"
	Message string `json:"message"` // Human-readable message
}

// PasswordStorage defines the interface for password storage implementations
type PasswordStorage interface {
	Save(service api.Service, password string, role string) error
	Get(service api.Service, role string) (string, error)
	Remove(service api.Service, role string) error
	GetStorageResult(err error, password string) PasswordStorageResult
}

// KeyringStorage implements password storage using system keyring
type KeyringStorage struct{}

func (k *KeyringStorage) Save(service api.Service, password string, role string) error {
	username, err := buildPasswordKeyringUsername(service, role)
	if err != nil {
		return err
	}

	return keyring.Set(getPasswordServiceName(), username, password)
}

func (k *KeyringStorage) Get(service api.Service, role string) (string, error) {
	username, err := buildPasswordKeyringUsername(service, role)
	if err != nil {
		return "", err
	}

	return keyring.Get(getPasswordServiceName(), username)
}

func (k *KeyringStorage) Remove(service api.Service, role string) error {
	username, err := buildPasswordKeyringUsername(service, role)
	if err != nil {
		return err
	}

	return keyring.Delete(getPasswordServiceName(), username)
}

func (k *KeyringStorage) GetStorageResult(err error, password string) PasswordStorageResult {
	if err != nil {
		sanitizedErr := sanitizeErrorMessage(err, password)
		return PasswordStorageResult{
			Success: false,
			Method:  "keyring",
			Message: fmt.Sprintf("Failed to save password to keyring: %s", sanitizedErr),
		}
	}
	return PasswordStorageResult{
		Success: true,
		Method:  "keyring",
		Message: "Password saved to system keyring for automatic authentication",
	}
}

// PgpassStorage implements password storage using ~/.pgpass file
type PgpassStorage struct{}

func (p *PgpassStorage) Save(service api.Service, password string, role string) error {
	if service.Endpoint == nil || service.Endpoint.Host == nil {
		return fmt.Errorf("service endpoint not available")
	}
	if role == "" {
		return fmt.Errorf("role is required")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	pgpassPath := filepath.Join(homeDir, ".pgpass")
	host := *service.Endpoint.Host
	port := "5432" // default PostgreSQL port
	if service.Endpoint.Port != nil {
		port = fmt.Sprintf("%d", *service.Endpoint.Port)
	}
	database := "tsdb" // TimescaleDB database name
	username := role   // Use the provided role as username

	// Remove existing entry first (if it exists)
	if err := p.removeEntry(pgpassPath, host, port, username); err != nil {
		return fmt.Errorf("failed to remove existing .pgpass entry: %w", err)
	}

	// Create new entry: hostname:port:database:username:password
	entry := fmt.Sprintf("%s:%s:%s:%s:%s\n", host, port, database, username, password)

	// Append to .pgpass file with restricted permissions
	file, err := os.OpenFile(pgpassPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open .pgpass file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to .pgpass file: %w", err)
	}

	return nil
}

func (p *PgpassStorage) Get(service api.Service, role string) (string, error) {
	if service.Endpoint == nil || service.Endpoint.Host == nil {
		return "", fmt.Errorf("service endpoint not available")
	}
	if role == "" {
		return "", fmt.Errorf("role is required")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	pgpassPath := filepath.Join(homeDir, ".pgpass")

	// Check if .pgpass file exists
	if _, err := os.Stat(pgpassPath); os.IsNotExist(err) {
		return "", fmt.Errorf("no .pgpass file found")
	}

	host := *service.Endpoint.Host
	port := "5432"
	if service.Endpoint.Port != nil {
		port = fmt.Sprintf("%d", *service.Endpoint.Port)
	}
	username := role

	// Read and parse .pgpass file
	content, err := os.ReadFile(pgpassPath)
	if err != nil {
		return "", fmt.Errorf("failed to read .pgpass file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.Split(line, ":")
		if len(parts) != 5 {
			continue // Invalid format
		}

		// Match host:port:database:username:password
		if parts[0] == host && parts[1] == port && parts[3] == username {
			return parts[4], nil // Return the password
		}
	}

	return "", fmt.Errorf("no matching entry found in .pgpass file")
}

func (p *PgpassStorage) Remove(service api.Service, role string) error {
	if service.Endpoint == nil || service.Endpoint.Host == nil {
		return fmt.Errorf("service endpoint not available")
	}
	if role == "" {
		return fmt.Errorf("role is required")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	pgpassPath := filepath.Join(homeDir, ".pgpass")
	host := *service.Endpoint.Host
	port := "5432"
	if service.Endpoint.Port != nil {
		port = fmt.Sprintf("%d", *service.Endpoint.Port)
	}
	username := role

	return p.removeEntry(pgpassPath, host, port, username)
}

// removeEntry removes an existing entry from the .pgpass file
func (p *PgpassStorage) removeEntry(pgpassPath, host, port, username string) error {
	// Read all lines from the file
	file, err := os.Open(pgpassPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, nothing to remove
		}
		return fmt.Errorf("failed to open .pgpass file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	targetPrefix := fmt.Sprintf("%s:%s:", host, port)

	for scanner.Scan() {
		line := scanner.Text()
		// Keep lines that don't match our target entry
		if !(strings.HasPrefix(line, targetPrefix) && strings.Contains(line, username)) {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading .pgpass file: %w", err)
	}

	// Write back all lines except the one we want to remove
	tmpFile, err := os.CreateTemp(filepath.Dir(pgpassPath), ".pgpass.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	for _, line := range lines {
		if _, err := tmpFile.WriteString(line + "\n"); err != nil {
			tmpFile.Close()
			return fmt.Errorf("failed to write to temporary file: %w", err)
		}
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Set proper permissions and replace the original file
	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		return fmt.Errorf("failed to set permissions on temporary file: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), pgpassPath); err != nil {
		return fmt.Errorf("failed to replace .pgpass file: %w", err)
	}

	return nil
}

func (p *PgpassStorage) GetStorageResult(err error, password string) PasswordStorageResult {
	if err != nil {
		sanitizedErr := sanitizeErrorMessage(err, password)
		return PasswordStorageResult{
			Success: false,
			Method:  "pgpass",
			Message: fmt.Sprintf("Failed to save password to ~/.pgpass: %s", sanitizedErr),
		}
	}
	return PasswordStorageResult{
		Success: true,
		Method:  "pgpass",
		Message: "Password saved to ~/.pgpass for automatic authentication",
	}
}

// NoStorage implements no password storage (passwords are not saved)
type NoStorage struct{}

func (n *NoStorage) Save(service api.Service, password string, role string) error {
	return nil // Do nothing
}

func (n *NoStorage) Get(service api.Service, role string) (string, error) {
	return "", fmt.Errorf("password storage disabled")
}

func (n *NoStorage) Remove(service api.Service, role string) error {
	return nil // Do nothing
}

func (n *NoStorage) GetStorageResult(err error, password string) PasswordStorageResult {
	return PasswordStorageResult{
		Success: false, // Not really an error, but password wasn't saved
		Method:  "none",
		Message: "Password not saved (--password-storage=none). Make sure to store it securely.",
	}
}

// AutoFallbackStorage tries keyring first, falls back to pgpass on any error
type AutoFallbackStorage struct {
	keyring    *KeyringStorage
	pgpass     *PgpassStorage
	lastMethod string // tracks which method was used for GetStorageResult
}

func (a *AutoFallbackStorage) Save(service api.Service, password string, role string) error {
	// Try keyring first
	if err := a.keyring.Save(service, password, role); err == nil {
		a.lastMethod = "keyring"
		return nil
	}
	// Any keyring error -> fall back to pgpass
	if err := a.pgpass.Save(service, password, role); err != nil {
		a.lastMethod = "auto"
		return err
	}
	a.lastMethod = "pgpass"
	return nil
}

func (a *AutoFallbackStorage) Get(service api.Service, role string) (string, error) {
	// Try keyring first
	if password, err := a.keyring.Get(service, role); err == nil {
		return password, nil
	}
	// Any keyring error -> try pgpass
	return a.pgpass.Get(service, role)
}

func (a *AutoFallbackStorage) Remove(service api.Service, role string) error {
	// Try to remove from both (best effort)
	keyringErr := a.keyring.Remove(service, role)
	pgpassErr := a.pgpass.Remove(service, role)

	// Success if removed from at least one location
	if keyringErr == nil || pgpassErr == nil {
		return nil
	}
	// If both failed, return a combined error
	return fmt.Errorf("keyring: %v, pgpass: %v", keyringErr, pgpassErr)
}

func (a *AutoFallbackStorage) GetStorageResult(err error, password string) PasswordStorageResult {
	if err != nil {
		return PasswordStorageResult{
			Success: false,
			Method:  a.lastMethod,
			Message: fmt.Sprintf("Failed to save password: %s", sanitizeErrorMessage(err, password)),
		}
	}

	// Return method-specific success message
	switch a.lastMethod {
	case "keyring":
		return PasswordStorageResult{
			Success: true,
			Method:  "keyring",
			Message: "Password saved to system keyring for automatic authentication",
		}
	case "pgpass":
		return PasswordStorageResult{
			Success: true,
			Method:  "pgpass",
			Message: "Password saved to ~/.pgpass for automatic authentication",
		}
	default:
		return PasswordStorageResult{
			Success: true,
			Method:  "auto",
			Message: "Password saved for automatic authentication",
		}
	}
}

// GetPasswordStorage returns the appropriate PasswordStorage implementation based on configuration
func GetPasswordStorage() PasswordStorage {
	storageMethod := viper.GetString("password_storage")
	switch storageMethod {
	case "keyring":
		return &KeyringStorage{}
	case "pgpass":
		return &PgpassStorage{}
	case "none":
		return &NoStorage{}
	case "auto":
		return &AutoFallbackStorage{
			keyring: &KeyringStorage{},
			pgpass:  &PgpassStorage{},
		}
	default:
		// Default to auto for best compatibility across environments
		return &AutoFallbackStorage{
			keyring: &KeyringStorage{},
			pgpass:  &PgpassStorage{},
		}
	}
}

// SavePasswordWithResult handles saving a password and returns both error and result info
func SavePasswordWithResult(service api.Service, password string, role string) (PasswordStorageResult, error) {
	if password == "" {
		return PasswordStorageResult{
			Success: false,
			Method:  "none",
			Message: "No password provided",
		}, nil
	}
	if role == "" {
		return PasswordStorageResult{
			Success: false,
			Method:  "none",
			Message: "Role is required",
		}, fmt.Errorf("role is required")
	}

	storage := GetPasswordStorage()
	err := storage.Save(service, password, role)
	result := storage.GetStorageResult(err, password)

	return result, err
}
