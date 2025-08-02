package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/tigerdata/tiger-cli/internal/tiger/config"
	"github.com/tigerdata/tiger-cli/internal/tiger/logging"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
	Long:  `Manage CLI configuration settings stored in ~/.config/tiger/config.yaml`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current CLI configuration settings`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		switch cfg.Output {
		case "json":
			return outputJSON(cfg)
		case "yaml":
			return outputYAML(cfg)
		default:
			return outputTable(cfg)
		}
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set configuration value",
	Long:  `Set a configuration value and save it to ~/.config/tiger/config.yaml`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if err := cfg.Set(key, value); err != nil {
			return fmt.Errorf("failed to set config: %w", err)
		}

		logging.Info("Configuration updated", zap.String("key", key), zap.String("value", value))
		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset <key>",
	Short: "Remove configuration value",
	Long:  `Remove a configuration value and save changes to ~/.config/tiger/config.yaml`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if err := cfg.Unset(key); err != nil {
			return fmt.Errorf("failed to unset config: %w", err)
		}

		logging.Info("Configuration updated", zap.String("key", key))
		fmt.Printf("Unset %s\n", key)
		return nil
	},
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset to defaults",
	Long:  `Reset all configuration settings to their default values`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if err := cfg.Reset(); err != nil {
			return fmt.Errorf("failed to reset config: %w", err)
		}

		logging.Info("Configuration reset to defaults")
		fmt.Println("Configuration reset to defaults")
		return nil
	},
}

func outputTable(cfg *config.Config) error {
	fmt.Println("Current Configuration:")
	fmt.Printf("  API URL:     %s\n", cfg.APIURL)
	fmt.Printf("  Project ID:  %s\n", valueOrEmpty(cfg.ProjectID))
	fmt.Printf("  Service ID:  %s\n", valueOrEmpty(cfg.ServiceID))
	fmt.Printf("  Output:      %s\n", cfg.Output)
	fmt.Printf("  Analytics:   %t\n", cfg.Analytics)
	fmt.Printf("  Config Dir:  %s\n", cfg.ConfigDir)
	return nil
}

func outputJSON(cfg *config.Config) error {
	data := map[string]interface{}{
		"api_url":    cfg.APIURL,
		"project_id": cfg.ProjectID,
		"service_id": cfg.ServiceID,
		"output":     cfg.Output,
		"analytics":  cfg.Analytics,
		"config_dir": cfg.ConfigDir,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func outputYAML(cfg *config.Config) error {
	data := map[string]interface{}{
		"api_url":    cfg.APIURL,
		"project_id": cfg.ProjectID,
		"service_id": cfg.ServiceID,
		"output":     cfg.Output,
		"analytics":  cfg.Analytics,
		"config_dir": cfg.ConfigDir,
	}

	encoder := yaml.NewEncoder(os.Stdout)
	defer encoder.Close()
	return encoder.Encode(data)
}

func valueOrEmpty(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configUnsetCmd)
	configCmd.AddCommand(configResetCmd)
	rootCmd.AddCommand(configCmd)
}