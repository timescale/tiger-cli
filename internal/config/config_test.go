package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/timescale/tiger-cli/internal/util"
)

// setupTestConfig returns an isolated, empty config directory. No config file
// is written: tests that need one write it themselves, so a test named for a
// missing file really runs without one.
func setupTestConfig(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func TestLoad_DefaultValues(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load(nil) failed: %v", err)
	}

	// Verify default values
	if cfg.APIURL != DefaultAPIURL {
		t.Errorf("Expected APIURL %s, got %s", DefaultAPIURL, cfg.APIURL)
	}
	if cfg.Output != DefaultOutput {
		t.Errorf("Expected Output %s, got %s", DefaultOutput, cfg.Output)
	}
	if cfg.Analytics != DefaultAnalytics {
		t.Errorf("Expected Analytics %t, got %t", DefaultAnalytics, cfg.Analytics)
	}
	if cfg.ReadOnly != DefaultReadOnly {
		t.Errorf("Expected ReadOnly %s, got %s", DefaultReadOnly, cfg.ReadOnly)
	}
	if cfg.MCPMaxRows != DefaultMCPMaxRows {
		t.Errorf("Expected MCPMaxRows %d, got %d", DefaultMCPMaxRows, cfg.MCPMaxRows)
	}
	if cfg.ConfigDir != tmpDir {
		t.Errorf("Expected ConfigDir %s, got %s", tmpDir, cfg.ConfigDir)
	}
}

func TestLoad_FromConfigFile(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Create config file
	configContent := `api_url: https://custom.api.com/v1
service_id: test-service-456
output: json
analytics: false
read_only: true
`
	configFile := GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load(nil) failed: %v", err)
	}

	// Verify loaded values
	if cfg.APIURL != "https://custom.api.com/v1" {
		t.Errorf("Expected APIURL https://custom.api.com/v1, got %s", cfg.APIURL)
	}
	if cfg.ServiceID != "test-service-456" {
		t.Errorf("Expected ServiceID test-service-456, got %s", cfg.ServiceID)
	}
	if cfg.Output != "json" {
		t.Errorf("Expected Output json, got %s", cfg.Output)
	}
	if cfg.Analytics != false {
		t.Errorf("Expected Analytics false, got %t", cfg.Analytics)
	}
	// A legacy boolean still means "every service".
	if cfg.ReadOnly != ReadOnlyAll {
		t.Errorf("Expected ReadOnly %s, got %s", ReadOnlyAll, cfg.ReadOnly)
	}
}

// TestParseReadOnlyMode covers the value vocabulary. It's a pure function, so the
// matrix lives here rather than in TestLoad_ReadOnlyMode, which pays for a temp
// dir and a config file per case.
func TestParseReadOnlyMode(t *testing.T) {
	tests := []struct {
		value   string
		want    ReadOnlyMode
		wantErr bool
	}{
		{value: "all", want: ReadOnlyAll},
		{value: "prod", want: ReadOnlyProd},
		{value: "off", want: ReadOnlyOff},

		// Accepted spellings, legacy and otherwise.
		{value: "true", want: ReadOnlyAll},
		{value: "false", want: ReadOnlyOff},
		{value: "1", want: ReadOnlyAll},
		{value: "0", want: ReadOnlyOff},
		{value: "on", want: ReadOnlyAll},
		// Lenient here on purpose: a file with a bare `read_only:` must not fail
		// the load. validateValue rejects it on the config set path instead.
		{value: "", want: ReadOnlyOff},
		{value: "  prod  ", want: ReadOnlyProd},

		// Case-insensitive, so the PROD spelling used everywhere else is accepted.
		{value: "PROD", want: ReadOnlyProd},
		{value: "ON", want: ReadOnlyAll},
		{value: "Off", want: ReadOnlyOff},

		{value: "sometimes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.value), func(t *testing.T) {
			got, err := parseReadOnlyMode(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseReadOnlyMode(%q) = %q, want an error", tt.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseReadOnlyMode(%q) failed: %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("parseReadOnlyMode(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestLoad_ReadOnlyMode covers the plumbing rather than the vocabulary (see
// TestParseReadOnlyMode): that each source reaches the parser, and that a shape
// only viper can produce survives the round trip.
func TestLoad_ReadOnlyMode(t *testing.T) {
	tests := []struct {
		name     string
		fileBody string
		env      string
		want     ReadOnlyMode
		wantErr  bool
	}{
		{name: "unset falls back to the default", want: DefaultReadOnly},
		{name: "from file", fileBody: "read_only: prod\n", want: ReadOnlyProd},
		{name: "from env", env: "prod", want: ReadOnlyProd},
		{name: "env overrides file", fileBody: "read_only: all\n", env: "off", want: ReadOnlyOff},
		{name: "invalid value fails the load", fileBody: "read_only: sometimes\n", wantErr: true},

		// A YAML int reaches the decoder as an int, not a string. Left unparsed it
		// would store a mode no gate matches, and the old bool field read 1 as
		// true, so this is the fail-open regression guard.
		{name: "YAML int", fileBody: "read_only: 1\n", want: ReadOnlyAll},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := setupTestConfig(t)

			if tt.fileBody != "" {
				if err := os.WriteFile(GetConfigFile(tmpDir), []byte(tt.fileBody), 0o644); err != nil {
					t.Fatalf("Failed to write config file: %v", err)
				}
			}

			t.Setenv("TIGER_CONFIG_DIR", tmpDir)
			if tt.env != "" {
				t.Setenv("TIGER_READ_ONLY", tt.env)
			}

			cfg, err := Load(nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load(nil) succeeded with ReadOnly = %q, want an error", cfg.ReadOnly)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(nil) failed: %v", err)
			}
			if cfg.ReadOnly != tt.want {
				t.Errorf("ReadOnly = %q, want %q", cfg.ReadOnly, tt.want)
			}
		})
	}
}

func TestLoad_MigrateVersionCheck(t *testing.T) {
	tests := []struct {
		name     string
		fileBody string
		env      map[string]string
		want     bool
	}{
		{
			name:     "legacy interval 0 keeps checks disabled",
			fileBody: "version_check_interval: 0\n",
			want:     false,
		},
		{
			name:     "explicit version_check overrides legacy interval",
			fileBody: "version_check_interval: 0\nversion_check: true\n",
			want:     true,
		},
		{
			name:     "env var overrides legacy interval",
			fileBody: "version_check_interval: 24h\n",
			env:      map[string]string{"TIGER_VERSION_CHECK": "false"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := setupTestConfig(t)

			configFile := GetConfigFile(tmpDir)
			if err := os.WriteFile(configFile, []byte(tt.fileBody), 0o644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			os.Setenv("TIGER_CONFIG_DIR", tmpDir)
			t.Cleanup(func() { os.Unsetenv("TIGER_CONFIG_DIR") })
			for k, val := range tt.env {
				os.Setenv(k, val)
				t.Cleanup(func() { os.Unsetenv(k) })
			}

			cfg, err := Load(nil)
			if err != nil {
				t.Fatalf("Load(nil) failed: %v", err)
			}
			if cfg.VersionCheck != tt.want {
				t.Errorf("VersionCheck = %t, want %t", cfg.VersionCheck, tt.want)
			}
		})
	}
}

func TestLoad_FromEnvironmentVariables(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Set environment variables
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_API_URL", "https://env.api.com/v1")
	os.Setenv("TIGER_SERVICE_ID", "env-service-101")
	os.Setenv("TIGER_OUTPUT", "yaml")
	os.Setenv("TIGER_ANALYTICS", "false")
	os.Setenv("TIGER_READ_ONLY", "true")

	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_API_URL")
		os.Unsetenv("TIGER_SERVICE_ID")
		os.Unsetenv("TIGER_OUTPUT")
		os.Unsetenv("TIGER_ANALYTICS")
		os.Unsetenv("TIGER_READ_ONLY")
	}()

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load(nil) failed: %v", err)
	}

	// Verify environment values
	if cfg.APIURL != "https://env.api.com/v1" {
		t.Errorf("Expected APIURL https://env.api.com/v1, got %s", cfg.APIURL)
	}
	if cfg.ServiceID != "env-service-101" {
		t.Errorf("Expected ServiceID env-service-101, got %s", cfg.ServiceID)
	}
	if cfg.Output != "yaml" {
		t.Errorf("Expected Output yaml, got %s", cfg.Output)
	}
	if cfg.Analytics != false {
		t.Errorf("Expected Analytics false, got %t", cfg.Analytics)
	}
	if cfg.ReadOnly != ReadOnlyAll {
		t.Errorf("Expected ReadOnly %s, got %s", ReadOnlyAll, cfg.ReadOnly)
	}
}

func TestLoad_Precedence(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Create config file with some values
	configContent := `api_url: https://file.api.com/v1
output: table
analytics: true
`
	configFile := GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variables that should override config file
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_OUTPUT", "json")

	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_OUTPUT")
	}()

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load(nil) failed: %v", err)
	}

	// Environment should override config file
	if cfg.Output != "json" {
		t.Errorf("Expected Output json (env override), got %s", cfg.Output)
	}

	// Config file should be used where env vars aren't set
	if cfg.APIURL != "https://file.api.com/v1" {
		t.Errorf("Expected APIURL https://file.api.com/v1 (from file), got %s", cfg.APIURL)
	}
	if cfg.Analytics != true {
		t.Errorf("Expected Analytics true (from file), got %t", cfg.Analytics)
	}
}

func TestLoad_IndependentInstances(t *testing.T) {
	tmpDir := setupTestConfig(t)

	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	// First load
	cfg1, err := Load(nil)
	if err != nil {
		t.Fatalf("First Load(nil) failed: %v", err)
	}

	// Second load should return new independent instance
	cfg2, err := Load(nil)
	if err != nil {
		t.Fatalf("Second Load(nil) failed: %v", err)
	}

	// Should be different instances but same values
	if cfg1 == cfg2 {
		t.Error("Expected different config instances, got same instance")
	}

	// But should have same configuration values
	if cfg1.APIURL != cfg2.APIURL || cfg1.Output != cfg2.Output {
		t.Error("Config instances should have same values even if different objects")
	}
}

// Set writes through to the config file, not just the in-memory struct, so a
// separately loaded Config sees the same values.
func TestSet_PersistsToDisk(t *testing.T) {
	tmpDir := setupTestConfig(t)

	cfg := &Config{ConfigDir: tmpDir}
	values := map[string]string{
		"api_url":    "https://test.api.com/v1",
		"service_id": "test-service",
		"output":     "json",
		"analytics":  "false",
	}
	for key, value := range values {
		if _, err := cfg.Set(key, value); err != nil {
			t.Fatalf("Set(%q, %q) failed: %v", key, value, err)
		}
	}

	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	loadedCfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loadedCfg.APIURL != cfg.APIURL {
		t.Errorf("Expected APIURL %s, got %s", cfg.APIURL, loadedCfg.APIURL)
	}
	if loadedCfg.ServiceID != cfg.ServiceID {
		t.Errorf("Expected ServiceID %s, got %s", cfg.ServiceID, loadedCfg.ServiceID)
	}
	if loadedCfg.Output != cfg.Output {
		t.Errorf("Expected Output %s, got %s", cfg.Output, loadedCfg.Output)
	}
	if loadedCfg.Analytics != cfg.Analytics {
		t.Errorf("Expected Analytics %t, got %t", cfg.Analytics, loadedCfg.Analytics)
	}
}

func TestSet(t *testing.T) {
	tmpDir := setupTestConfig(t)

	cfg := &Config{
		APIURL:    DefaultAPIURL,
		Output:    DefaultOutput,
		Analytics: DefaultAnalytics,
		ConfigDir: tmpDir,
	}

	tests := []struct {
		key           string
		value         string
		expectedError bool
		checkFunc     func() bool
	}{
		{
			key:   "api_url",
			value: "https://new.api.com/v1",
			checkFunc: func() bool {
				return cfg.APIURL == "https://new.api.com/v1"
			},
		},
		{
			key:   "service_id",
			value: "new-service-456",
			checkFunc: func() bool {
				return cfg.ServiceID == "new-service-456"
			},
		},
		{
			key:   "output",
			value: "json",
			checkFunc: func() bool {
				return cfg.Output == "json"
			},
		},
		{
			key:   "output",
			value: "yaml",
			checkFunc: func() bool {
				return cfg.Output == "yaml"
			},
		},
		{
			key:   "output",
			value: "table",
			checkFunc: func() bool {
				return cfg.Output == "table"
			},
		},
		{
			key:           "output",
			value:         "invalid",
			expectedError: true,
		},
		{
			key:   "analytics",
			value: "true",
			checkFunc: func() bool {
				return cfg.Analytics == true
			},
		},
		{
			key:   "analytics",
			value: "false",
			checkFunc: func() bool {
				return cfg.Analytics == false
			},
		},
		{
			key:           "analytics",
			value:         "invalid",
			expectedError: true,
		},
		{
			key:   "mcp_max_rows",
			value: "250",
			checkFunc: func() bool {
				return cfg.MCPMaxRows == 250
			},
		},
		{
			key:           "mcp_max_rows",
			value:         "0",
			expectedError: true,
		},
		{
			key:           "mcp_max_rows",
			value:         "-5",
			expectedError: true,
		},
		{
			key:           "mcp_max_rows",
			value:         "notanumber",
			expectedError: true,
		},
		{
			key:   "read_only",
			value: "prod",
			checkFunc: func() bool {
				return cfg.ReadOnly == ReadOnlyProd
			},
		},
		{
			key:   "read_only",
			value: "true",
			checkFunc: func() bool {
				return cfg.ReadOnly == ReadOnlyAll
			},
		},
		{
			key:           "read_only",
			value:         "invalid",
			expectedError: true,
		},
		// Empty is lenient in parseReadOnlyMode so a malformed file can't brick
		// every command, but typed at config set it's a slip - and resolving one
		// by silently turning protection off is the wrong direction.
		{
			key:           "read_only",
			value:         "",
			expectedError: true,
		},
		{
			key:           "read_only",
			value:         "   ",
			expectedError: true,
		},
		{
			key:           "unknown_key",
			value:         "value",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s=%s", tt.key, tt.value), func(t *testing.T) {
			_, err := cfg.Set(tt.key, tt.value)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.checkFunc != nil && !tt.checkFunc() {
				t.Errorf("Configuration value not set correctly for %s=%s", tt.key, tt.value)
			}
		})
	}
}

// TestSet_ReadOnlyNormalizesOnWrite checks the one thing TestSet's checkFunc
// can't: read_only is stored as the canonical mode rather than verbatim, so a
// config file written with a legacy boolean is cleaned up rather than kept.
func TestSet_ReadOnlyNormalizesOnWrite(t *testing.T) {
	for _, tt := range []struct{ value, want, wantFile string }{
		{value: "prod", want: "prod", wantFile: "read_only: prod"},
		{value: "true", want: "all", wantFile: "read_only: all"},
		{value: "on", want: "all", wantFile: "read_only: all"},
		// The writer quotes `off`, which unquoted would round-trip as a YAML 1.1
		// boolean. That quoting is what makes the mode name safe to store.
		{value: "false", want: "off", wantFile: `read_only: "off"`},
	} {
		t.Run(tt.value, func(t *testing.T) {
			tmpDir := setupTestConfig(t)
			cfg := &Config{ConfigDir: tmpDir}

			stored, err := cfg.Set("read_only", tt.value)
			if err != nil {
				t.Fatalf("Set(read_only, %q) failed: %v", tt.value, err)
			}
			if stored != tt.want {
				t.Errorf("Set returned %q, want %q", stored, tt.want)
			}

			body, err := os.ReadFile(GetConfigFile(tmpDir))
			if err != nil {
				t.Fatalf("Failed to read config file: %v", err)
			}
			if got := strings.TrimSpace(string(body)); got != tt.wantFile {
				t.Errorf("config file = %q, want %q", got, tt.wantFile)
			}
		})
	}
}

func TestUnset(t *testing.T) {
	tmpDir := setupTestConfig(t)

	cfg := &Config{
		APIURL:    "https://custom.api.com/v1",
		ServiceID: "custom-service",
		Output:    "json",
		Analytics: false,
		ConfigDir: tmpDir,
	}

	tests := []struct {
		key           string
		expectedError bool
		checkFunc     func() bool
	}{
		{
			key: "api_url",
			checkFunc: func() bool {
				return cfg.APIURL == DefaultAPIURL
			},
		},
		{
			key: "service_id",
			checkFunc: func() bool {
				return cfg.ServiceID == ""
			},
		},
		{
			key: "output",
			checkFunc: func() bool {
				return cfg.Output == DefaultOutput
			},
		},
		{
			key: "analytics",
			checkFunc: func() bool {
				return cfg.Analytics == DefaultAnalytics
			},
		},
		{
			key:           "unknown_key",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := cfg.Unset(tt.key)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.checkFunc != nil && !tt.checkFunc() {
				t.Errorf("Configuration value not unset correctly for %s", tt.key)
			}
		})
	}
}

func TestReset(t *testing.T) {
	tmpDir := setupTestConfig(t)

	cfg := &Config{
		APIURL:    "https://custom.api.com/v1",
		ServiceID: "custom-service",
		Output:    "json",
		Analytics: false,
		ConfigDir: tmpDir,
	}

	err := cfg.Reset()
	if err != nil {
		t.Fatalf("Reset() failed: %v", err)
	}

	// Verify all values are reset to defaults
	if cfg.APIURL != DefaultAPIURL {
		t.Errorf("Expected APIURL %s, got %s", DefaultAPIURL, cfg.APIURL)
	}
	if cfg.ServiceID != "" {
		t.Errorf("Expected empty ServiceID, got %s", cfg.ServiceID)
	}
	if cfg.Output != DefaultOutput {
		t.Errorf("Expected Output %s, got %s", DefaultOutput, cfg.Output)
	}
	if cfg.Analytics != DefaultAnalytics {
		t.Errorf("Expected Analytics %t, got %t", DefaultAnalytics, cfg.Analytics)
	}
}

func TestLoad_WithMissingConfigFile(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Test Load succeeds with missing file
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load(nil) failed: %v", err)
	}
	if cfg == nil {
		t.Error("Load(nil) returned nil config")
	}

	// Should return defaults when config file is missing
	if cfg.APIURL != DefaultAPIURL {
		t.Errorf("Expected default APIURL %s, got %s", DefaultAPIURL, cfg.APIURL)
	}
	if cfg.Output != DefaultOutput {
		t.Errorf("Expected default Output %s, got %s", DefaultOutput, cfg.Output)
	}
	if cfg.Analytics != DefaultAnalytics {
		t.Errorf("Expected default Analytics %t, got %t", DefaultAnalytics, cfg.Analytics)
	}

	// Second load should create new instance with same values
	cfg2, err := Load(nil)
	if err != nil {
		t.Fatalf("Second Load(nil) failed: %v", err)
	}
	if cfg == cfg2 {
		t.Error("Expected Load(nil) to create new instances, got same instance")
	}
	if cfg.APIURL != cfg2.APIURL {
		t.Error("Expected same configuration values across different instances")
	}
}

func TestLoad_ErrorHandling(t *testing.T) {
	// Test Load() when it fails due to invalid config file
	tmpDir := setupTestConfig(t)

	// Create invalid YAML config file
	invalidConfig := `api_url: https://test.api.com/v1
invalid yaml content [
`
	configFile := GetConfigFile(tmpDir)
	if err := os.WriteFile(configFile, []byte(invalidConfig), 0o644); err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	// Load should fail with invalid config file
	if _, err := Load(nil); err == nil {
		t.Error("Expected Load() to fail with invalid config file, but it succeeded")
	}
}

func TestGetEffectiveConfigDir(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	tests := []struct {
		name      string
		envVar    string
		flagValue string
		noFlags   bool
		expected  string
	}{
		{
			name:     "default behavior",
			expected: GetDefaultConfigDir(),
		},
		{
			name:     "no flag set",
			noFlags:  true,
			envVar:   "/env/config/path",
			expected: "/env/config/path",
		},
		{
			name:     "env var normal path",
			envVar:   "/env/config/path",
			expected: "/env/config/path",
		},
		{
			name:     "env var tilde expansion",
			envVar:   "~/env/config/path/tiger-config",
			expected: filepath.Join(homeDir, "/env/config/path/tiger-config"),
		},
		{
			name:      "flag normal path overrides env var",
			envVar:    "/env/config/path",
			flagValue: "/flag/config/path",
			expected:  "/flag/config/path",
		},
		{
			name:      "flag tilde expansion overrides env var",
			envVar:    "/env/config/path",
			flagValue: "~/flag/config/path",
			expected:  filepath.Join(homeDir, "/flag/config/path"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up env var before each test
			os.Unsetenv("TIGER_CONFIG_DIR")

			// Set env var if specified
			if tt.envVar != "" {
				os.Setenv("TIGER_CONFIG_DIR", tt.envVar)
				defer os.Unsetenv("TIGER_CONFIG_DIR")
			}

			// Create mock flag set
			var fs *pflag.FlagSet
			if !tt.noFlags {
				var flagVar string
				fs = pflag.NewFlagSet("test", pflag.ContinueOnError)
				fs.StringVar(&flagVar, "config-dir", "", "config directory")
				if tt.flagValue != "" {
					fs.Set("config-dir", tt.flagValue)
				}
			}

			// Test the function
			result := getEffectiveConfigDir(fs)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~", homeDir},
		{"~/config", filepath.Join(homeDir, "config")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~invalid", "~invalid"}, // Should not expand
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := util.ExpandPath(tt.input)
			if result != tt.expected {
				t.Errorf("expandPath(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSet_CreatesConfigDirectory(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Use non-existent subdirectory
	configDir := filepath.Join(tmpDir, "nested", "config")

	cfg := &Config{ConfigDir: configDir}
	if _, err := cfg.Set("api_url", "https://test.api.com/v1"); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error("Config directory was not created")
	}

	// Verify config file was created
	configFile := GetConfigFile(configDir)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}

func TestLoad_RereadsEnvironment(t *testing.T) {
	tmpDir := setupTestConfig(t)

	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_SERVICE_ID", "test-service-before")
	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_SERVICE_ID")
	}()

	// Load config first
	cfg1, err := Load(nil)
	if err != nil {
		t.Fatalf("Load(nil) failed: %v", err)
	}

	// Verify environment was used
	if cfg1.ServiceID != "test-service-before" {
		t.Errorf("Expected service ID from env, got %s", cfg1.ServiceID)
	}

	// Change env var
	os.Setenv("TIGER_SERVICE_ID", "test-service-after")

	// Load again should pick up new env value
	cfg2, err := Load(nil)
	if err != nil {
		t.Fatalf("Second Load(nil) failed: %v", err)
	}

	// Should be different instances
	if cfg1 == cfg2 {
		t.Error("Expected different config instances, got same instance")
	}

	// Should have new env value
	if cfg2.ServiceID != "test-service-after" {
		t.Errorf("Expected new service ID, got %s", cfg2.ServiceID)
	}
	// The first instance is untouched by the second load
	if cfg1.ServiceID != "test-service-before" {
		t.Errorf("Expected first config to keep its value, got %s", cfg1.ServiceID)
	}
}

func TestEnsureConfigDir_ErrorSuggestsOverride(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot trigger mkdir failure as root")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	_, err := ensureConfigDir(filepath.Join(parent, "tiger"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, want := range []string{"TIGER_CONFIG_DIR", "--config-dir"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected error to mention %q; got: %s", want, err.Error())
		}
	}
}

// TestValidConfigOptionValues_ReadOnly guards the completion values for
// read_only.
func TestValidConfigOptionValues_ReadOnly(t *testing.T) {
	got := ValidConfigOptionValues("read_only")
	want := []string{"all", "prod", "off"}
	if !slices.Equal(got, want) {
		t.Errorf("ValidConfigOptionValues(\"read_only\") = %v, want %v", got, want)
	}

	// Every offered value must survive the parser that validateValue uses.
	for _, v := range got {
		if _, err := parseReadOnlyMode(v); err != nil {
			t.Errorf("completion offers %q, which parseReadOnlyMode rejects: %v", v, err)
		}
	}
}
