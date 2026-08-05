package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
)

func buildLogoutCmd(app *common.App) *cobra.Command {
	return &cobra.Command{
		Use:               "logout",
		Short:             "Remove stored credentials",
		Long:              `Remove stored credentials. For OAuth logins, also revokes the refresh token server-side.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg := app.GetConfig()

			revokeOAuthSession(cmd, app, cfg)

			if err := cfg.RemoveCredentials(); err != nil {
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
//
// It also replaces the App's client with one that has no persist callback. The
// new client will still renew an expired access token (which is required
// because /auth/logout and the analytics endpoint are authenticated), but it
// won't persist the token back to storage, ensuring that we don't
// unintentionally restore the credentials after deleting them (the analytics
// event deferred by wrapCommands reuses the App's client after the deletion).
func revokeOAuthSession(cmd *cobra.Command, app *common.App, cfg *config.Config) {
	stored, err := cfg.GetStoredCredentials()
	if err != nil || stored.OAuth == nil {
		return
	}
	client, err := api.NewTigerClientWithToken(cfg, stored.OAuth, nil)
	if err != nil {
		return
	}
	app.SetClient(client, stored.ProjectID)

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
