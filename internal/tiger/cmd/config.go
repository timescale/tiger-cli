package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
)

func buildConfigShowCmd() *cobra.Command {
	var output string
	var noDefaults bool
	var withEnv bool

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Long:  `Display the current CLI configuration settings`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Use flag value if provided, otherwise use config value
			outputFormat := cfg.Output
			if cmd.Flags().Changed("output") {
				outputFormat = output
			}

			configFile, err := cfg.EnsureConfigDir()
			if err != nil {
				return err
			}

			// a new viper, free from env and cli flags
			v := viper.New()
			v.SetConfigFile(configFile)
			if withEnv {
				config.ApplyEnvOverrides(v)
			}
			if !noDefaults {
				config.ApplyDefaults(v)
			}
			if err := config.ReadInConfig(v); err != nil {
				return err
			}

			cfgOut, err := config.ForOutputFromViper(v)
			if err != nil {
				return err
			}

			if *cfgOut.ConfigDir == config.GetDefaultConfigDir() {
				cfgOut.ConfigDir = nil
			}

			switch outputFormat {
			case "json":
				return outputJSON(cfgOut, cmd)
			case "yaml":
				return outputYAML(cfgOut, cmd)
			default:
				return outputTable(cfgOut, cmd)
			}
		},
	}

	cmd.Flags().VarP((*outputFlag)(&output), "output", "o", "output format (json, yaml, table)")
	cmd.Flags().BoolVar(&noDefaults, "no-defaults", false, "do not show default values for unset fields")
	cmd.Flags().BoolVar(&withEnv, "with-env", false, "apply environment variable overrides")

	return cmd
}

func buildConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set configuration value",
		Long:  `Set a configuration value and save it to ~/.config/tiger/config.yaml`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]

			cmd.SilenceUsage = true

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := cfg.Set(key, value); err != nil {
				return fmt.Errorf("failed to set config: %w", err)
			}

			logging.Info("Configuration updated", zap.String("key", key), zap.String("value", value))
			fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, value)
			return nil
		},
	}
}

func buildConfigUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove configuration value",
		Long:  `Remove a configuration value and save changes to ~/.config/tiger/config.yaml`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			cmd.SilenceUsage = true

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := cfg.Unset(key); err != nil {
				return fmt.Errorf("failed to unset config: %w", err)
			}

			logging.Info("Configuration updated", zap.String("key", key))
			fmt.Fprintf(cmd.OutOrStdout(), "Unset %s\n", key)
			return nil
		},
	}
}

func buildConfigResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset to defaults",
		Long:  `Reset all configuration settings to their default values`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := cfg.Reset(); err != nil {
				return fmt.Errorf("failed to reset config: %w", err)
			}

			logging.Info("Configuration reset to defaults")
			fmt.Fprintln(cmd.OutOrStdout(), "Configuration reset to defaults")
			return nil
		},
	}
}

func buildConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
		Long:  `Manage CLI configuration settings stored in ~/.config/tiger/config.yaml`,
	}

	cmd.AddCommand(buildConfigShowCmd())
	cmd.AddCommand(buildConfigSetCmd())
	cmd.AddCommand(buildConfigUnsetCmd())
	cmd.AddCommand(buildConfigResetCmd())

	return cmd
}

func outputTable(cfg *config.ConfigOutput, cmd *cobra.Command) error {
	table := tablewriter.NewWriter(cmd.OutOrStdout())
	table.Header("PROPERTY", "VALUE")
	if cfg.APIURL != nil {
		table.Append("api_url", *cfg.APIURL)
	}
	if cfg.ConsoleURL != nil {
		table.Append("console_url", *cfg.ConsoleURL)
	}
	if cfg.GatewayURL != nil {
		table.Append("gateway_url", *cfg.GatewayURL)
	}
	if cfg.DocsMCP != nil {
		table.Append("docs_mcp", fmt.Sprintf("%t", *cfg.DocsMCP))
	}
	if cfg.DocsMCPURL != nil {
		table.Append("docs_mcp_url", *cfg.DocsMCPURL)
	}
	if cfg.ProjectID != nil {
		table.Append("project_id", *cfg.ProjectID)
	}
	if cfg.ServiceID != nil {
		table.Append("service_id", *cfg.ServiceID)
	}
	if cfg.Output != nil {
		table.Append("output", *cfg.Output)
	}
	if cfg.Analytics != nil {
		table.Append("analytics", fmt.Sprintf("%t", *cfg.Analytics))
	}
	if cfg.PasswordStorage != nil {
		table.Append("password_storage", *cfg.PasswordStorage)
	}
	if cfg.Debug != nil {
		table.Append("debug", fmt.Sprintf("%t", *cfg.Debug))
	}
	if cfg.ConfigDir != nil {
		table.Append("config_dir", *cfg.ConfigDir)
	}
	if cfg.VersionCheckURL != nil {
		table.Append("version_check_url", *cfg.VersionCheckURL)
	}
	if cfg.VersionCheckInterval != nil {
		table.Append("version_check_interval", fmt.Sprintf("%d seconds", *cfg.VersionCheckInterval))
	}
	if cfg.VersionCheckLastTime != nil {
		table.Append("version_check_last_time", time.Unix(int64(*cfg.VersionCheckLastTime), 0).Format(time.RFC1123))
	}
	return table.Render()
}

func outputJSON(cfg *config.ConfigOutput, cmd *cobra.Command) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(cfg)
}

func outputYAML(cfg *config.ConfigOutput, cmd *cobra.Command) error {
	encoder := yaml.NewEncoder(cmd.OutOrStdout())
	defer encoder.Close()
	return encoder.Encode(cfg)
}
