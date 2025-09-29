package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/tiger/config"
	"github.com/timescale/tiger-cli/internal/tiger/logging"
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
	fmt.Fprintf(out, "  api_url:          %s\n", cfg.APIURL)
	fmt.Fprintf(out, "  console_url:      %s\n", cfg.ConsoleURL)
	fmt.Fprintf(out, "  gateway_url:      %s\n", cfg.GatewayURL)
	fmt.Fprintf(out, "  docs_mcp:         %t\n", cfg.DocsMCP)
	fmt.Fprintf(out, "  docs_mcp_url:     %s\n", cfg.DocsMCPURL)
	fmt.Fprintf(out, "  project_id:       %s\n", valueOrEmpty(cfg.ProjectID))
	fmt.Fprintf(out, "  service_id:       %s\n", valueOrEmpty(cfg.ServiceID))
	fmt.Fprintf(out, "  output:           %s\n", cfg.Output)
	fmt.Fprintf(out, "  analytics:        %t\n", cfg.Analytics)
	fmt.Fprintf(out, "  password_storage: %s\n", cfg.PasswordStorage)
	fmt.Fprintf(out, "  debug:            %t\n", cfg.Debug)
	fmt.Fprintf(out, "  config_dir:       %s\n", cfg.ConfigDir)
	return nil
}

func outputJSON(cfg *config.Config, cmd *cobra.Command) error {
	data := map[string]interface{}{
		"api_url":          cfg.APIURL,
		"console_url":      cfg.ConsoleURL,
		"gateway_url":      cfg.GatewayURL,
		"docs_mcp":         cfg.DocsMCP,
		"docs_mcp_url":     cfg.DocsMCPURL,
		"project_id":       cfg.ProjectID,
		"service_id":       cfg.ServiceID,
		"output":           cfg.Output,
		"analytics":        cfg.Analytics,
		"password_storage": cfg.PasswordStorage,
		"debug":            cfg.Debug,
		"config_dir":       cfg.ConfigDir,
	}

	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func outputYAML(cfg *config.Config, cmd *cobra.Command) error {
	data := map[string]interface{}{
		"api_url":          cfg.APIURL,
		"console_url":      cfg.ConsoleURL,
		"gateway_url":      cfg.GatewayURL,
		"docs_mcp":         cfg.DocsMCP,
		"docs_mcp_url":     cfg.DocsMCPURL,
		"project_id":       cfg.ProjectID,
		"service_id":       cfg.ServiceID,
		"output":           cfg.Output,
		"analytics":        cfg.Analytics,
		"password_storage": cfg.PasswordStorage,
		"debug":            cfg.Debug,
		"config_dir":       cfg.ConfigDir,
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
