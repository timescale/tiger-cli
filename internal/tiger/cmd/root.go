package cmd

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/version"
)

func buildRootCmd() *cobra.Command {
	var configDir string
	var debug bool
	var projectID string
	var serviceID string
	var analytics bool
	var passwordStorage string
	var skipUpdateCheck bool
	var colorFlag bool

	cmd := &cobra.Command{
		Use:   "tiger",
		Short: "Tiger CLI - TigerData Cloud Platform command-line interface",
		Long: `Tiger CLI is a command-line interface for managing TigerData Cloud Platform resources.
Built as a single Go binary, it provides comprehensive tools for managing database services,
VPCs, replicas, and related infrastructure components.

To get started, run:

tiger auth login

`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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

	// Setup configuration initialization
	initConfigFunc := func() {
		configDirFlag := cmd.PersistentFlags().Lookup("config-dir")
		if err := config.SetupViper(config.GetEffectiveConfigDir(configDirFlag)); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting up config: %v\n", err)
			os.Exit(1)
		}
	}

	cobra.OnInitialize(initConfigFunc)

	// Add persistent flags
	cmd.PersistentFlags().StringVar(&configDir, "config-dir", config.GetDefaultConfigDir(), "config directory")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	cmd.PersistentFlags().StringVar(&projectID, "project-id", "", "project ID")
	cmd.PersistentFlags().StringVar(&serviceID, "service-id", "", "service ID")
	cmd.PersistentFlags().BoolVar(&analytics, "analytics", true, "enable/disable usage analytics")
	cmd.PersistentFlags().StringVar(&passwordStorage, "password-storage", config.DefaultPasswordStorage, "password storage method (keyring, pgpass, none)")
	cmd.PersistentFlags().BoolVar(&skipUpdateCheck, "skip-update-check", false, "skip checking for updates on startup")
	cmd.PersistentFlags().BoolVar(&colorFlag, "color", true, "enable colored output")

	// Bind flags to viper
	viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))
	viper.BindPFlag("project_id", cmd.PersistentFlags().Lookup("project-id"))
	viper.BindPFlag("service_id", cmd.PersistentFlags().Lookup("service-id"))
	viper.BindPFlag("analytics", cmd.PersistentFlags().Lookup("analytics"))
	viper.BindPFlag("password_storage", cmd.PersistentFlags().Lookup("password-storage"))
	viper.BindPFlag("color", cmd.PersistentFlags().Lookup("color"))

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

	return cmd
}

func Execute() {
	rootCmd := buildRootCmd()
	err := rootCmd.Execute()
	if err != nil {
		// Check if it's a custom exit code error
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}
