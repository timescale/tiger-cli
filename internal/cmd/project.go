package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/analytics"
	"github.com/timescale/tiger-cli/internal/common"
)

func buildProjectCmd(app *common.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project [project-id]",
		Aliases: []string{"projects", "proj"},
		Short:   "Switch the active Tiger Cloud project",
		Long: `Switch the Tiger Cloud project that subsequent commands operate on.

Without an argument, the accessible projects are listed for you to pick from. Pass a project ID
to switch without the prompt, which is what you want when no terminal is available.

Switching projects requires an OAuth login ('tiger auth login' without --public-key/--secret-key),
because an API key is scoped to a single project.

The default service (config key service_id) belongs to the project it was set in, so it is cleared
when you switch away.

Examples:
  # Pick a project interactively
  tiger project

  # Switch to a specific project
  tiger project rp1pz7uyae`,
		Args: cobra.MaximumNArgs(1),
		// The argument is a project ID, which analytics excludes.
		Annotations:       map[string]string{analytics.OmitArgsAnnotation: "true"},
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			// Refuse credentials a switch can't move, before GetAll can fail
			// validating them and bury this explanation under a generic error.
			if _, fromEnv := common.EnvAPIKey(); fromEnv {
				return fmt.Errorf("cannot switch projects while TIGER_PUBLIC_KEY/TIGER_SECRET_KEY are set: those credentials take precedence over the stored login, and an API key is scoped to a single project")
			}

			// GetAll's error already carries the auth exit code and login hint.
			cfg, client, currentProjectID, err := app.GetAll()
			if err != nil {
				return err
			}
			stored, err := cfg.GetStoredCredentials()
			if err != nil {
				return err
			}
			if stored.OAuth == nil {
				return fmt.Errorf("switching projects requires an OAuth login: an API key is scoped to a single project. Run 'tiger auth login' without --public-key/--secret-key")
			}

			projects, err := fetchProjects(cmd.Context(), client)
			if err != nil {
				return err
			}

			var requested string
			if len(args) > 0 {
				requested = args[0]
			}
			project, err := resolveProjectID(cmd, projects, requested, "pass the project ID as an argument")
			if err != nil {
				return err
			}

			if project.ID == currentProjectID {
				cmd.Printf("Already using project %s\n", describeProject(project))
				return nil
			}

			if err := cfg.SwitchProject(project.ID); err != nil {
				return fmt.Errorf("failed to store credentials: %w", err)
			}

			clearStaleDefaultService(cmd, cfg, currentProjectID, project.ID)

			// Later readers in this invocation — the trailing analytics event
			// in particular — see the new project.
			app.SetClient(client, project.ID)

			cmd.Printf("Switched to project %s\n", describeProject(project))
			return nil
		},
	}

	return cmd
}
