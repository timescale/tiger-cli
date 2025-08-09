package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	// Reset global config before each test run
	globalConfig = nil
	code := m.Run()
	os.Exit(code)
}

func setupTestConfig(t *testing.T) string {
	t.Helper()

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-test-config-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Clean up Viper state
	viper.Reset()
	globalConfig = nil

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
		viper.Reset()
		globalConfig = nil
	})

	return tmpDir
}

func setupViper(t *testing.T, tmpDir string) {
	t.Helper()

	// Set up Viper configuration using the shared function
	configFile := filepath.Join(tmpDir, ConfigFileName)
	if err := SetupViper(configFile); err != nil {
		t.Fatalf("Failed to setup Viper: %v", err)
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	tmpDir := setupTestConfig(t)
	setupViper(t, tmpDir)

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
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
	if cfg.ConfigDir != tmpDir {
		t.Errorf("Expected ConfigDir %s, got %s", tmpDir, cfg.ConfigDir)
	}
}

func TestLoad_FromConfigFile(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Create config file
	configContent := `api_url: https://custom.api.com/v1
project_id: test-project-123
service_id: test-service-456
output: json
analytics: false
`
	configFile := filepath.Join(tmpDir, ConfigFileName)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	setupViper(t, tmpDir)

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify loaded values
	if cfg.APIURL != "https://custom.api.com/v1" {
		t.Errorf("Expected APIURL https://custom.api.com/v1, got %s", cfg.APIURL)
	}
	if cfg.ProjectID != "test-project-123" {
		t.Errorf("Expected ProjectID test-project-123, got %s", cfg.ProjectID)
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
}

func TestLoad_FromEnvironmentVariables(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Set environment variables
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_API_URL", "https://env.api.com/v1")
	os.Setenv("TIGER_PROJECT_ID", "env-project-789")
	os.Setenv("TIGER_SERVICE_ID", "env-service-101")
	os.Setenv("TIGER_OUTPUT", "yaml")
	os.Setenv("TIGER_ANALYTICS", "false")

	setupViper(t, tmpDir)

	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_API_URL")
		os.Unsetenv("TIGER_PROJECT_ID")
		os.Unsetenv("TIGER_SERVICE_ID")
		os.Unsetenv("TIGER_OUTPUT")
		os.Unsetenv("TIGER_ANALYTICS")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify environment values
	if cfg.APIURL != "https://env.api.com/v1" {
		t.Errorf("Expected APIURL https://env.api.com/v1, got %s", cfg.APIURL)
	}
	if cfg.ProjectID != "env-project-789" {
		t.Errorf("Expected ProjectID env-project-789, got %s", cfg.ProjectID)
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
}

func TestLoad_Precedence(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Create config file with some values
	configContent := `api_url: https://file.api.com/v1
project_id: file-project
output: table
analytics: true
`
	configFile := filepath.Join(tmpDir, ConfigFileName)
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variables that should override config file
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_PROJECT_ID", "env-project-override")
	os.Setenv("TIGER_OUTPUT", "json")

	setupViper(t, tmpDir)

	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_PROJECT_ID")
		os.Unsetenv("TIGER_OUTPUT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Environment should override config file
	if cfg.ProjectID != "env-project-override" {
		t.Errorf("Expected ProjectID env-project-override (env override), got %s", cfg.ProjectID)
	}
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

func TestLoad_GlobalConfigSingleton(t *testing.T) {
	tmpDir := setupTestConfig(t)
	setupViper(t, tmpDir)

	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	// First load
	cfg1, err := Load()
	if err != nil {
		t.Fatalf("First Load() failed: %v", err)
	}

	// Second load should return same instance
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Second Load() failed: %v", err)
	}

	if cfg1 != cfg2 {
		t.Error("Expected same config instance, got different instances")
	}
}

func TestSave(t *testing.T) {
	tmpDir := setupTestConfig(t)
	setupViper(t, tmpDir)

	cfg := &Config{
		APIURL:    "https://test.api.com/v1",
		ProjectID: "test-project",
		ServiceID: "test-service",
		Output:    "json",
		Analytics: false,
		ConfigDir: tmpDir,
	}

	err := cfg.Save()
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file was created
	configFile := filepath.Join(tmpDir, ConfigFileName)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Load and verify content
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	viper.Reset()
	globalConfig = nil

	// Setup Viper again to read the saved config file
	setupViper(t, tmpDir)

	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loadedCfg.APIURL != cfg.APIURL {
		t.Errorf("Expected APIURL %s, got %s", cfg.APIURL, loadedCfg.APIURL)
	}
	if loadedCfg.ProjectID != cfg.ProjectID {
		t.Errorf("Expected ProjectID %s, got %s", cfg.ProjectID, loadedCfg.ProjectID)
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
	setupViper(t, tmpDir)

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
			key:   "project_id",
			value: "new-project-123",
			checkFunc: func() bool {
				return cfg.ProjectID == "new-project-123"
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
			key:           "unknown_key",
			value:         "value",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s=%s", tt.key, tt.value), func(t *testing.T) {
			err := cfg.Set(tt.key, tt.value)

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

func TestUnset(t *testing.T) {
	tmpDir := setupTestConfig(t)
	setupViper(t, tmpDir)

	cfg := &Config{
		APIURL:    "https://custom.api.com/v1",
		ProjectID: "custom-project",
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
			key: "project_id",
			checkFunc: func() bool {
				return cfg.ProjectID == ""
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
	setupViper(t, tmpDir)

	cfg := &Config{
		APIURL:    "https://custom.api.com/v1",
		ProjectID: "custom-project",
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
	if cfg.ProjectID != "" {
		t.Errorf("Expected empty ProjectID, got %s", cfg.ProjectID)
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

func TestLoad_Singleton(t *testing.T) {
	tmpDir := setupTestConfig(t)
	setupViper(t, tmpDir)

	// Test when globalConfig is nil (Load succeeds with missing file)
	globalConfig = nil
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg == nil {
		t.Error("Load() returned nil config")
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

	// Test when globalConfig is already set (singleton behavior)
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Second Load() failed: %v", err)
	}
	if cfg != cfg2 {
		t.Error("Expected Load() to return same instance when globalConfig is already set")
	}
}

func TestLoad_ErrorHandling(t *testing.T) {
	// Test SetupViper() when it fails due to invalid config file
	tmpDir := setupTestConfig(t)

	// Create invalid YAML config file
	invalidConfig := `api_url: https://test.api.com/v1
project_id: test-project
invalid yaml content [
`
	configFile := filepath.Join(tmpDir, ConfigFileName)
	if err := os.WriteFile(configFile, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	globalConfig = nil

	// SetupViper should fail with invalid config file
	err := SetupViper(configFile)
	if err == nil {
		t.Error("Expected SetupViper() to fail with invalid config file, but it succeeded")
	}
}

func TestGetConfigDir(t *testing.T) {
	// Test with TIGER_CONFIG_DIR environment variable
	os.Setenv("TIGER_CONFIG_DIR", "/custom/config/path")
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	dir := GetConfigDir()
	if dir != "/custom/config/path" {
		t.Errorf("Expected /custom/config/path, got %s", dir)
	}

	// Test with tilde expansion
	os.Setenv("TIGER_CONFIG_DIR", "~/tiger-config")
	dir = GetConfigDir()
	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, "tiger-config")
	if dir != expected {
		t.Errorf("Expected %s, got %s", expected, dir)
	}

	// Test default behavior
	os.Unsetenv("TIGER_CONFIG_DIR")
	dir = GetConfigDir()
	homeDir, _ = os.UserHomeDir()
	expected = filepath.Join(homeDir, ".config", "tiger")
	if dir != expected {
		t.Errorf("Expected %s, got %s", expected, dir)
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
			result := expandPath(tt.input)
			if result != tt.expected {
				t.Errorf("expandPath(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSave_CreateDirectory(t *testing.T) {
	tmpDir := setupTestConfig(t)

	// Use non-existent subdirectory
	configDir := filepath.Join(tmpDir, "nested", "config")

	cfg := &Config{
		APIURL:    "https://test.api.com/v1",
		ConfigDir: configDir,
	}

	err := cfg.Save()
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error("Config directory was not created")
	}

	// Verify config file was created
	configFile := filepath.Join(configDir, ConfigFileName)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}

func TestResetGlobalConfig(t *testing.T) {
	tmpDir := setupTestConfig(t)
	setupViper(t, tmpDir)

	// Set environment variable for test
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	// Load config to populate globalConfig
	cfg1, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify globalConfig is set
	if globalConfig == nil {
		t.Error("Expected globalConfig to be set after Load()")
	}

	// Reset global config
	ResetGlobalConfig()

	// Verify globalConfig is nil
	if globalConfig != nil {
		t.Error("Expected globalConfig to be nil after ResetGlobalConfig()")
	}

	// Load again should create new instance
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Second Load() failed: %v", err)
	}

	// Should be different instances since we reset
	if cfg1 == cfg2 {
		t.Error("Expected different config instances after reset, got same instance")
	}
}
