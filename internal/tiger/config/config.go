package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/timescale/tiger-cli/internal/tiger/util"
)

type Config struct {
	APIURL          string `mapstructure:"api_url" yaml:"api_url"`
	ConsoleURL      string `mapstructure:"console_url" yaml:"console_url"`
	GatewayURL      string `mapstructure:"gateway_url" yaml:"gateway_url"`
	DocsMCP         bool   `mapstructure:"docs_mcp" yaml:"docs_mcp"`
	DocsMCPURL      string `mapstructure:"docs_mcp_url" yaml:"docs_mcp_url"`
	ProjectID       string `mapstructure:"project_id" yaml:"project_id"`
	ServiceID       string `mapstructure:"service_id" yaml:"service_id"`
	Output          string `mapstructure:"output" yaml:"output"`
	Analytics       bool   `mapstructure:"analytics" yaml:"analytics"`
	PasswordStorage string `mapstructure:"password_storage" yaml:"password_storage"`
	Debug           bool   `mapstructure:"debug" yaml:"debug"`
	ConfigDir       string `mapstructure:"config_dir" yaml:"-"`
}

const (
	DefaultAPIURL          = "https://console.cloud.timescale.com/public/api/v1"
	DefaultConsoleURL      = "https://console.cloud.timescale.com"
	DefaultGatewayURL      = "https://console.cloud.timescale.com/api"
	DefaultDocsMCP         = true
	DefaultDocsMCPURL      = "https://mcp.tigerdata.com/docs"
	DefaultOutput          = "table"
	DefaultAnalytics       = true
	DefaultPasswordStorage = "keyring"
	DefaultDebug           = false
	ConfigFileName         = "config.yaml"
)

var defaultValues = map[string]any{
	"api_url":          DefaultAPIURL,
	"console_url":      DefaultConsoleURL,
	"gateway_url":      DefaultGatewayURL,
	"docs_mcp":         DefaultDocsMCP,
	"docs_mcp_url":     DefaultDocsMCPURL,
	"project_id":       "",
	"service_id":       "",
	"output":           DefaultOutput,
	"analytics":        DefaultAnalytics,
	"password_storage": DefaultPasswordStorage,
	"debug":            DefaultDebug,
}

// SetupViper configures the global Viper instance with defaults, env vars, and config file
func SetupViper(configDir string) error {
	// Configure viper to read from config file
	configFile := GetConfigFile(configDir)
	viper.SetConfigFile(configFile)

	// Configure viper to read from env vars
	viper.SetEnvPrefix("TIGER")
	viper.AutomaticEnv()

	// Set defaults for all config values
	for key, value := range defaultValues {
		viper.SetDefault(key, value)
	}

	return readInConfig()
}

func readInConfig() error {
	// Try to read config file if it exists
	// If file doesn't exist, that's okay - we'll use defaults and env vars
	if err := viper.ReadInConfig(); err != nil &&
		!errors.As(err, &viper.ConfigFileNotFoundError{}) &&
		!errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Load creates a new Config instance from the current viper state
// This function should be called after SetupViper has been called to initialize viper
func Load() (*Config, error) {
	// Try to read config file into viper to ensure we're unmarshaling the most
	// up-to-date values into the config struct.
	if err := readInConfig(); err != nil {
		return nil, err
	}

	cfg := &Config{
		ConfigDir: GetConfigDir(),
	}

	// Use the global Viper instance that's already configured by SetupViper
	// This gives us proper precedence: CLI flags > env vars > config file > defaults
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return cfg, nil
}

func ensureConfigDir(configDir string) (string, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("error creating config directory: %w", err)
	}
	return GetConfigFile(configDir), nil
}

func (c *Config) ensureConfigDir() (string, error) {
	return ensureConfigDir(c.ConfigDir)
}

// UseTestConfig writes only the specified key-value pairs to the config file and
// returns a Config instance with those values set.
// This function is intended for testing purposes only, where you need to set up
// specific config file state without writing default values for unspecified keys.
func UseTestConfig(configDir string, values map[string]any) (*Config, error) {
	configFile, err := ensureConfigDir(configDir)
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigFile(configFile)

	// Write only the specified key-value pairs
	for key, value := range values {
		v.Set(key, value)
	}

	if err := v.WriteConfigAs(configFile); err != nil {
		return nil, fmt.Errorf("error writing config file: %w", err)
	}

	viper.Reset()
	if err := SetupViper(configDir); err != nil {
		return nil, err
	}

	// Construct and return a Config instance with the values
	cfg := &Config{
		ConfigDir: configDir,
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Set(key, value string) error {
	var validated any
	switch key {
	case "api_url":
		c.APIURL = value
		validated = value
	case "console_url":
		c.ConsoleURL = value
		validated = value
	case "gateway_url":
		c.GatewayURL = value
		validated = value
	case "docs_mcp":
		b, err := setBool("docs_mcp", value)
		if err != nil {
			return err
		}
		c.DocsMCP = b
		validated = b
	case "docs_mcp_url":
		c.DocsMCPURL = value
		validated = value
	case "project_id":
		c.ProjectID = value
		validated = value
	case "service_id":
		c.ServiceID = value
		validated = value
	case "output":
		if err := ValidateOutputFormat(value); err != nil {
			return err
		}
		c.Output = value
		validated = value
	case "analytics":
		b, err := setBool("analytics", value)
		if err != nil {
			return err
		}
		c.Analytics = b
		validated = b
	case "password_storage":
		if value != "keyring" && value != "pgpass" && value != "none" {
			return fmt.Errorf("invalid password_storage value: %s (must be keyring, pgpass, or none)", value)
		}
		c.PasswordStorage = value
		validated = value
	case "debug":
		b, err := setBool("debug", value)
		if err != nil {
			return err
		}
		c.Debug = b
		validated = b
	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	configFile, err := c.ensureConfigDir()
	if err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(configFile)
	v.ReadInConfig()

	v.Set(key, validated)
	// Also update the global viper state
	viper.Set(key, validated)

	if err := v.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

func setBool(key, val string) (bool, error) {
	b, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("invalid %s value: %s (must be true or false)", key, val)
	}
	return b, nil
}

// updateField uses reflection to update the field in the Config struct corresponding to the given key.
// This is used to keep the struct in sync with the viper state when setting values.
func (c *Config) updateField(key string, value any) error {
	v := reflect.ValueOf(c).Elem()
	t := v.Type()

	// Find field by json tag
	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")

		if tag == key {
			fieldValue := v.Field(i)
			if fieldValue.CanSet() {
				fieldValue.Set(reflect.ValueOf(value))
				viper.Set(key, value)
				return nil
			}
		}
	}
	return fmt.Errorf("field not found: %s", key)
}

func (c *Config) Unset(key string) error {
	configFile, err := c.ensureConfigDir()
	if err != nil {
		return err
	}

	vCurrent := viper.New()
	vCurrent.SetConfigFile(configFile)
	vCurrent.ReadInConfig()

	vNew := viper.New()
	vNew.SetConfigFile(configFile)

	_, validKey := defaultValues[key]
	for k, v := range vCurrent.AllSettings() {
		if k != key {
			vNew.Set(k, v)
		} else {
			validKey = true
		}
	}

	if !validKey {
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	// Apply the default to the current global viper state
	if def, ok := defaultValues[key]; ok {
		c.updateField(key, def)
	}

	if err := vNew.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

func (c *Config) Reset() error {
	configFile, err := c.ensureConfigDir()
	if err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(configFile)

	// Preserve the project id, as this is part of the auth scheme
	v.Set("project_id", c.ProjectID)

	if err := v.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	// Apply all defaults to the current global viper state
	for key, value := range defaultValues {
		if key == "project_id" {
			continue
		}
		c.updateField(key, value)
	}

	return nil
}

func GetConfigFile(dir string) string {
	return filepath.Join(dir, ConfigFileName)
}

// TODO: This function is currently used to get the directory that the API
// key fallback file should be stored in (see api_key.go). But ideally, those
// functions would take a Config struct and use the ConfigDir field instead.
func GetConfigDir() string {
	return filepath.Dir(viper.ConfigFileUsed())
}

func GetDefaultConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./.config/tiger"
	}

	return filepath.Join(homeDir, ".config", "tiger")
}

func GetEffectiveConfigDir(configDirFlag *pflag.Flag) string {
	if configDirFlag.Changed {
		return util.ExpandPath(configDirFlag.Value.String())
	}

	if dir := os.Getenv("TIGER_CONFIG_DIR"); dir != "" {
		return util.ExpandPath(dir)
	}

	return GetDefaultConfigDir()
}

// ResetGlobalConfig clears the global viper state for testing
// This is mainly used to reset viper configuration between test runs
func ResetGlobalConfig() {
	viper.Reset()
}
