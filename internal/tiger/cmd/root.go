package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/analytics"
	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/version"
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

	// Bind flags to viper
	err := errors.Join(
		viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug")),
		viper.BindPFlag("service_id", cmd.PersistentFlags().Lookup("service-id")),
		viper.BindPFlag("analytics", cmd.PersistentFlags().Lookup("analytics")),
		viper.BindPFlag("password_storage", cmd.PersistentFlags().Lookup("password-storage")),
		viper.BindPFlag("color", cmd.PersistentFlags().Lookup("color")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to bind flags: %w", err)
	}

	// Note: api_url is intentionally not exposed as a CLI flag.
	// It can be configured via:
	// - Environment variable: TIGER_API_URL
	// - Config file: ~/.config/tiger/config.yaml
	// - Config command: tiger config set api_url <url>
	// This is primarily used for internal debugging and development.

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
		cmd.RunE = func(c *cobra.Command, args []string) (err error) {
			start := time.Now()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			a := analytics.TryInit(cfg)
			defer func() {
				a.Track(fmt.Sprintf("Run %s", c.CommandPath()),
					analytics.Property("args", args), // NOTE: Safe right now, but might need allow-list in the future if some args end up containing sensitive info
					analytics.Property("elapsed_seconds", time.Since(start).Seconds()),
					analytics.FlagSet(c.Flags()),
					analytics.Error(err),
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

func readString(ctx context.Context, readFn func() (string, error)) (string, error) {
	valCh := make(chan string)
	errCh := make(chan error)
	defer func() { close(valCh); close(errCh) }()
	go func() {
		val, err := readFn()
		if err != nil {
			errCh <- err
			return
		}
		select {
		case <-ctx.Done(): // don't return an empty value if the context is already canceled
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
