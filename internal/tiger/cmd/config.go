package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/tigerdata/tiger-cli/internal/tiger/config"
	"github.com/tigerdata/tiger-cli/internal/tiger/logging"
)

func buildConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Long:  `Display the current CLI configuration settings`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			switch cfg.Output {
			case "json":
				return outputJSON(cfg, cmd)
			case "yaml":
				return outputYAML(cfg, cmd)
			default:
				return outputTable(cfg, cmd)
			}
		},
	}
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

func outputTable(cfg *config.Config, cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Current Configuration:")
	fmt.Fprintf(out, "  API URL:     %s\n", cfg.APIURL)
	fmt.Fprintf(out, "  Project ID:  %s\n", valueOrEmpty(cfg.ProjectID))
	fmt.Fprintf(out, "  Service ID:  %s\n", valueOrEmpty(cfg.ServiceID))
	fmt.Fprintf(out, "  Output:      %s\n", cfg.Output)
	fmt.Fprintf(out, "  Analytics:   %t\n", cfg.Analytics)
	fmt.Fprintf(out, "  Debug:       %t\n", cfg.Debug)
	fmt.Fprintf(out, "  Config Dir:  %s\n", cfg.ConfigDir)
	return nil
}

func outputJSON(cfg *config.Config, cmd *cobra.Command) error {
	data := map[string]interface{}{
		"api_url":    cfg.APIURL,
		"project_id": cfg.ProjectID,
		"service_id": cfg.ServiceID,
		"output":     cfg.Output,
		"analytics":  cfg.Analytics,
		"debug":      cfg.Debug,
		"config_dir": cfg.ConfigDir,
	}

	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func outputYAML(cfg *config.Config, cmd *cobra.Command) error {
	data := map[string]interface{}{
		"api_url":    cfg.APIURL,
		"project_id": cfg.ProjectID,
		"service_id": cfg.ServiceID,
		"output":     cfg.Output,
		"analytics":  cfg.Analytics,
		"debug":      cfg.Debug,
		"config_dir": cfg.ConfigDir,
	}

	encoder := yaml.NewEncoder(cmd.OutOrStdout())
	defer encoder.Close()
	return encoder.Encode(data)
}

func valueOrEmpty(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}
