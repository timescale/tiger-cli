package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/tigerdata/tiger-cli/internal/tiger/config"
	"github.com/tigerdata/tiger-cli/internal/tiger/logging"
)

func buildRootCmd() *cobra.Command {
	var cfgFile string
	var debug bool
	var output string
	var apiKey string
	var projectID string
	var serviceID string
	var analytics bool

	cmd := &cobra.Command{
		Use:   "tiger",
		Short: "Tiger CLI - TigerData Cloud Platform command-line interface",
		Long: `Tiger CLI is a command-line interface for managing TigerData Cloud Platform resources.
Built as a single Go binary, it provides comprehensive tools for managing database services,
VPCs, replicas, and related infrastructure components.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := logging.Init(debug); err != nil {
				return fmt.Errorf("failed to initialize logging: %w", err)
			}

			cfg, err := config.Load()
			if err != nil {
				logging.Error("failed to load config", zap.Error(err))
				return err
			}

			logging.Debug("CLI initialized",
				zap.String("config_dir", cfg.ConfigDir),
				zap.String("output", cfg.Output),
				zap.Bool("debug", cfg.Debug),
			)

			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			logging.Sync()
		},
	}

	// Setup configuration initialization
	initConfigFunc := func() {
		// cfgFile now always has a value (either default or user-specified)
		if err := config.SetupViper(cfgFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting up config: %v\n", err)
			os.Exit(1)
		}
		
		if debug {
			if configFile := viper.ConfigFileUsed(); configFile != "" {
				fmt.Fprintln(os.Stderr, "Using config file:", configFile)
			}
		}
	}

	cobra.OnInitialize(initConfigFunc)

	// Add persistent flags
	defaultConfigFile := filepath.Join(config.GetConfigDir(), config.ConfigFileName)
	cmd.PersistentFlags().StringVar(&cfgFile, "config", defaultConfigFile, "config file")
	cmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	cmd.PersistentFlags().StringVarP(&output, "output", "o", "", "output format (json, yaml, table)")
	cmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "TigerData API key")
	cmd.PersistentFlags().StringVar(&projectID, "project-id", "", "project ID")
	cmd.PersistentFlags().StringVar(&serviceID, "service-id", "", "service ID")
	cmd.PersistentFlags().BoolVar(&analytics, "analytics", true, "enable/disable usage analytics")

	// Bind flags to viper
	viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))
	viper.BindPFlag("output", cmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("api_key", cmd.PersistentFlags().Lookup("api-key"))
	viper.BindPFlag("project_id", cmd.PersistentFlags().Lookup("project-id"))
	viper.BindPFlag("service_id", cmd.PersistentFlags().Lookup("service-id"))
	viper.BindPFlag("analytics", cmd.PersistentFlags().Lookup("analytics"))

	// Add all subcommands
	cmd.AddCommand(buildVersionCmd())
	cmd.AddCommand(buildConfigCmd())
	cmd.AddCommand(buildAuthCmd())
	cmd.AddCommand(buildServiceCmd())
	cmd.AddCommand(buildDbCmd())

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

