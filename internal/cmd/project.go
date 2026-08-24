package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/analytics"
	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/common"
)

func buildProjectCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project <project-id>",
		Short: "Switch the active Tiger Cloud project",
		Long: `Switch the Tiger Cloud project that subsequent commands operate on.

Switching requires an OAuth login ('tiger auth login' without API keys), because an API key
is scoped to a single project. To use an API key for another project, run 'tiger auth login'
with that project's keys instead.

The default service (config key service_id) belongs to the project it was set in, so it is
cleared when you switch away.

Example:
  tiger project my-project-id`,
		Args: cobra.ExactArgs(1),
		// The argument is a project ID, which analytics must not record.
		Annotations:       map[string]string{analytics.OmitArgsAnnotation: "true"},
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			targetID := args[0]

			// Env API keys override the stored login and are pinned to one project.
			if _, _, fromEnv := common.EnvAPIKey(); fromEnv {
				return common.ExitWithCode(common.ExitAuthenticationError,
					errors.New("cannot switch projects while TIGER_PUBLIC_KEY/TIGER_SECRET_KEY are set: an API key is scoped to a single project"))
			}

			cfg, client, currentProjectID, err := app.GetAll()
			if err != nil {
				return err
			}

			stored, err := cfg.GetStoredCredentials()
			if err != nil {
				return err
			}
			if stored.OAuth == nil {
				return common.ExitWithCode(common.ExitAuthenticationError,
					errors.New("an API key is scoped to a single project. Run 'tiger auth login' without --public-key/--secret-key"))
			}

			if targetID == currentProjectID {
				cmd.Printf("Already using project %s\n", targetID)
				return nil
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			projects, err := fetchProjects(ctx, client)
			if err != nil {
				return err
			}
			if err := requireProjectAccess(cmd, projects, targetID); err != nil {
				return err
			}

			// Re-read in case the API call above refreshed and persisted the token.
			stored, err = cfg.GetStoredCredentials()
			if err != nil {
				return err
			}
			if err := cfg.StoreOAuthCredentials(stored.OAuth, targetID); err != nil {
				return fmt.Errorf("failed to store credentials: %w", err)
			}

			clearStaleDefaultService(cmd, cfg)

			// Rebuild the client so its token-persist callback carries the new
			// project ID; the old client's would revert the switch on a refresh.
			stored.ProjectID = targetID
			rebuilt, err := api.NewTigerClientForCredentials(cfg, stored)
			if err != nil {
				return fmt.Errorf("project switched, but failed to rebuild the API client: %w", err)
			}

			// Later readers in this invocation — analytics in particular — see
			// the new project.
			app.SetClient(rebuilt, targetID)

			cmd.Printf("Switched to project %s\n", targetID)
			return nil
		},
	}

	return cmd
}
