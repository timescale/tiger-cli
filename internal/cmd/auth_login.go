package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

// validateAPIKey can be overridden for testing
var validateAPIKey = common.ValidateAPIKey

// nextStepsMessage is the message shown after successful login
const nextStepsMessage = `
🎉 Next steps:
• Install MCP server for your favorite AI coding tool: tiger mcp install
• List existing services: tiger service list
• Create a new service: tiger service create
• Enable read-only mode: tiger config set read_only true
`

type credentials struct {
	publicKey string
	secretKey string
}

func buildLoginCmd() *cobra.Command {
	var flags credentials

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Tiger Cloud API",
		Long: `Authenticate with Tiger Cloud API using predefined keys or an interactive OAuth flow

By default, the command will launch an interactive OAuth flow in your browser to sign in.
The OAuth flow will:
- Open your browser for authentication
- Let you select a project (if you have multiple)
- Store an OAuth session for the selected project

The credentials and project ID will be stored securely in the system keyring, or in a fallback file with
restricted permissions.

You may also provide API keys via flags or environment variables, in which case they will be used
directly. The CLI will prompt for any missing information.

You can find your API credentials at: https://console.cloud.tigerdata.com/dashboard/settings

Examples:
  # Interactive login with OAuth (opens browser)
  tiger auth login

  # Login with keys (project ID will be auto-detected)
  tiger auth login --public-key your-public-key --secret-key your-secret-key

  # Login using environment variables
  export TIGER_PUBLIC_KEY="your-public-key"
  export TIGER_SECRET_KEY="your-secret-key"
  tiger auth login`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			creds := credentials{
				publicKey: flagOrEnvVar(flags.publicKey, "TIGER_PUBLIC_KEY"),
				secretKey: flagOrEnvVar(flags.secretKey, "TIGER_SECRET_KEY"),
			}

			if creds.publicKey == "" && creds.secretKey == "" {
				l := &oauthLogin{
					cfg:        cfg,
					authURL:    cfg.ConsoleURL + "/oauth/authorize",
					tokenURL:   cfg.GatewayURL + "/idp/external/cli/token",
					successURL: cfg.ConsoleURL + "/oauth/code/success",
					out:        cmd.OutOrStdout(),
				}

				token, client, projectID, err := l.loginWithOAuth(cmd.Context())
				if err != nil {
					return err
				}
				if err := config.StoreOAuthCredentials(token, projectID); err != nil {
					return fmt.Errorf("failed to store credentials: %w", err)
				}
				// Identify the user for analytics.
				common.IdentifyOAuthUser(cmd.Context(), cfg, client, projectID)
				finishLogin(cmd, projectID)
				return nil
			} else if creds.publicKey == "" || creds.secretKey == "" {
				creds, err = promptForCredentials(cmd.Context(), cfg.ConsoleURL, creds)
				if err != nil {
					return fmt.Errorf("failed to get credentials: %w", err)
				}
				if creds.publicKey == "" || creds.secretKey == "" {
					return fmt.Errorf("both public key and secret key are required")
				}
			}

			apiKey := fmt.Sprintf("%s:%s", creds.publicKey, creds.secretKey)
			client, err := api.NewTigerClient(cfg, apiKey)
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Validating API key...")
			authInfo, err := validateAPIKey(cmd.Context(), cfg, client)
			if err != nil {
				return fmt.Errorf("API key validation failed: %w", err)
			}
			if err := config.StoreCredentials(apiKey, authInfo.ApiKey.Project.Id); err != nil {
				return fmt.Errorf("failed to store credentials: %w", err)
			}
			finishLogin(cmd, authInfo.ApiKey.Project.Id)
			return nil
		},
	}

	cmd.Flags().StringVar(&flags.publicKey, "public-key", "", "Public key for authentication")
	cmd.Flags().StringVar(&flags.secretKey, "secret-key", "", "Secret key for authentication")

	return cmd
}

func finishLogin(cmd *cobra.Command, projectID string) {
	fmt.Fprintf(cmd.OutOrStdout(), "Successfully logged in (project: %s)\n", projectID)
	fmt.Fprint(cmd.OutOrStdout(), nextStepsMessage)
}

func flagOrEnvVar(flagVal, envVarName string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(envVarName)
}

func promptForCredentials(ctx context.Context, consoleURL string, creds credentials) (credentials, error) {
	if !util.IsTerminal(os.Stdin) {
		return credentials{}, fmt.Errorf("TTY not detected - credentials required. Use flags (--public-key, --secret-key) or environment variables (TIGER_PUBLIC_KEY, TIGER_SECRET_KEY)")
	}

	fmt.Printf("You can find your API credentials at: %s/dashboard/settings\n\n", consoleURL)

	reader := bufio.NewReader(os.Stdin)

	if creds.publicKey == "" {
		fmt.Print("Enter your public key: ")
		publicKey, err := readString(ctx, func() (string, error) { return reader.ReadString('\n') })
		if err != nil {
			return credentials{}, err
		}
		creds.publicKey = publicKey
	}

	if creds.secretKey == "" {
		fmt.Print("Enter your secret key: ")
		password, err := readString(ctx, func() (string, error) {
			val, err := term.ReadPassword(int(os.Stdin.Fd()))
			return string(val), err
		})
		if err != nil {
			return credentials{}, err
		}
		fmt.Println()
		creds.secretKey = password
	}

	return creds, nil
}
