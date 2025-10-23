package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// validateAPIKeyForLogin can be overridden for testing
var validateAPIKeyForLogin = api.ValidateAPIKey

// nextStepsMessage is the message shown after successful login
const nextStepsMessage = `
🎉 Next steps:
• Install MCP server for your favorite AI coding tool: tiger mcp install
• List existing services: tiger service list
• Create a new service: tiger service create
`

type credentials struct {
	publicKey string
	secretKey string
	projectID string
}

func buildLoginCmd() *cobra.Command {
	var flags credentials

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with TigerData API",
		Long: `Authenticate with TigerData API using predefined keys or an interactive OAuth flow

By default, the command will launch an interactive OAuth flow in your browser to create new API keys.
The OAuth flow will:
- Open your browser for authentication
- Let you select a project (if you have multiple)
- Create API keys automatically for the selected project

The keys will be combined and stored securely in the system keyring or as a fallback file.
The project ID will be stored in the configuration file.

You may also provide API keys via flags or environment variables, in which case
they will be used directly. The CLI will prompt for any missing information.

You can find your API credentials and project ID at: https://console.cloud.timescale.com/dashboard/settings

Examples:
  # Interactive login with OAuth (opens browser, creates API keys automatically)
  tiger auth login

  # Login with project ID (will prompt for keys if not provided)
  tiger auth login --project-id your-project-id

  # Login with keys and project ID
  tiger auth login --public-key your-public-key --secret-key your-secret-key --project-id your-project-id

  # Login using environment variables
  export TIGER_PUBLIC_KEY="your-public-key"
  export TIGER_SECRET_KEY="your-secret-key"
  export TIGER_PROJECT_ID="proj-123"
  tiger auth login`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// TODO: It should be possible to get the projectID corresponding to the
			// API keys programmatically, making the project-id flag/env var unnecessary
			creds := credentials{
				publicKey: flagOrEnvVar(flags.publicKey, "TIGER_PUBLIC_KEY"),
				secretKey: flagOrEnvVar(flags.secretKey, "TIGER_SECRET_KEY"),
				projectID: flagOrEnvVar(flags.projectID, "TIGER_PROJECT_ID"),
			}

			if creds.publicKey == "" && creds.secretKey == "" && creds.projectID == "" {
				// If no credentials were provided, start interactive OAuth login flow
				l := &oauthLogin{
					authURL:    cfg.ConsoleURL + "/oauth/authorize",
					tokenURL:   cfg.GatewayURL + "/idp/external/cli/token",
					successURL: cfg.ConsoleURL + "/oauth/code/success",
					graphql: &GraphQLClient{
						URL: cfg.GatewayURL + "/query",
					},
					out: cmd.OutOrStdout(),
				}

				creds, err = l.loginWithOAuth(cmd.Context())
				if err != nil {
					return err
				}
			} else if creds.publicKey == "" || creds.secretKey == "" || creds.projectID == "" {
				// If some credentials were provided, prompt for missing ones
				creds, err = promptForCredentials(cfg.ConsoleURL, creds)
				if err != nil {
					return fmt.Errorf("failed to get credentials: %w", err)
				}

				if creds.publicKey == "" || creds.secretKey == "" {
					return fmt.Errorf("both public key and secret key are required")
				}

				if creds.projectID == "" {
					return fmt.Errorf("project ID is required")
				}
			}

			// Combine the keys in the format "public:secret" for storage
			apiKey := fmt.Sprintf("%s:%s", creds.publicKey, creds.secretKey)

			// Validate the API key by making a test API call
			fmt.Fprintln(cmd.OutOrStdout(), "Validating API key...")
			if err := validateAPIKeyForLogin(cmd.Context(), apiKey, creds.projectID); err != nil {
				return fmt.Errorf("API key validation failed: %w", err)
			}

			// Store the credentials (API key + project ID) together securely
			if err := config.StoreCredentials(apiKey, creds.projectID); err != nil {
				return fmt.Errorf("failed to store credentials: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Successfully logged in (project: %s)\n", creds.projectID)

			// Show helpful next steps
			fmt.Fprint(cmd.OutOrStdout(), nextStepsMessage)

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&flags.publicKey, "public-key", "", "Public key for authentication")
	cmd.Flags().StringVar(&flags.secretKey, "secret-key", "", "Secret key for authentication")
	cmd.Flags().StringVar(&flags.projectID, "project-id", "", "Default project ID to set in configuration")

	return cmd
}

func buildLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials",
		Long:  `Remove stored API key and clear authentication credentials.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			if err := config.RemoveCredentials(); err != nil {
				return fmt.Errorf("failed to remove credentials: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Successfully logged out and removed stored credentials")
			return nil
		},
	}
}

func buildStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current authentication status and project ID",
		Long:  "Displays whether you are logged in and shows your currently configured project ID.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			_, projectID, err := config.GetCredentials()
			if err != nil {
				return err
			}

			// TODO: Make API call to get token information
			fmt.Fprintln(cmd.OutOrStdout(), "Logged in (API key stored)")
			fmt.Fprintf(cmd.OutOrStdout(), "Project ID: %s\n", projectID)

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
	cmd.AddCommand(buildStatusCmd())

	return cmd
}

func flagOrEnvVar(flagVal, envVarName string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(envVarName)
}

// promptForCredentials prompts the user to enter any missing credentials
func promptForCredentials(consoleURL string, creds credentials) (credentials, error) {
	// Check if we're in a terminal for interactive input
	if !util.IsTerminal(os.Stdin) {
		return credentials{}, fmt.Errorf("TTY not detected - credentials required. Use flags (--public-key, --secret-key, --project-id) or environment variables (TIGER_PUBLIC_KEY, TIGER_SECRET_KEY, TIGER_PROJECT_ID)")
	}

	fmt.Printf("You can find your API credentials and project ID at: %s/dashboard/settings\n\n", consoleURL)

	reader := bufio.NewReader(os.Stdin)

	// Prompt for public key if missing
	if creds.publicKey == "" {
		fmt.Print("Enter your public key: ")
		publicKey, err := reader.ReadString('\n')
		if err != nil {
			return credentials{}, err
		}
		creds.publicKey = strings.TrimSpace(publicKey)
	}

	// Prompt for secret key if missing
	if creds.secretKey == "" {
		fmt.Print("Enter your secret key: ")
		bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return credentials{}, err
		}
		fmt.Println() // Print newline after hidden input
		creds.secretKey = strings.TrimSpace(string(bytePassword))
	}

	// Prompt for project ID if missing
	if creds.projectID == "" {
		fmt.Print("Enter your project ID: ")
		projectID, err := reader.ReadString('\n')
		if err != nil {
			return credentials{}, err
		}
		creds.projectID = strings.TrimSpace(projectID)
	}

	return creds, nil
}
