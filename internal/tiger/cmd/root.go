package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/tigerdata/tiger-cli/internal/tiger/config"
	"github.com/tigerdata/tiger-cli/internal/tiger/logging"
)

var (
	cfgFile   string
	debug     bool
	output    string
	apiKey    string
	projectID string
	serviceID string
	analytics bool
)

var rootCmd = &cobra.Command{
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

		if output != "" {
			cfg.Output = output
		}
		if apiKey != "" {
			cfg.APIURL = apiKey
		}
		if projectID != "" {
			cfg.ProjectID = projectID
		}
		if serviceID != "" {
			cfg.ServiceID = serviceID
		}
		if cmd.Flags().Changed("analytics") {
			cfg.Analytics = analytics
		}
		if debug {
			cfg.Debug = debug
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

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ~/.config/tiger/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "", "output format (json, yaml, table)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "TigerData API key")
	rootCmd.PersistentFlags().StringVar(&projectID, "project-id", "", "project ID")
	rootCmd.PersistentFlags().StringVar(&serviceID, "service-id", "", "service ID")
	rootCmd.PersistentFlags().BoolVar(&analytics, "analytics", true, "enable/disable usage analytics")

	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("api_key", rootCmd.PersistentFlags().Lookup("api-key"))
	viper.BindPFlag("project_id", rootCmd.PersistentFlags().Lookup("project-id"))
	viper.BindPFlag("service_id", rootCmd.PersistentFlags().Lookup("service-id"))
	viper.BindPFlag("analytics", rootCmd.PersistentFlags().Lookup("analytics"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		cfg := config.Get()
		viper.AddConfigPath(cfg.ConfigDir)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		if debug {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}