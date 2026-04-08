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

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/timescale/tiger-cli/internal/tiger/analytics"
	"github.com/timescale/tiger-cli/internal/tiger/common"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/version"
	"go.uber.org/zap"
	"golang.org/x/term"
)

func buildRootCmd(ctx context.Context) (*cobra.Command, error) {
	var configDir string
	var debug bool
	var serviceID string
	var analytics bool
	var passwordStorage string
	var skipUpdateCheck bool
	var colorFlag bool

	cmd := &cobra.Command{
		Use:   "tiger",
		Short: "Tiger CLI - Tiger Cloud Platform command-line interface",
		Long: `Tiger CLI is a command-line interface for managing Tiger Cloud platform resources.
Built as a single Go binary, it provides comprehensive tools for managing database services,
VPCs, replicas, and related infrastructure components.

To get started, run:

tiger auth login

`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cmd.SetContext(ctx)

			// Bind persistent flags to viper
			// Use cmd.Flags() which includes inherited persistent flags from parents
			if err := errors.Join(
				viper.BindPFlag("debug", cmd.Flags().Lookup("debug")),
				viper.BindPFlag("service_id", cmd.Flags().Lookup("service-id")),
				viper.BindPFlag("analytics", cmd.Flags().Lookup("analytics")),
				viper.BindPFlag("password_storage", cmd.Flags().Lookup("password-storage")),
				viper.BindPFlag("color", cmd.Flags().Lookup("color")),
			); err != nil {
				return fmt.Errorf("failed to bind flags: %w", err)
			}

			// Setup configuration initialization
			configDirFlag := cmd.Flags().Lookup("config-dir")
			if err := config.SetupViper(config.GetEffectiveConfigDir(configDirFlag)); err != nil {
				return fmt.Errorf("error setting up config: %w", err)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := logging.Init(cfg.Debug); err != nil {
				return fmt.Errorf("failed to initialize logging: %w", err)
			}

			logging.Debug("CLI initialized",
				zap.String("config_dir", cfg.ConfigDir),
				zap.String("output", cfg.Output),
				zap.Bool("debug", cfg.Debug),
			)

			if !cfg.Color {
				color.NoColor = true
			}

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Skip update check if:
			// 1. --skip-update-check flag was provided
			// 2. Running "version --check" (version command handles its own check)
			isVersionCheck := cmd.Name() == "version" && cmd.Flag("check").Changed
			if !skipUpdateCheck && !isVersionCheck {
				output := cmd.ErrOrStderr()
				result := version.PerformCheck(cfg, &output, false)
				version.PrintUpdateWarning(result, cfg, &output)
			}

			logging.Sync()
			return nil
		},
	}

	// Add persistent flags
	cmd.PersistentFlags().StringVar(&configDir, "config-dir", config.GetDefaultConfigDir(), "config directory")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	cmd.PersistentFlags().StringVar(&serviceID, "service-id", "", "service ID")
	cmd.PersistentFlags().BoolVar(&analytics, "analytics", true, "enable/disable usage analytics")
	cmd.PersistentFlags().StringVar(&passwordStorage, "password-storage", config.DefaultPasswordStorage, "password storage method (keyring, pgpass, none)")
	cmd.PersistentFlags().BoolVar(&skipUpdateCheck, "skip-update-check", false, "skip checking for updates on startup")
	cmd.PersistentFlags().BoolVar(&colorFlag, "color", true, "enable colored output")

	// Add all subcommands
	cmd.AddCommand(buildVersionCmd())
	cmd.AddCommand(buildConfigCmd())
	cmd.AddCommand(buildAuthCmd())
	cmd.AddCommand(buildServiceCmd())
	cmd.AddCommand(buildDbCmd())
	cmd.AddCommand(buildMCPCmd())

	wrapCommandsWithAnalytics(cmd)

	return cmd, nil
}

func wrapCommandsWithAnalytics(cmd *cobra.Command) {
	// Wrap this command's RunE if it exists
	if cmd.RunE != nil {
		originalRunE := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) (runErr error) {
			start := time.Now()

			defer func() {
				// Reload config after command to account for config changes
				// during command (e.g. `tiger config set analytics false`
				// should not result in an analytics event being sent).
				cfg, err := config.Load()
				if err != nil {
					return
				}

				// Reload credentials after command to account for credentials
				// changes during command (e.g. `tiger auth login` should
				// record an analytics event).
				client, projectID, _ := common.NewAPIClient(cmd.Context(), cfg)
				a := analytics.New(cfg, client, projectID)
				a.Track(fmt.Sprintf("Run %s", c.CommandPath()),
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
		wrapCommandsWithAnalytics(child)
	}
}

func Execute(ctx context.Context) error {
	rootCmd, err := buildRootCmd(ctx)
	if err != nil {
		return err
	}

	return rootCmd.Execute()
}

// readLine reads a line of text from the input, cancellable via context.
func readLine(ctx context.Context, in io.Reader) (string, error) {
	valCh := make(chan string)
	errCh := make(chan error)
	defer func() { close(valCh); close(errCh) }()
	go func() {
		val, err := bufio.NewReader(in).ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		valCh <- val
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case val := <-valCh:
		return strings.TrimSpace(val), nil
	}
}

// readPassword reads a password with hidden input, cancellable via context.
// This is a package-level var so it can be overridden for testing.
var readPassword = func(ctx context.Context, in io.Reader) (string, error) {
	f, ok := in.(*os.File)
	if !ok {
		return "", fmt.Errorf("password input requires a terminal file descriptor")
	}

	valCh := make(chan string)
	errCh := make(chan error)
	defer func() { close(valCh); close(errCh) }()
	go func() {
		val, err := term.ReadPassword(int(f.Fd()))
		if err != nil {
			errCh <- err
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		valCh <- string(val)
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case val := <-valCh:
		return strings.TrimSpace(val), nil
	}
}
