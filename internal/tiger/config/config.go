package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	APIURL     string `mapstructure:"api_url" yaml:"api_url"`
	ConsoleURL string `mapstructure:"console_url" yaml:"console_url"`
	GatewayURL string `mapstructure:"gateway_url" yaml:"gateway_url"`
	ProjectID  string `mapstructure:"project_id" yaml:"project_id"`
	ServiceID  string `mapstructure:"service_id" yaml:"service_id"`
	Output     string `mapstructure:"output" yaml:"output"`
	Analytics  bool   `mapstructure:"analytics" yaml:"analytics"`
	ConfigDir  string `mapstructure:"config_dir" yaml:"-"`
	Debug      bool   `mapstructure:"debug" yaml:"debug"`
}

const (
	DefaultAPIURL     = "https://console.cloud.timescale.com/public/api/v1"
	DefaultConsoleURL = "https://console.cloud.timescale.com"
	DefaultGatewayURL = "https://console.cloud.timescale.com/api"
	DefaultOutput     = "table"
	DefaultAnalytics  = true
	DefaultDebug      = false
	DefaultConfigDir  = "~/.config/tiger"
	ConfigFileName    = "config.yaml"
)

// SetupViper configures the global Viper instance with defaults, env vars, and config file
func SetupViper(configFile string) error {
	viper.SetConfigFile(configFile)
	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

	// Set defaults for all config values
	viper.SetDefault("api_url", DefaultAPIURL)
	viper.SetDefault("console_url", DefaultConsoleURL)
	viper.SetDefault("gateway_url", DefaultGatewayURL)
	viper.SetDefault("project_id", "")
	viper.SetDefault("service_id", "")
	viper.SetDefault("output", DefaultOutput)
	viper.SetDefault("analytics", DefaultAnalytics)
	viper.SetDefault("debug", DefaultDebug)

	// Try to read config file if it exists
	if _, err := os.Stat(configFile); err == nil {
		// File exists, try to read it
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("error reading config file: %w", err)
		}
	}
	// If file doesn't exist, that's okay - we'll use defaults and env vars

	return nil
}

// Load creates a new Config instance from the current viper state
// This function should be called after SetupViper has been called to initialize viper
func Load() (*Config, error) {
	cfg := &Config{
		ConfigDir: GetConfigDir(),
	}

	// Use the global Viper instance that's already configured by initConfig() and bindFlags()
	// This gives us proper precedence: CLI flags > env vars > config file > defaults
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Save() error {
	configDir := c.ConfigDir
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	configFile := filepath.Join(configDir, ConfigFileName)

	viper.Set("api_url", c.APIURL)
	viper.Set("console_url", c.ConsoleURL)
	viper.Set("gateway_url", c.GatewayURL)
	viper.Set("project_id", c.ProjectID)
	viper.Set("service_id", c.ServiceID)
	viper.Set("output", c.Output)
	viper.Set("analytics", c.Analytics)
	viper.Set("debug", c.Debug)

	if err := viper.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}

func (c *Config) Set(key, value string) error {
	switch key {
	case "api_url":
		c.APIURL = value
	case "console_url":
		c.ConsoleURL = value
	case "gateway_url":
		c.GatewayURL = value
	case "project_id":
		c.ProjectID = value
	case "service_id":
		c.ServiceID = value
	case "output":
		if value != "json" && value != "yaml" && value != "table" {
			return fmt.Errorf("invalid output format: %s (must be json, yaml, or table)", value)
		}
		c.Output = value
	case "analytics":
		if value == "true" {
			c.Analytics = true
		} else if value == "false" {
			c.Analytics = false
		} else {
			return fmt.Errorf("invalid analytics value: %s (must be true or false)", value)
		}
	case "debug":
		if value == "true" {
			c.Debug = true
		} else if value == "false" {
			c.Debug = false
		} else {
			return fmt.Errorf("invalid debug value: %s (must be true or false)", value)
		}
	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	return c.Save()
}

func (c *Config) Unset(key string) error {
	switch key {
	case "api_url":
		c.APIURL = DefaultAPIURL
	case "console_url":
		c.ConsoleURL = DefaultConsoleURL
	case "gateway_url":
		c.GatewayURL = DefaultGatewayURL
	case "project_id":
		c.ProjectID = ""
	case "service_id":
		c.ServiceID = ""
	case "output":
		c.Output = DefaultOutput
	case "analytics":
		c.Analytics = DefaultAnalytics
	case "debug":
		c.Debug = DefaultDebug
	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	return c.Save()
}

func (c *Config) Reset() error {
	c.APIURL = DefaultAPIURL
	c.ConsoleURL = DefaultConsoleURL
	c.GatewayURL = DefaultGatewayURL
	c.ProjectID = ""
	c.ServiceID = ""
	c.Output = DefaultOutput
	c.Analytics = DefaultAnalytics
	c.Debug = DefaultDebug

	return c.Save()
}

func GetConfigDir() string {
	if dir := os.Getenv("TIGER_CONFIG_DIR"); dir != "" {
		return expandPath(dir)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./.config/tiger"
	}

	return filepath.Join(homeDir, ".config", "tiger")
}

func expandPath(path string) string {
	if path == "~" {
		homeDir, _ := os.UserHomeDir()
		return homeDir
	}

	if len(path) > 1 && path[:2] == "~/" {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, path[2:])
	}

	return path
}

// ResetGlobalConfig clears the global viper state for testing
// This is mainly used to reset viper configuration between test runs
func ResetGlobalConfig() {
	viper.Reset()
}
