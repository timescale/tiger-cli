package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"

	"github.com/tigerdata/tiger-cli/internal/tiger/api"
	"github.com/tigerdata/tiger-cli/internal/tiger/config"
)

const (
	serviceName = "tiger-cli"
	username    = "api-key"
)

var (
	apiKeyFlag string
	// validateAPIKeyForLogin can be overridden for testing
	validateAPIKeyForLogin = api.ValidateAPIKey
)

// authCmd represents the auth command
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication and credentials",
	Long:  `Manage authentication and credentials for TigerData Cloud Platform.`,
}

// loginCmd represents the login command
var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with API token",
	Long: `Authenticate with TigerData API using an API token.
The API key will be stored securely in the system keyring or as a fallback file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := apiKeyFlag

		// If no API key provided via flag, check environment variable
		if apiKey == "" {
			apiKey = os.Getenv("TIGER_API_KEY")
		}

		// If still no API key, prompt for it interactively
		if apiKey == "" {
			var err error
			apiKey, err = promptForAPIKey()
			if err != nil {
				return fmt.Errorf("failed to get API key: %w", err)
			}
		}

		if apiKey == "" {
			return fmt.Errorf("API key is required")
		}

		// Validate the API key by making a test API call
		fmt.Fprintln(cmd.OutOrStdout(), "Validating API key...")
		if err := validateAPIKeyForLogin(apiKey); err != nil {
			return fmt.Errorf("API key validation failed: %w", err)
		}

		// Store the API key securely
		if err := storeAPIKey(apiKey); err != nil {
			return fmt.Errorf("failed to store API key: %w", err)
		}

		// TODO: Retrieve project ID from API and store in config
		// For now, just confirm successful storage
		fmt.Fprintln(cmd.OutOrStdout(), "Successfully logged in and stored API key securely")
		
		return nil
	},
}

// logoutCmd represents the logout command
var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials",
	Long:  `Remove stored API key and clear authentication credentials.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := removeAPIKey(); err != nil {
			return fmt.Errorf("failed to remove API key: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Successfully logged out and removed stored credentials")
		return nil
	},
}

// whoamiCmd represents the whoami command
var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current user information",
	Long:  `Show information about the currently authenticated user.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey, err := getAPIKey()
		if err != nil {
			return fmt.Errorf("not logged in: %w", err)
		}

		if apiKey == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "Not logged in")
			return nil
		}

		// TODO: Make API call to get user information
		fmt.Fprintln(cmd.OutOrStdout(), "Logged in (API key stored)")
		
		return nil
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(loginCmd)
	authCmd.AddCommand(logoutCmd)
	authCmd.AddCommand(whoamiCmd)

	// Add flags
	loginCmd.Flags().StringVar(&apiKeyFlag, "api-key", "", "API key for authentication")
}

// storeAPIKey stores the API key using keyring with file fallback
func storeAPIKey(apiKey string) error {
	// Try keyring first
	err := keyring.Set(serviceName, username, apiKey)
	if err == nil {
		return nil
	}

	// Fallback to file storage
	return storeAPIKeyToFile(apiKey)
}

// getAPIKey retrieves the API key from keyring or file fallback
func getAPIKey() (string, error) {
	// Try keyring first
	apiKey, err := keyring.Get(serviceName, username)
	if err == nil {
		return apiKey, nil
	}

	// Fallback to file storage
	return getAPIKeyFromFile()
}

// removeAPIKey removes the API key from keyring and file fallback
func removeAPIKey() error {
	// Try to remove from keyring (ignore errors as it might not exist)
	keyring.Delete(serviceName, username)

	// Remove from file fallback
	return removeAPIKeyFromFile()
}

// storeAPIKeyToFile stores API key to ~/.config/tiger/api-key with restricted permissions
func storeAPIKeyToFile(apiKey string) error {
	configDir := config.GetConfigDir()
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

	return nil
}

// getAPIKeyFromFile retrieves API key from ~/.config/tiger/api-key
func getAPIKeyFromFile() (string, error) {
	configDir := config.GetConfigDir()
	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)

	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("not logged in")
		}
		return "", fmt.Errorf("failed to read API key file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// removeAPIKeyFromFile removes the API key file
func removeAPIKeyFromFile() error {
	configDir := config.GetConfigDir()
	apiKeyFile := fmt.Sprintf("%s/api-key", configDir)

	err := os.Remove(apiKeyFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove API key file: %w", err)
	}

	return nil
}

// promptForAPIKey prompts the user to enter their API key securely
func promptForAPIKey() (string, error) {
	fmt.Print("Enter your API key: ")

	// Check if we're in a terminal for secure input
	if term.IsTerminal(int(syscall.Stdin)) {
		// Use terminal to hide input
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return "", err
		}
		fmt.Println() // Print newline after hidden input
		return strings.TrimSpace(string(bytePassword)), nil
	} else {
		// Fallback to regular input if not in terminal
		reader := bufio.NewReader(os.Stdin)
		apiKey, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(apiKey), nil
	}
}