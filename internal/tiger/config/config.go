package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/timescale/tiger-cli/internal/tiger/util"
)

type Config struct {
	APIURL               string        `mapstructure:"api_url" yaml:"api_url"`
	Analytics            bool          `mapstructure:"analytics" yaml:"analytics"`
	Color                bool          `mapstructure:"color" yaml:"color"`
	ConfigDir            string        `mapstructure:"config_dir" yaml:"-"`
	ConsoleURL           string        `mapstructure:"console_url" yaml:"console_url"`
	Debug                bool          `mapstructure:"debug" yaml:"debug"`
	DocsMCP              bool          `mapstructure:"docs_mcp" yaml:"docs_mcp"`
	DocsMCPURL           string        `mapstructure:"docs_mcp_url" yaml:"docs_mcp_url"`
	GatewayURL           string        `mapstructure:"gateway_url" yaml:"gateway_url"`
	Output               string        `mapstructure:"output" yaml:"output"`
	PasswordStorage      string        `mapstructure:"password_storage" yaml:"password_storage"`
	ReleasesURL          string        `mapstructure:"releases_url" yaml:"releases_url"`
	ServiceID            string        `mapstructure:"service_id" yaml:"service_id"`
	VersionCheckInterval time.Duration `mapstructure:"version_check_interval" yaml:"version_check_interval"`
	VersionCheckLastTime time.Time     `mapstructure:"version_check_last_time" yaml:"version_check_last_time"`
	viper                *viper.Viper  `mapstructure:"-" yaml:"-"`
}

type ConfigOutput struct {
	APIURL               *string        `mapstructure:"api_url" json:"api_url,omitempty" yaml:"api_url,omitempty"`
	Analytics            *bool          `mapstructure:"analytics" json:"analytics,omitempty" yaml:"analytics,omitempty"`
	Color                *bool          `mapstructure:"color" json:"color,omitempty" yaml:"color,omitempty"`
	ConfigDir            *string        `mapstructure:"config_dir" json:"config_dir,omitempty" yaml:"config_dir,omitempty"`
	ConsoleURL           *string        `mapstructure:"console_url" json:"console_url,omitempty" yaml:"console_url,omitempty"`
	Debug                *bool          `mapstructure:"debug" json:"debug,omitempty" yaml:"debug,omitempty"`
	DocsMCP              *bool          `mapstructure:"docs_mcp" json:"docs_mcp,omitempty" yaml:"docs_mcp,omitempty"`
	DocsMCPURL           *string        `mapstructure:"docs_mcp_url" json:"docs_mcp_url,omitempty" yaml:"docs_mcp_url,omitempty"`
	GatewayURL           *string        `mapstructure:"gateway_url" json:"gateway_url,omitempty" yaml:"gateway_url,omitempty"`
	Output               *string        `mapstructure:"output" json:"output,omitempty" yaml:"output,omitempty"`
	PasswordStorage      *string        `mapstructure:"password_storage" json:"password_storage,omitempty" yaml:"password_storage,omitempty"`
	ReleasesURL          *string        `mapstructure:"releases_url" json:"releases_url,omitempty" yaml:"releases_url,omitempty"`
	ServiceID            *string        `mapstructure:"service_id" json:"service_id,omitempty" yaml:"service_id,omitempty"`
	VersionCheckInterval *time.Duration `mapstructure:"version_check_interval" json:"version_check_interval,omitempty" yaml:"version_check_interval,omitempty"`
	VersionCheckLastTime *time.Time     `mapstructure:"version_check_last_time" json:"version_check_last_time,omitempty" yaml:"version_check_last_time,omitempty"`
}

const (
	ConfigFileName              = "config.yaml"
	DefaultAPIURL               = "https://console.cloud.timescale.com/public/api/v1"
	DefaultAnalytics            = true
	DefaultColor                = true
	DefaultConsoleURL           = "https://console.cloud.timescale.com"
	DefaultDebug                = false
	DefaultDocsMCP              = true
	DefaultDocsMCPURL           = "https://mcp.tigerdata.com/docs"
	DefaultGatewayURL           = "https://console.cloud.timescale.com/api"
	DefaultOutput               = "table"
	DefaultPasswordStorage      = "keyring"
	DefaultReleasesURL          = "https://cli.tigerdata.com"
	DefaultVersionCheckInterval = 24 * time.Hour
)

var defaultValues = map[string]any{
	"analytics":               DefaultAnalytics,
	"api_url":                 DefaultAPIURL,
	"color":                   DefaultColor,
	"console_url":             DefaultConsoleURL,
	"debug":                   DefaultDebug,
	"docs_mcp":                DefaultDocsMCP,
	"docs_mcp_url":            DefaultDocsMCPURL,
	"gateway_url":             DefaultGatewayURL,
	"output":                  DefaultOutput,
	"password_storage":        DefaultPasswordStorage,
	"releases_url":            DefaultReleasesURL,
	"service_id":              "",
	"version_check_interval":  DefaultVersionCheckInterval,
	"version_check_last_time": time.Time{},
}

func ApplyDefaults(v *viper.Viper) {
	for key, value := range defaultValues {
		v.SetDefault(key, value)
	}
}

func ApplyEnvOverrides(v *viper.Viper) {
	v.SetEnvPrefix("TIGER")
	v.AutomaticEnv()
}

func ReadInConfig(v *viper.Viper) error {
	// Try to read config file if it exists
	// If file doesn't exist, that's okay - we'll use defaults and env vars
	if err := v.ReadInConfig(); err != nil &&
		!errors.As(err, &viper.ConfigFileNotFoundError{}) &&
		!errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// SetupViper configures the global Viper instance with defaults, env vars, and config file
func SetupViper(configDir string) error {
	v := viper.GetViper()

	// Configure viper to read from config file
	configFile := GetConfigFile(configDir)
	v.SetConfigFile(configFile)

	// Configure viper to read from env vars
	ApplyEnvOverrides(v)

	// Set defaults for all config values
	ApplyDefaults(v)

	return ReadInConfig(v)
}

func FromViper(v *viper.Viper) (*Config, error) {
	cfg := &Config{
		ConfigDir: filepath.Dir(v.ConfigFileUsed()),
		viper:     v,
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return cfg, nil
}

func ForOutputFromViper(v *viper.Viper) (*ConfigOutput, error) {
	configDir := filepath.Dir(v.ConfigFileUsed())
	cfg := &ConfigOutput{
		ConfigDir: &configDir,
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config for output: %w", err)
	}

	return cfg, nil
}

// Load creates a new Config instance from the current viper state
// This function should be called after SetupViper has been called to initialize viper
func Load() (*Config, error) {
	v := viper.GetViper()

	// Try to read config file into viper to ensure we're unmarshaling the most
	// up-to-date values into the config struct.
	if err := ReadInConfig(v); err != nil {
		return nil, err
	}

	return FromViper(v)
}

func ensureConfigDir(configDir string) (string, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("error creating config directory: %w", err)
	}
	return GetConfigFile(configDir), nil
}

func (c *Config) EnsureConfigDir() (string, error) {
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
	// Validate and update the field
	validated, err := c.UpdateField(key, value)
	if err != nil {
		return err
	}

	// Write to config file
	configFile, err := c.EnsureConfigDir()
	if err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(configFile)
	v.ReadInConfig()

	v.Set(key, validated)

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

// UpdateField updates the field in the Config struct corresponding to the given key.
// It accepts either a string (from user input) or a typed value (string/bool from defaults).
// The function validates the value and updates both the struct field and viper state.
func (c *Config) UpdateField(key string, value any) (any, error) {
	var validated any

	switch key {
	case "api_url":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("api_url must be string, got %T", value)
		}
		c.APIURL = s
		validated = s

	case "console_url":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("console_url must be string, got %T", value)
		}
		c.ConsoleURL = s
		validated = s

	case "gateway_url":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("gateway_url must be string, got %T", value)
		}
		c.GatewayURL = s
		validated = s

	case "docs_mcp":
		switch v := value.(type) {
		case bool:
			c.DocsMCP = v
			validated = v
		case string:
			b, err := setBool("docs_mcp", v)
			if err != nil {
				return nil, err
			}
			c.DocsMCP = b
			validated = b
		default:
			return nil, fmt.Errorf("docs_mcp must be string or bool, got %T", value)
		}

	case "docs_mcp_url":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("docs_mcp_url must be string, got %T", value)
		}
		c.DocsMCPURL = s
		validated = s

	case "service_id":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("service_id must be string, got %T", value)
		}
		c.ServiceID = s
		validated = s

	case "color":
		switch v := value.(type) {
		case bool:
			c.Color = v
			validated = v
		case string:
			b, err := setBool("color", v)
			if err != nil {
				return nil, err
			}
			c.Color = b
			validated = b
		default:
			return nil, fmt.Errorf("color must be string or bool, got %T", value)
		}

	case "output":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("output must be string, got %T", value)
		}
		if err := ValidateOutputFormat(s, false); err != nil {
			return nil, err
		}
		c.Output = s
		validated = s

	case "analytics":
		switch v := value.(type) {
		case bool:
			c.Analytics = v
			validated = v
		case string:
			b, err := setBool("analytics", v)
			if err != nil {
				return nil, err
			}
			c.Analytics = b
			validated = b
		default:
			return nil, fmt.Errorf("analytics must be string or bool, got %T", value)
		}

	case "password_storage":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("password_storage must be string, got %T", value)
		}
		if s != "keyring" && s != "pgpass" && s != "none" {
			return nil, fmt.Errorf("invalid password_storage value: %s (must be keyring, pgpass, or none)", s)
		}
		c.PasswordStorage = s
		validated = s

	case "debug":
		switch v := value.(type) {
		case bool:
			c.Debug = v
			validated = v
		case string:
			b, err := setBool("debug", v)
			if err != nil {
				return nil, err
			}
			c.Debug = b
			validated = b
		default:
			return nil, fmt.Errorf("debug must be string or bool, got %T", value)
		}

	case "releases_url":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("releases_url must be string, got %T", value)
		}
		c.ReleasesURL = s
		validated = s

	case "version_check_interval":
		switch v := value.(type) {
		case time.Duration:
			if v < 0 {
				return nil, fmt.Errorf("version_check_interval must be non-negative (0 to disable)")
			}
			c.VersionCheckInterval = v
			validated = v
		case string:
			// Parse duration string like "1h", "30m", "24h"
			d, err := time.ParseDuration(v)
			if err != nil {
				return nil, fmt.Errorf("invalid version_check_interval value: %s (must be a duration like '1h', '30m', etc.)", v)
			}
			if d < 0 {
				return nil, fmt.Errorf("version_check_interval must be non-negative (0 to disable)")
			}
			c.VersionCheckInterval = d
			validated = d
		default:
			return nil, fmt.Errorf("version_check_interval must be string or duration, got %T", value)
		}

	case "version_check_last_time":
		nowish := time.Now().Add(time.Hour)
		switch v := value.(type) {
		case time.Time:
			if v.After(nowish) {
				return nil, fmt.Errorf("version_check_last_time cannot be in the future")
			}
			c.VersionCheckLastTime = v
			validated = v
		case string:
			// Try parsing as RFC3339 first, then as unix timestamp
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				// Try parsing as unix timestamp
				i, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid version_check_last_time value: %s (must be RFC3339 timestamp or unix timestamp)", v)
				}
				t = time.Unix(i, 0)
			}
			if t.After(nowish) {
				return nil, fmt.Errorf("version_check_last_time cannot be in the future")
			}
			c.VersionCheckLastTime = t
			validated = t
		default:
			return nil, fmt.Errorf("version_check_last_time must be string or time, got %T", value)
		}

	default:
		return nil, fmt.Errorf("unknown configuration key: %s", key)
	}

	if c.viper == nil {
		viper.Set(key, validated)
	} else {
		c.viper.Set(key, validated)
	}
	return validated, nil
}

func (c *Config) Unset(key string) error {
	configFile, err := c.EnsureConfigDir()
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
		if _, err := c.UpdateField(key, def); err != nil {
			return err
		}
	}

	if err := vNew.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

func (c *Config) Reset() error {
	configFile, err := c.EnsureConfigDir()
	if err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(configFile)

	if err := v.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	// Apply all defaults to the current global viper state
	for key, value := range defaultValues {
		if _, err := c.UpdateField(key, value); err != nil {
			return err
		}
	}

	return nil
}

func GetConfigFile(dir string) string {
	return filepath.Join(dir, ConfigFileName)
}

func (c *Config) GetConfigFile() string {
	return GetConfigFile(c.ConfigDir)
}

// TODO: This function is currently used to get the directory that the API
// key fallback file should be stored in (see credentials.go). But ideally, those
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
