package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/timescale/tiger-cli/internal/util"
)

const (
	ConfigFileName         = "config.yaml"
	DefaultAPIURL          = "https://console.cloud.tigerdata.com/public/api/v1"
	DefaultAnalytics       = true
	DefaultColor           = true
	DefaultConsoleURL      = "https://console.cloud.tigerdata.com"
	DefaultDocsMCP         = true
	DefaultDocsMCPURL      = "https://mcp.tigerdata.com/docs?disabled_skills=ghost-database"
	DefaultGatewayURL      = "https://console.cloud.tigerdata.com/api"
	DefaultMCPMaxRows      = 100
	DefaultOutput          = "table"
	DefaultPasswordStorage = "keyring"
	DefaultReadOnly        = ReadOnlyOff
	DefaultReleasesURL     = "https://cli.tigerdata.com"
	DefaultVersionCheck    = true

	// TigerCLIClientID is the OAuth client identifier registered with
	// savannah-gateway's /idp/external/cli/token endpoint. Used for both the
	// initial PKCE flow and refresh-token grants.
	TigerCLIClientID = "45e1b16d-e435-4049-97b2-8daad150818c"
)

// ReadOnlyMode selects which services read-only mode protects from mutating
// operations. The legacy booleans map onto it: true is ReadOnlyAll, false is
// ReadOnlyOff.
type ReadOnlyMode string

const (
	ReadOnlyAll  ReadOnlyMode = "all"  // every service
	ReadOnlyProd ReadOnlyMode = "prod" // only services tagged PROD
	ReadOnlyOff  ReadOnlyMode = "off"  // none
)

// BlocksAll reports whether the mode blocks writes to every service. It's the
// only verdict reachable without knowing the target.
func (m ReadOnlyMode) BlocksAll() bool {
	return m == ReadOnlyAll
}

// parseReadOnlyMode converts a user-supplied read_only value into a ReadOnlyMode,
// still accepting the booleans this option took before the modes existed.
func parseReadOnlyMode(value string) (ReadOnlyMode, error) {
	trimmed := strings.TrimSpace(value)

	// Empty means unset. Erroring would be unrecoverable — config.Load runs for
	// every command, so even `config set` couldn't repair the file.
	if trimmed == "" {
		return ReadOnlyOff, nil
	}

	// Matched case-insensitively, unlike `output` and `password_storage`: PROD is
	// how the tag is spelled everywhere else, so it's the spelling most likely to
	// be copied, and ParseBool below is case-insensitive anyway.
	lowered := strings.ToLower(trimmed)

	switch mode := ReadOnlyMode(lowered); mode {
	case ReadOnlyAll, ReadOnlyProd, ReadOnlyOff:
		return mode, nil
	}

	// `on` pairs with the `off` mode name; strconv.ParseBool rejects it.
	if lowered == "on" {
		return ReadOnlyAll, nil
	}

	// ParseBool also covers 1/0 and t/f, which is what viper's weak typing
	// produces for a boolean config value.
	if b, err := strconv.ParseBool(trimmed); err == nil {
		if b {
			return ReadOnlyAll, nil
		}
		return ReadOnlyOff, nil
	}

	return "", fmt.Errorf("invalid read_only value: %s (must be all, prod, or off)", value)
}

var defaultValues = map[string]any{
	"analytics":        DefaultAnalytics,
	"api_url":          DefaultAPIURL,
	"color":            DefaultColor,
	"console_url":      DefaultConsoleURL,
	"docs_mcp":         DefaultDocsMCP,
	"docs_mcp_url":     DefaultDocsMCPURL,
	"gateway_url":      DefaultGatewayURL,
	"mcp_max_rows":     DefaultMCPMaxRows,
	"output":           DefaultOutput,
	"password_storage": DefaultPasswordStorage,
	"read_only":        DefaultReadOnly,
	"releases_url":     DefaultReleasesURL,
	"service_id":       "",
	"version_check":    DefaultVersionCheck,
}

// flagBindings maps CLI flag names to the config keys they override. Flags
// missing from a caller's flag set are skipped, so command-local flags (e.g.
// --output) bind only for the commands that define them.
var flagBindings = map[string]string{
	"analytics":        "analytics",
	"color":            "color",
	"output":           "output",
	"password-storage": "password_storage",
	"service-id":       "service_id",
}

// Config holds the effective configuration for a single command invocation,
// resolved through viper's normal precedence (flag > env > file > default).
type Config struct {
	APIURL          string       `mapstructure:"api_url"`
	Analytics       bool         `mapstructure:"analytics"`
	Color           bool         `mapstructure:"color"`
	ConsoleURL      string       `mapstructure:"console_url"`
	DocsMCP         bool         `mapstructure:"docs_mcp"`
	DocsMCPURL      string       `mapstructure:"docs_mcp_url"`
	GatewayURL      string       `mapstructure:"gateway_url"`
	MCPMaxRows      int          `mapstructure:"mcp_max_rows"`
	Output          string       `mapstructure:"output"`
	PasswordStorage string       `mapstructure:"password_storage"`
	ReadOnly        ReadOnlyMode `mapstructure:"read_only"`
	ReleasesURL     string       `mapstructure:"releases_url"`
	ServiceID       string       `mapstructure:"service_id"`
	VersionCheck    bool         `mapstructure:"version_check"`

	ConfigDir string         `mapstructure:"-"`
	flags     *pflag.FlagSet `mapstructure:"-"`
}

// ConfigOutput is the shape `tiger config show` renders. Every field is a
// pointer so unset values can be omitted when defaults are suppressed.
type ConfigOutput struct {
	Analytics       *bool         `mapstructure:"analytics" json:"analytics,omitempty"`
	APIURL          *string       `mapstructure:"api_url" json:"api_url,omitempty"`
	Color           *bool         `mapstructure:"color" json:"color,omitempty"`
	ConsoleURL      *string       `mapstructure:"console_url" json:"console_url,omitempty"`
	DocsMCP         *bool         `mapstructure:"docs_mcp" json:"docs_mcp,omitempty"`
	DocsMCPURL      *string       `mapstructure:"docs_mcp_url" json:"docs_mcp_url,omitempty"`
	GatewayURL      *string       `mapstructure:"gateway_url" json:"gateway_url,omitempty"`
	MCPMaxRows      *int          `mapstructure:"mcp_max_rows" json:"mcp_max_rows,omitempty"`
	Output          *string       `mapstructure:"output" json:"output,omitempty"`
	PasswordStorage *string       `mapstructure:"password_storage" json:"password_storage,omitempty"`
	ReadOnly        *ReadOnlyMode `mapstructure:"read_only" json:"read_only,omitempty"`
	ReleasesURL     *string       `mapstructure:"releases_url" json:"releases_url,omitempty"`
	ServiceID       *string       `mapstructure:"service_id" json:"service_id,omitempty"`
	VersionCheck    *bool         `mapstructure:"version_check" json:"version_check,omitempty"`
}

// Load creates a new Config instance. The provided flag set is used to resolve
// the effective config directory (via the config-dir flag) and to bind the CLI
// flags in flagBindings so they override file/env values. It may be nil for
// callers that have no flags to apply.
func Load(flags *pflag.FlagSet) (*Config, error) {
	cfg := &Config{
		ConfigDir: getEffectiveConfigDir(flags),
		flags:     flags,
	}
	if err := cfg.reload(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadForOutput loads config values for display purposes using a fresh viper
// instance, independent of CLI flags. This keeps `tiger config show -o json`
// from reporting the flag's format as the configured `output` value.
func LoadForOutput(configDir string, withEnv bool, noDefaults bool) (*ConfigOutput, error) {
	v := viper.New()
	v.SetConfigFile(GetConfigFile(configDir))

	if withEnv {
		applyEnvOverrides(v)
	}
	if !noDefaults {
		applyDefaults(v)
	}

	if err := readInConfig(v); err != nil {
		return nil, err
	}
	migrateVersionCheck(v)

	cfg := &ConfigOutput{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config for output: %w", err)
	}
	if cfg.ReadOnly != nil {
		mode, err := parseReadOnlyMode(string(*cfg.ReadOnly))
		if err != nil {
			return nil, err
		}
		cfg.ReadOnly = &mode
	}

	return cfg, nil
}

// reload reads the config file and resolves effective values through viper's
// normal precedence (flag > env > file > default). Called by Load for the
// initial load, and by Set/Unset/Reset after writing the config file.
func (c *Config) reload() error {
	v := viper.New()
	v.SetConfigFile(c.GetConfigFile())
	applyEnvOverrides(v)
	applyDefaults(v)

	if err := bindFlags(v, c.flags); err != nil {
		return fmt.Errorf("failed to bind flags: %w", err)
	}

	if err := readInConfig(v); err != nil {
		return err
	}
	migrateVersionCheck(v)

	if err := v.Unmarshal(c); err != nil {
		return fmt.Errorf("error unmarshaling config: %w", err)
	}

	// read_only arrives as whatever its source held — viper's WeaklyTypedInput
	// renders a YAML bool or int as "1"/"0" — so normalize it here rather than
	// leaving a value no gate matches.
	mode, err := parseReadOnlyMode(string(c.ReadOnly))
	if err != nil {
		return err
	}
	c.ReadOnly = mode
	return nil
}

// Set writes a config value and returns it as stored, which isn't always the
// input: read_only normalizes legacy booleans into modes. Echo the return value
// to the user, not the argument.
func (c *Config) Set(key, value string) (string, error) {
	// Validate and convert the value to the correct type for the config file
	validated, err := validateValue(key, value)
	if err != nil {
		return "", err
	}

	// Write to config file
	configFile, err := c.EnsureConfigDir()
	if err != nil {
		return "", err
	}

	v := viper.New()
	v.SetConfigFile(configFile)
	v.ReadInConfig()

	v.Set(key, validated)

	if err := v.WriteConfigAs(configFile); err != nil {
		return "", fmt.Errorf("error writing config file: %w", err)
	}

	return fmt.Sprint(validated), c.reload()
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

	if err := vNew.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return c.reload()
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

	return c.reload()
}

// EnsureConfigDir creates the config directory if it does not already exist
// and returns the path to the config file within it.
func (c *Config) EnsureConfigDir() (string, error) {
	return ensureConfigDir(c.ConfigDir)
}

func (c *Config) GetConfigFile() string {
	return GetConfigFile(c.ConfigDir)
}

// ValidConfigOptions returns every settable config key, sorted: shells present
// completion candidates in the order they are given, so an unsorted list would
// reshuffle on every keystroke.
func ValidConfigOptions() []string {
	return slices.Sorted(maps.Keys(defaultValues))
}

// ValidConfigOptionValues returns known completion values for a config key's
// value, or nil if the key doesn't have a fixed set of values.
func ValidConfigOptionValues(key string) []string {
	switch key {
	case "output":
		return ValidOutputFormats()
	case "password_storage":
		return ValidPasswordStorageOptions()
	case "read_only":
		return ValidReadOnlyModes()
	case "analytics", "color", "docs_mcp", "version_check":
		return []string{"true", "false"}
	default:
		return nil
	}
}

// ValidPasswordStorageOptions returns the accepted values for password_storage.
func ValidPasswordStorageOptions() []string {
	return []string{"keyring", "pgpass", "none"}
}

func ValidReadOnlyModes() []string {
	return []string{string(ReadOnlyAll), string(ReadOnlyProd), string(ReadOnlyOff)}
}

func GetConfigFile(dir string) string {
	return filepath.Join(dir, ConfigFileName)
}

func GetDefaultConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./.config/tiger"
	}

	return filepath.Join(homeDir, ".config", "tiger")
}

// getEffectiveConfigDir resolves the config directory from the --config-dir
// flag, then TIGER_CONFIG_DIR, then the default location.
func getEffectiveConfigDir(flags *pflag.FlagSet) string {
	if flags != nil {
		if flag := flags.Lookup("config-dir"); flag != nil && flag.Changed {
			return util.ExpandPath(flag.Value.String())
		}
	}

	if dir := os.Getenv("TIGER_CONFIG_DIR"); dir != "" {
		return util.ExpandPath(dir)
	}

	return GetDefaultConfigDir()
}

func ensureConfigDir(configDir string) (string, error) {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", fmt.Errorf("error creating config directory: %w (set TIGER_CONFIG_DIR or --config-dir to a writable path)", err)
	}
	return GetConfigFile(configDir), nil
}

func applyDefaults(v *viper.Viper) {
	for key, value := range defaultValues {
		v.SetDefault(key, value)
	}
}

func applyEnvOverrides(v *viper.Viper) {
	v.SetEnvPrefix("TIGER")
	v.AutomaticEnv()
}

func readInConfig(v *viper.Viper) error {
	// Try to read config file if it exists
	// If file doesn't exist, that's okay - we'll use defaults and env vars
	if err := v.ReadInConfig(); err != nil &&
		!errors.As(err, &viper.ConfigFileNotFoundError{}) &&
		!errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func bindFlags(v *viper.Viper, flags *pflag.FlagSet) error {
	if flags == nil {
		return nil
	}

	var errs []error
	for name, key := range flagBindings {
		if flag := flags.Lookup(name); flag != nil {
			errs = append(errs, v.BindPFlag(key, flag))
		}
	}
	return errors.Join(errs...)
}

// migrateVersionCheck preserves backward compatibility with configs written by
// older CLI versions, which used a `version_check_interval` duration (0 to
// disable) instead of the current `version_check` bool. If a pre-existing
// config file set the old key and not the new one, we derive the new value
// from it (0 → false, any non-zero interval → true) so a user who had disabled
// update checks doesn't have them silently re-enabled on upgrade.
//
// The derived value is applied via SetDefault, so an explicit `version_check`
// from the config file or a TIGER_VERSION_CHECK env var still takes precedence.
// It must be called after applyDefaults so the derived value overrides the
// generic default, and after readInConfig so InConfig can see the file keys.
// This is an in-memory shim only; the old key remains in the file until it is
// rewritten (e.g. via `tiger config set`/`unset`).
func migrateVersionCheck(v *viper.Viper) {
	if v.InConfig("version_check_interval") && !v.InConfig("version_check") {
		v.SetDefault("version_check", v.GetDuration("version_check_interval") != 0)
	}
}

// validateValue validates and converts a user-provided value for the given
// config key. String values are returned as-is (after any key-specific
// validation); bool and int keys are parsed from their string form. Returns
// the converted value suitable for writing to the config file.
func validateValue(key, value string) (any, error) {
	switch key {
	case "api_url", "console_url", "docs_mcp_url", "gateway_url", "releases_url", "service_id":
		return value, nil
	case "analytics", "color", "docs_mcp", "version_check":
		return parseBool(key, value)
	case "read_only":
		// parseReadOnlyMode reads empty as off so a malformed file can't brick every
		// command. Typed here it's a slip, and silently turning protection off is
		// the wrong way to resolve one — `unset` clears the key.
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("invalid read_only value: %q (must be all, prod, or off; use `tiger config unset read_only` to clear it)", value)
		}
		// Stored as the canonical mode, cleaning up a legacy boolean in the file.
		mode, err := parseReadOnlyMode(value)
		if err != nil {
			return nil, err
		}
		return string(mode), nil
	case "mcp_max_rows":
		return parsePositiveInt(key, value)
	case "output":
		if err := ValidateOutputFormat(value); err != nil {
			return nil, err
		}
		return value, nil
	case "password_storage":
		options := ValidPasswordStorageOptions()
		if !slices.Contains(options, value) {
			return nil, fmt.Errorf("invalid password_storage value: %s (must be one of: %s)", value, strings.Join(options, ", "))
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unknown configuration key: %s", key)
	}
}

func parseBool(key, value string) (bool, error) {
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid %s value: %s (must be true or false)", key, value)
	}
	return b, nil
}

func parsePositiveInt(key, value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value: %s (must be an integer)", key, value)
	}
	if n < 1 {
		return 0, fmt.Errorf("%s must be at least 1, got %d", key, n)
	}
	return n, nil
}
