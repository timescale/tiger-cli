package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/zalando/go-keyring"
)

// getPasswordServiceName returns the service name for password storage
// Uses the same service name as auth for consistency
func getPasswordServiceName() string {
	return getServiceName()
}

// buildPasswordKeyringUsername creates a unique keyring username for service passwords
func buildPasswordKeyringUsername(service api.Service) (string, error) {
	if service.ServiceId == nil {
		return "", fmt.Errorf("service ID is required")
	}
	if service.ProjectId == nil {
		return "", fmt.Errorf("project ID is required")
	}

	return fmt.Sprintf("password-%s-%s", *service.ProjectId, *service.ServiceId), nil
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

// PasswordStorage defines the interface for password storage implementations
type PasswordStorage interface {
	Save(service api.Service, password string) error
	Get(service api.Service) (string, error)
	Remove(service api.Service) error
	HandleSaveMessage(err error, password string, output io.Writer)
}

// KeyringStorage implements password storage using system keyring
type KeyringStorage struct{}

func (k *KeyringStorage) Save(service api.Service, password string) error {
	username, err := buildPasswordKeyringUsername(service)
	if err != nil {
		return err
	}

	return keyring.Set(getPasswordServiceName(), username, password)
}

func (k *KeyringStorage) Get(service api.Service) (string, error) {
	username, err := buildPasswordKeyringUsername(service)
	if err != nil {
		return "", err
	}

	return keyring.Get(getPasswordServiceName(), username)
}

func (k *KeyringStorage) Remove(service api.Service) error {
	username, err := buildPasswordKeyringUsername(service)
	if err != nil {
		return err
	}

	return keyring.Delete(getPasswordServiceName(), username)
}

func (k *KeyringStorage) HandleSaveMessage(err error, password string, output io.Writer) {
	if err != nil {
		sanitizedErr := sanitizeErrorMessage(err, password)
		fmt.Fprintf(output, "⚠️  Failed to save password to keyring: %s\n", sanitizedErr)
	} else {
		fmt.Fprintf(output, "🔐 Password saved to system keyring for automatic authentication\n")
	}
}

// PgpassStorage implements password storage using ~/.pgpass file
type PgpassStorage struct{}

func (p *PgpassStorage) Save(service api.Service, password string) error {
	if service.Endpoint == nil || service.Endpoint.Host == nil {
		return fmt.Errorf("service endpoint not available")
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
	database := "tsdb"      // TimescaleDB database name
	username := "tsdbadmin" // default admin user

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

func (p *PgpassStorage) Get(service api.Service) (string, error) {
	if service.Endpoint == nil || service.Endpoint.Host == nil {
		return "", fmt.Errorf("service endpoint not available")
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
	username := "tsdbadmin"

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

func (p *PgpassStorage) Remove(service api.Service) error {
	if service.Endpoint == nil || service.Endpoint.Host == nil {
		return fmt.Errorf("service endpoint not available")
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
	username := "tsdbadmin"

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

func (p *PgpassStorage) HandleSaveMessage(err error, password string, output io.Writer) {
	if err != nil {
		sanitizedErr := sanitizeErrorMessage(err, password)
		fmt.Fprintf(output, "⚠️  Failed to save password to ~/.pgpass: %s\n", sanitizedErr)
	} else {
		fmt.Fprintf(output, "🔐 Password saved to ~/.pgpass for automatic authentication\n")
	}
}

// NoStorage implements no password storage (passwords are not saved)
type NoStorage struct{}

func (n *NoStorage) Save(service api.Service, password string) error {
	return nil // Do nothing
}

func (n *NoStorage) Get(service api.Service) (string, error) {
	return "", fmt.Errorf("password storage disabled")
}

func (n *NoStorage) Remove(service api.Service) error {
	return nil // Do nothing
}

func (n *NoStorage) HandleSaveMessage(err error, password string, output io.Writer) {
	fmt.Fprintf(output, "💡 Password not saved (--password-storage=none). Make sure to store it securely.\n")
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
	default:
		return &KeyringStorage{} // Default to keyring
	}
}

// SavePasswordWithMessages handles saving a password and displaying appropriate messages
func SavePasswordWithMessages(service api.Service, password string, output io.Writer) error {
	if password == "" {
		return nil
	}

	storage := GetPasswordStorage()
	err := storage.Save(service, password)
	storage.HandleSaveMessage(err, password, output)

	return err
}
