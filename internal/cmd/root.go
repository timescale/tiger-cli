package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/analytics"
	"github.com/timescale/tiger-cli/internal/common"
	"github.com/timescale/tiger-cli/internal/config"
	"github.com/timescale/tiger-cli/internal/util"
	"github.com/timescale/tiger-cli/internal/version"
)

func buildRootCmd(ctx context.Context) (*cobra.Command, error) {
	// TIGER_EXPERIMENTAL toggles preview-stage commands and MCP tools. Read at
	// build time so ungated subtrees are never added to the command tree — the
	// command literally does not exist when the env var is unset/false, so it
	// won't appear in help, tab-completion, or `unknown command` disambiguation.
	// This is intentionally undocumented (env var only, no config key); see
	// CLAUDE.md's "Experimental Feature Gating" section.
	experimental, _ := strconv.ParseBool(os.Getenv("TIGER_EXPERIMENTAL"))

	app := &common.App{
		Experimental: experimental,
	}

	cmd := &cobra.Command{
		Use:   "tiger",
		Short: "Tiger CLI - Tiger Cloud Platform command-line interface",
		Long: `Tiger CLI is a command-line interface for managing Tiger Cloud platform resources.
Built as a single Go binary, it provides comprehensive tools for managing database services,
VPCs, replicas, and related infrastructure components.

To get started, run:

tiger auth login

`,
	}

	// Every command runs with this context — cobra copies it onto the command it
	// executes — so handlers can use cmd.Context() for cancellation.
	cmd.SetContext(ctx)

	// Add persistent flags. Values are read back from the config (see
	// flagBindings in internal/config) rather than from the flag variables, so
	// only --skip-update-check — which isn't a config value — is captured here.
	cmd.PersistentFlags().Bool("analytics", true, "enable/disable usage analytics")
	cmd.PersistentFlags().Bool("color", true, "enable colored output")
	cmd.PersistentFlags().String("config-dir", config.GetDefaultConfigDir(), "config directory")
	cmd.PersistentFlags().String("password-storage", config.DefaultPasswordStorage, "password storage method (keyring, pgpass, none)")
	cmd.PersistentFlags().String("service-id", "", "service ID")
	skipUpdateCheck := cmd.PersistentFlags().Bool("skip-update-check", false, "skip checking for updates on startup")

	// Add all subcommands
	cmd.AddCommand(buildVersionCmd(app))
	cmd.AddCommand(buildUpgradeCmd(app))
	cmd.AddCommand(buildConfigCmd(app))
	cmd.AddCommand(buildAuthCmd(app))
	cmd.AddCommand(buildServiceCmd(app))
	cmd.AddCommand(buildDbCmd(app))
	cmd.AddCommand(buildMCPCmd(app))

	wrapCommands(cmd, app, skipUpdateCheck)

	return cmd, nil
}

// wrapCommands recursively wraps the RunE of every command in the tree rooted at
// cmd with the shared per-invocation lifecycle: loading the config and API
// client, configuring color output, checking for a newer release, and tracking
// analytics.
//
// Commands added to the tree after this runs (cobra's built-in help, completion,
// and __complete commands) are not wrapped and so skip the load entirely, which
// keeps `tiger --help` and tab completion away from the config file, the system
// keyring, and the network. Completion functions that do need the config or
// client load on demand via withAppLoad. Group commands (`tiger service`) have no
// RunE of their own and only print help, so they're skipped as well.
func wrapCommands(cmd *cobra.Command, app *common.App, skipUpdateCheck *bool) {
	// Wrap this command's RunE if it exists
	if cmd.RunE != nil {
		originalRunE := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) (runErr error) {
			// Load the config and API client once for the whole invocation.
			// c.Flags() carries the persistent flags inherited from parents, so
			// flags take precedence over env vars and the config file.
			app.SetFlags(c.Flags())
			cfg, _, _, err := app.Load(c.Context())
			if err != nil {
				return err
			}

			if !cfg.Color {
				color.NoColor = true
			}

			// Check for a newer release in the background, printing the result
			// after the command's own output.
			defer versionCheck(c, cfg, *skipUpdateCheck)()

			// Track analytics. The config and client are re-read from the App so
			// changes the command made are reflected: `tiger config set analytics
			// false` sends no event, and `tiger auth login` is attributed to the
			// credentials it just stored.
			start := time.Now()
			defer func() {
				cfg, client, projectID := app.TryGetAll()
				a := analytics.New(cfg, client, projectID)
				a.Track(
					fmt.Sprintf("Run %s", c.CommandPath()),
					analytics.Property("args", args), // NOTE: Safe right now, but might need allow-list in the future if some args end up containing sensitive info
					analytics.Property("elapsed_seconds", time.Since(start).Seconds()),
					analytics.FlagSet(c.Flags()),
					analytics.Error(runErr),
				)
			}()

			return originalRunE(c, args)
		}
	}

	// Recursively wrap all children
	for _, child := range cmd.Commands() {
		wrapCommands(child, app, skipUpdateCheck)
	}
}

// versionCheck starts a background check for a newer release and returns the
// function that prints the result. Deferring the returned function lets the
// network fetch overlap with the command's own work.
//
// The check is limited to interactive, non-CI terminals. `tiger version --check`
// runs its own synchronous check and `tiger upgrade` performs its own version
// comparison, so both are excluded to avoid a duplicate notice.
func versionCheck(cmd *cobra.Command, cfg *config.Config, skipUpdateCheck bool) func() {
	isVersionCheckCmd := cmd.Name() == "version" && cmd.Flag("check") != nil && cmd.Flag("check").Changed
	isUpgradeCmd := cmd.Name() == "upgrade"
	if !cfg.VersionCheck || skipUpdateCheck || isVersionCheckCmd || isUpgradeCmd ||
		util.IsCI() || !util.IsTerminal(cmd.ErrOrStderr()) {
		return func() {}
	}

	type checkResult struct {
		result *version.CheckResult
		err    error
	}
	resultCh := make(chan checkResult, 1)
	go func() {
		result, err := version.CheckForUpdate(cfg)
		resultCh <- checkResult{result: result, err: err}
	}()

	return func() {
		res := <-resultCh

		// Re-check cfg.VersionCheck: the command may have turned checks off in
		// place (e.g. `tiger config set version_check false`, which reloads the
		// config struct rather than replacing it).
		if !cfg.VersionCheck {
			return
		}

		if res.err != nil {
			cmd.PrintErrf("Warning: failed to check for updates: %v\n", res.err)
			return
		}

		output := cmd.ErrOrStderr()
		version.PrintUpdateWarning(res.result, cfg, &output)
	}
}

func Execute(ctx context.Context) error {
	rootCmd, err := buildRootCmd(ctx)
	if err != nil {
		return err
	}

	return rootCmd.Execute()
}
