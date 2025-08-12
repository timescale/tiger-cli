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

// getServiceName returns the appropriate service name for keyring operations
// Uses a test-specific service name when running in test mode to avoid polluting the real keyring
func getServiceName() string {
	// Check if we're running in a test - look for .test suffix in the binary name
	if strings.HasSuffix(os.Args[0], ".test") {
		return "tiger-cli-test"
	}
	// Also check for test-specific arguments
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") {
			return "tiger-cli-test"
		}
	}
	return serviceName
}

var (
	// validateAPIKeyForLogin can be overridden for testing
	validateAPIKeyForLogin = func(apiKey, projectID string) error {
		return api.ValidateAPIKey(apiKey, projectID)
	}
)

func buildLoginCmd() *cobra.Command {
	var publicKeyFlag string
	var secretKeyFlag string
	var projectIDFlag string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with public and secret keys and project ID",
		Long: `Authenticate with TigerData API using public and secret keys and a project ID.
The keys will be combined and stored securely in the system keyring or as a fallback file.
The project ID will be stored in the configuration file.

You can find your API credentials and project ID at: https://console.timescale.com/dashboard/settings

Examples:
  # Interactive login (will prompt for keys and project ID if not provided)
  tiger login

  # Login with project ID (will prompt for keys if not provided)
  tiger login --project-id proj-123

  # Login with all flags
  tiger login --public-key your-public-key --secret-key your-secret-key --project-id proj-123

  # Login using environment variables
  export TIGER_PUBLIC_KEY="your-public-key"
  export TIGER_SECRET_KEY="your-secret-key"
  export TIGER_PROJECT_ID="proj-123"
  tiger login`,
		RunE: func(cmd *cobra.Command, args []string) error {
			publicKey := publicKeyFlag
			secretKey := secretKeyFlag
			projectID := projectIDFlag

			// If no keys provided via flags, check environment variables
			if publicKey == "" {
				publicKey = os.Getenv("TIGER_PUBLIC_KEY")
			}
			if secretKey == "" {
				secretKey = os.Getenv("TIGER_SECRET_KEY")
			}
			if projectID == "" {
				projectID = os.Getenv("TIGER_PROJECT_ID")
			}

			// If any credentials are missing, prompt for them all at once
			if publicKey == "" || secretKey == "" || projectID == "" {
				// Set SilenceUsage before attempting interactive prompts
				// This prevents showing usage help for TTY/environmental errors
				cmd.SilenceUsage = true

				var err error
				publicKey, secretKey, projectID, err = promptForCredentials(publicKey, secretKey, projectID)
				if err != nil {
					return fmt.Errorf("failed to get credentials: %w", err)
				}
			}

			if publicKey == "" || secretKey == "" {
				return fmt.Errorf("both public key and secret key are required")
			}

			if projectID == "" {
				return fmt.Errorf("project ID is required")
			}

			// Combine the keys in the format "public:secret" for storage
			apiKey := fmt.Sprintf("%s:%s", publicKey, secretKey)

			cmd.SilenceUsage = true

			// Validate the API key by making a test API call
			fmt.Fprintln(cmd.OutOrStdout(), "Validating API key...")
			if err := validateAPIKeyForLogin(apiKey, projectID); err != nil {
				return fmt.Errorf("API key validation failed: %w", err)
			}

			// Store the API key securely
			if err := storeAPIKey(apiKey); err != nil {
				return fmt.Errorf("failed to store API key: %w", err)
			}

			// Store project ID in config if provided
			if projectID != "" {
				if err := storeProjectID(projectID); err != nil {
					return fmt.Errorf("failed to store project ID: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Successfully logged in and stored API key securely. Set default project ID to: %s\n", projectID)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Successfully logged in and stored API key securely")
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&publicKeyFlag, "public-key", "", "Public key for authentication")
	cmd.Flags().StringVar(&secretKeyFlag, "secret-key", "", "Secret key for authentication")
	cmd.Flags().StringVar(&projectIDFlag, "project-id", "", "Default project ID to set in configuration")

	return cmd
}

func buildLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials",
		Long:  `Remove stored API key and clear authentication credentials.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			if err := removeAPIKey(); err != nil {
				return fmt.Errorf("failed to remove API key: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Successfully logged out and removed stored credentials")
			return nil
		},
	}
}

func buildWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current user information",
		Long:  `Show information about the currently authenticated user.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

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
}

func buildAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication and credentials",
		Long:  `Manage authentication and credentials for TigerData Cloud Platform.`,
	}

	cmd.AddCommand(buildLoginCmd())
	cmd.AddCommand(buildLogoutCmd())
	cmd.AddCommand(buildWhoamiCmd())

	return cmd
}

// storeAPIKey stores the API key using keyring with file fallback
func storeAPIKey(apiKey string) error {
	// Try keyring first
	err := keyring.Set(getServiceName(), username, apiKey)
	if err == nil {
		return nil
	}

	// Fallback to file storage
	return storeAPIKeyToFile(apiKey)
}

// getAPIKey retrieves the API key from keyring or file fallback
func getAPIKey() (string, error) {
	// Try keyring first
	apiKey, err := keyring.Get(getServiceName(), username)
	if err == nil {
		return apiKey, nil
	}

	// Fallback to file storage
	return getAPIKeyFromFile()
}

// removeAPIKey removes the API key from keyring and file fallback
func removeAPIKey() error {
	// Try to remove from keyring (ignore errors as it might not exist)
	keyring.Delete(getServiceName(), username)

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

// promptForCredentials prompts the user to enter any missing credentials
func promptForCredentials(publicKey, secretKey, projectID string) (string, string, string, error) {
	// Check if we're in a terminal for interactive input
	if !term.IsTerminal(int(syscall.Stdin)) {
		return "", "", "", fmt.Errorf("TTY not detected - credentials required. Use flags (--public-key, --secret-key, --project-id) or environment variables (TIGER_PUBLIC_KEY, TIGER_SECRET_KEY, TIGER_PROJECT_ID)")
	}

	fmt.Println("You can find your API credentials and project ID at: https://console.timescale.com/dashboard/settings")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// Prompt for public key if missing
	if publicKey == "" {
		fmt.Print("Enter your public key: ")
		var err error
		publicKey, err = reader.ReadString('\n')
		if err != nil {
			return "", "", "", err
		}
		publicKey = strings.TrimSpace(publicKey)
	}

	// Prompt for secret key if missing
	if secretKey == "" {
		fmt.Print("Enter your secret key: ")
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return "", "", "", err
		}
		fmt.Println() // Print newline after hidden input
		secretKey = strings.TrimSpace(string(bytePassword))
	}

	// Prompt for project ID if missing
	if projectID == "" {
		fmt.Print("Enter your project ID: ")
		var err error
		projectID, err = reader.ReadString('\n')
		if err != nil {
			return "", "", "", err
		}
		projectID = strings.TrimSpace(projectID)
	}

	return publicKey, secretKey, projectID, nil
}

// storeProjectID stores the project ID in the configuration file
func storeProjectID(projectID string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	return cfg.Set("project_id", projectID)
}
