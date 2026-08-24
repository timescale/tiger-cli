package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
)

func buildStatusCmd(app *common.App) *cobra.Command {

	cmd := &cobra.Command{
		Use:               "status",
		Aliases:           []string{"whoami"},
		Short:             "Show current authentication status and project ID",
		Long:              "Displays whether you are logged in and shows your currently configured project ID.",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, _, err := app.GetAll()
			if err != nil {
				if errors.Is(err, config.ErrNotLoggedIn) {
					return common.ExitWithCode(common.ExitAuthenticationError, config.ErrNotLoggedIn)
				}
				return err
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

	cmd.Flags().VarP(new(outputFlag), "output", "o", "output format (json, yaml, table)")
	cmd.RegisterFlagCompletionFunc("output", outputCompletion())

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
	case api.AuthInfoTypeAPIKey:
		apiKey := authInfo.APIKey
		planType := cases.Title(language.English).String(apiKey.Project.PlanType)
		table.Append("Credential Name", apiKey.Name)
		table.Append("Public Key", apiKey.PublicKey)
		table.Append("Created At", apiKey.Created.Format("2006-01-02 15:04:05 MST"))
		table.Append("Project", fmt.Sprintf("%s (%s)", apiKey.Project.Name, apiKey.Project.ID))
		table.Append("Plan Type", planType)
		table.Append("Issuing User", fmt.Sprintf("%s (%s)", apiKey.IssuingUser.Name, apiKey.IssuingUser.Email))
	case api.AuthInfoTypeOauth:
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
