package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/timescale/tiger-cli/internal/tiger/analytics"
	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// validateAndGetAuthInfo can be overridden for testing
var validateAndGetAuthInfo = validateAndGetAuthInfoImpl

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
}

func buildLoginCmd() *cobra.Command {
	var flags credentials

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Tiger Cloud API",
		Long: `Authenticate with Tiger Cloud API using predefined keys or an interactive OAuth flow

By default, the command will launch an interactive OAuth flow in your browser to create new API keys.
The OAuth flow will:
- Open your browser for authentication
- Let you select a project (if you have multiple)
- Create API keys automatically for the selected project

The keys and project ID will be stored securely in the system keyring, or in a fallback file with
restricted permissions.

You may also provide API keys via flags or environment variables, in which case they will be used
directly. The CLI will prompt for any missing information.

You can find your API credentials at: https://console.cloud.timescale.com/dashboard/settings

Examples:
  # Interactive login with OAuth (opens browser, creates API keys automatically)
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

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			creds := credentials{
				publicKey: flagOrEnvVar(flags.publicKey, "TIGER_PUBLIC_KEY"),
				secretKey: flagOrEnvVar(flags.secretKey, "TIGER_SECRET_KEY"),
			}

			if creds.publicKey == "" && creds.secretKey == "" {
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
			} else if creds.publicKey == "" || creds.secretKey == "" {
				// If some credentials were provided, prompt for missing ones
				creds, err = promptForCredentials(cmd.Context(), cfg.ConsoleURL, creds)
				if err != nil {
					return fmt.Errorf("failed to get credentials: %w", err)
				}

				if creds.publicKey == "" || creds.secretKey == "" {
					return fmt.Errorf("both public key and secret key are required")
				}
			}

			// Combine the keys in the format "public:secret" for storage
			apiKey := fmt.Sprintf("%s:%s", creds.publicKey, creds.secretKey)

			// Validate the API key and get auth info by calling the /auth/info endpoint
			fmt.Fprintln(cmd.OutOrStdout(), "Validating API key...")
			authInfo, err := validateAndGetAuthInfo(cmd.Context(), cfg, apiKey)
			if err != nil {
				return fmt.Errorf("API key validation failed: %w", err)
			}

			// Store the credentials (API key + project ID) together securely
			if err := config.StoreCredentials(apiKey, authInfo.ApiKey.Project.Id); err != nil {
				return fmt.Errorf("failed to store credentials: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Successfully logged in (project: %s)\n", authInfo.ApiKey.Project.Id)

			// Show helpful next steps
			fmt.Fprint(cmd.OutOrStdout(), nextStepsMessage)

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringVar(&flags.publicKey, "public-key", "", "Public key for authentication")
	cmd.Flags().StringVar(&flags.secretKey, "secret-key", "", "Secret key for authentication")

	return cmd
}

func buildLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "logout",
		Short:             "Remove stored credentials",
		Long:              `Remove stored API key and clear authentication credentials.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
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
	var output string

	cmd := &cobra.Command{
		Use:               "status",
		Short:             "Show current authentication status and project ID",
		Long:              "Displays whether you are logged in and shows your currently configured project ID.",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		PreRunE:           bindFlags("output"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Get config
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			apiKey, _, err := config.GetCredentials()
			if err != nil {
				return err
			}

			// Create API client
			client, err := api.NewTigerClient(cfg, apiKey)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			// Make API call to get auth information
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			resp, err := client.GetAuthInfoWithResponse(ctx)
			if err != nil {
				return fmt.Errorf("failed to get auth information: %w", err)
			}

			// Handle API response
			if resp.StatusCode() != 200 {
				return common.ExitWithErrorFromStatusCode(resp.StatusCode(), resp.JSON4XX)
			}

			if resp.JSON200 == nil {
				return fmt.Errorf("empty response from API")
			}

			authInfo := *resp.JSON200

			// Output auth info in requested format
			return outputAuthInfo(cmd, authInfo, cfg.Output)
		},
	}

	cmd.Flags().VarP((*outputFlag)(&output), "output", "o", "output format (json, yaml, table)")

	return cmd
}

func buildAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication and credentials",
		Long:  `Manage authentication and credentials for Tiger Cloud platform.`,
	}

	cmd.AddCommand(buildLoginCmd())
	cmd.AddCommand(buildLogoutCmd())
	cmd.AddCommand(buildStatusCmd())

	return cmd
}

// outputAuthInfo formats and outputs authentication information based on the specified format
func outputAuthInfo(cmd *cobra.Command, authInfo api.AuthInfo, format string) error {
	outputWriter := cmd.OutOrStdout()

	switch strings.ToLower(format) {
	case "json":
		return util.SerializeToJSON(outputWriter, authInfo)
	case "yaml":
		return util.SerializeToYAML(outputWriter, authInfo, true)
	default: // table format (default)
		return outputAuthInfoTable(authInfo, outputWriter)
	}
}

// outputAuthInfoTable outputs authentication information in a formatted table
func outputAuthInfoTable(authInfo api.AuthInfo, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("PROPERTY", "VALUE")

	// Convert plan type to title case for display
	planType := cases.Title(language.English).String(authInfo.ApiKey.Project.PlanType)

	table.Append("Status", "Logged in")
	table.Append("Credential Name", authInfo.ApiKey.Name)
	table.Append("Public Key", authInfo.ApiKey.PublicKey)
	table.Append("Created At", authInfo.ApiKey.Created.Format("2006-01-02 15:04:05 MST"))
	table.Append("Project", fmt.Sprintf("%s (%s)", authInfo.ApiKey.Project.Name, authInfo.ApiKey.Project.Id))
	table.Append("Plan Type", planType)
	table.Append("Issuing User", fmt.Sprintf("%s (%s)", authInfo.ApiKey.IssuingUser.Name, authInfo.ApiKey.IssuingUser.Email))

	return table.Render()
}

func flagOrEnvVar(flagVal, envVarName string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(envVarName)
}

// promptForCredentials prompts the user to enter any missing credentials
func promptForCredentials(ctx context.Context, consoleURL string, creds credentials) (credentials, error) {
	// Check if we're in a terminal for interactive input
	if !util.IsTerminal(os.Stdin) {
		return credentials{}, fmt.Errorf("TTY not detected - credentials required. Use flags (--public-key, --secret-key) or environment variables (TIGER_PUBLIC_KEY, TIGER_SECRET_KEY)")
	}

	fmt.Printf("You can find your API credentials at: %s/dashboard/settings\n\n", consoleURL)

	reader := bufio.NewReader(os.Stdin)

	// Prompt for public key if missing
	if creds.publicKey == "" {
		fmt.Print("Enter your public key: ")
		publicKey, err := readString(ctx, func() (string, error) { return reader.ReadString('\n') })
		if err != nil {
			return credentials{}, err
		}
		creds.publicKey = publicKey
	}

	// Prompt for secret key if missing
	if creds.secretKey == "" {
		fmt.Print("Enter your secret key: ")
		password, err := readString(ctx, func() (string, error) {
			val, err := term.ReadPassword(int(os.Stdin.Fd()))
			return string(val), err
		})
		if err != nil {
			return credentials{}, err
		}
		fmt.Println() // Print newline after hidden input
		creds.secretKey = password
	}

	return creds, nil
}

// validateAndGetAuthInfoImpl validates the API key and returns authentication information
// by calling the /auth/info endpoint.
func validateAndGetAuthInfoImpl(ctx context.Context, cfg *config.Config, apiKey string) (*api.AuthInfo, error) {
	client, err := api.NewTigerClient(cfg, apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Call the /auth/info endpoint to validate credentials and get auth info
	resp, err := client.GetAuthInfoWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	// Check the response status
	if resp.StatusCode() != 200 {
		if resp.JSON4XX != nil {
			return nil, resp.JSON4XX
		}
		return nil, fmt.Errorf("unexpected API response: %d", resp.StatusCode())
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from API")
	}

	authInfo := resp.JSON200

	// Identify the user with analytics
	a := analytics.New(cfg, client, authInfo.ApiKey.Project.Id)
	a.Identify(
		analytics.Property("userId", authInfo.ApiKey.IssuingUser.Id),
		analytics.Property("email", string(authInfo.ApiKey.IssuingUser.Email)),
		analytics.Property("planType", authInfo.ApiKey.Project.PlanType),
	)

	return authInfo, nil
}
