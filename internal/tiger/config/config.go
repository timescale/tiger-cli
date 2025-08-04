package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	APIURL     string `mapstructure:"api_url" yaml:"api_url"`
	ProjectID  string `mapstructure:"project_id" yaml:"project_id"`
	ServiceID  string `mapstructure:"service_id" yaml:"service_id"`
	Output     string `mapstructure:"output" yaml:"output"`
	Analytics  bool   `mapstructure:"analytics" yaml:"analytics"`
	ConfigDir  string `mapstructure:"config_dir" yaml:"-"`
	Debug      bool   `mapstructure:"debug" yaml:"-"`
}

const (
	DefaultAPIURL     = "https://api.tigerdata.com/public/v1"
	DefaultOutput     = "table"
	DefaultAnalytics  = true
	DefaultConfigDir  = "~/.config/tiger"
	ConfigFileName    = "config.yaml"
)

var globalConfig *Config

func Load() (*Config, error) {
	if globalConfig != nil {
		return globalConfig, nil
	}

	cfg := &Config{
		APIURL:    DefaultAPIURL,
		Output:    DefaultOutput,
		Analytics: DefaultAnalytics,
	}

	configDir := GetConfigDir()
	cfg.ConfigDir = configDir

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configDir)

	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

	viper.SetDefault("api_url", DefaultAPIURL)
	viper.SetDefault("project_id", "")
	viper.SetDefault("service_id", "")
	viper.SetDefault("output", DefaultOutput)
	viper.SetDefault("analytics", DefaultAnalytics)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	globalConfig = cfg
	return cfg, nil
}

func (c *Config) Save() error {
	configDir := c.ConfigDir
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	configFile := filepath.Join(configDir, ConfigFileName)
	
	viper.Set("api_url", c.APIURL)
	viper.Set("project_id", c.ProjectID)
	viper.Set("service_id", c.ServiceID)
	viper.Set("output", c.Output)
	viper.Set("analytics", c.Analytics)

	if err := viper.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}

func (c *Config) Set(key, value string) error {
	switch key {
	case "api_url":
		c.APIURL = value
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
	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	return c.Save()
}

func (c *Config) Unset(key string) error {
	switch key {
	case "api_url":
		c.APIURL = DefaultAPIURL
	case "project_id":
		c.ProjectID = ""
	case "service_id":
		c.ServiceID = ""
	case "output":
		c.Output = DefaultOutput
	case "analytics":
		c.Analytics = DefaultAnalytics
	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	return c.Save()
}

func (c *Config) Reset() error {
	c.APIURL = DefaultAPIURL
	c.ProjectID = ""
	c.ServiceID = ""
	c.Output = DefaultOutput
	c.Analytics = DefaultAnalytics

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


// ResetGlobalConfig clears the global config singleton for testing
func ResetGlobalConfig() {
	globalConfig = nil
}