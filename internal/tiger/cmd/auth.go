package cmd

import (
	"bufio"
	"context"
	"errors"
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

	"github.com/timescale/tiger-cli/internal/tiger/api"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// validateAPIKey can be overridden for testing
var validateAPIKey = common.ValidateAPIKey

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

func buildLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "logout",
		Short:             "Remove stored credentials",
		Long:              `Remove stored credentials. For OAuth logins, also revokes the refresh token server-side.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			revokeOAuthSession(cmd)

			if err := config.RemoveCredentials(); err != nil {
				return fmt.Errorf("failed to remove credentials: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Successfully logged out and removed stored credentials")
			return nil
		},
	}
}

// revokeOAuthSession asks the server to revoke the refresh token for an OAuth
// session. Failures are intentionally non-fatal — local credential removal
// must always succeed even if the server is unreachable or returns 501.
func revokeOAuthSession(cmd *cobra.Command) {
	stored, err := config.GetStoredCredentials()
	if err != nil || stored.OAuth == nil {
		return
	}
	cfg, err := config.Load()
	if err != nil {
		return
	}
	client, err := api.NewTigerClientWithToken(cfg, stored.OAuth, nil)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	body := api.LogoutJSONRequestBody{}
	if rt := stored.OAuth.RefreshToken; rt != "" {
		body.RefreshToken = &rt
	}
	if _, err := client.LogoutWithResponse(ctx, body); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: server-side logout failed: %v\n", err)
	}
}

func finishLogin(cmd *cobra.Command, projectID string) {
	fmt.Fprintf(cmd.OutOrStdout(), "Successfully logged in (project: %s)\n", projectID)
	fmt.Fprint(cmd.OutOrStdout(), nextStepsMessage)
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

			// Load config and API client
			cfg, err := common.LoadConfig(cmd.Context())
			if err != nil {
				if errors.Is(err, config.ErrNotLoggedIn) {
					return common.ExitWithCode(common.ExitAuthenticationError, config.ErrNotLoggedIn)
				}
				return err
			}

			// Make API call to get auth information
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			resp, err := cfg.Client.GetAuthInfoWithResponse(ctx)
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
		return util.SerializeToYAML(outputWriter, authInfo)
	default: // table format (default)
		return outputAuthInfoTable(authInfo, outputWriter)
	}
}

func outputAuthInfoTable(authInfo api.AuthInfo, output io.Writer) error {
	table := tablewriter.NewWriter(output)
	table.Header("PROPERTY", "VALUE")
	table.Append("Status", "Logged in")

	switch authInfo.Type {
	case api.ApiKey:
		apiKey := authInfo.ApiKey
		planType := cases.Title(language.English).String(apiKey.Project.PlanType)
		table.Append("Credential Name", apiKey.Name)
		table.Append("Public Key", apiKey.PublicKey)
		table.Append("Created At", apiKey.Created.Format("2006-01-02 15:04:05 MST"))
		table.Append("Project", fmt.Sprintf("%s (%s)", apiKey.Project.Name, apiKey.Project.Id))
		table.Append("Plan Type", planType)
		table.Append("Issuing User", fmt.Sprintf("%s (%s)", apiKey.IssuingUser.Name, apiKey.IssuingUser.Email))
	case api.Oauth:
		user := authInfo.Oauth.User
		displayName := string(user.Email)
		if user.Name != "" {
			displayName = fmt.Sprintf("%s (%s)", user.Name, user.Email)
		}
		table.Append("Auth Method", "OAuth")
		table.Append("User", displayName)
	default:
		return fmt.Errorf("unsupported auth info type: %q", authInfo.Type)
	}

	return table.Render()
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
