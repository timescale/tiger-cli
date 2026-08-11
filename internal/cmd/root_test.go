package cmd

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"github.com/timescale/tiger-cli/internal/config"
)

// loadEffectiveConfig runs the given args, then returns the config as the
// executed command resolved it: cobra parses flags into the leaf command's flag
// set, which is what commands hand to config.Load.
func loadEffectiveConfig(t *testing.T, args ...string) *config.Config {
	t.Helper()

	testCmd, err := buildRootCmd(t.Context())
	if err != nil {
		t.Fatalf("Failed to build root command: %v", err)
	}
	testCmd.SetArgs(args)
	if err := testCmd.Execute(); err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	executed, _, err := testCmd.Find(args)
	if err != nil {
		t.Fatalf("Failed to find executed command: %v", err)
	}
	cfg, err := config.Load(executed.Flags())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	return cfg
}

// The context passed to buildRootCmd must reach the command that runs, so
// handlers can rely on cmd.Context() for cancellation. It's set on the root at
// build time (cobra copies it onto the executed command) rather than in a
// PersistentPreRunE hook.
func TestContextReachesCommand(t *testing.T) {
	setupTestCommand(t)

	type ctxKey struct{}
	ctx := context.WithValue(t.Context(), ctxKey{}, "from-execute")

	rootCmd, err := buildRootCmd(ctx)
	if err != nil {
		t.Fatalf("Failed to build root command: %v", err)
	}

	var got any
	versionCmd, _, err := rootCmd.Find([]string{"version"})
	if err != nil {
		t.Fatalf("Failed to find version command: %v", err)
	}
	inner := versionCmd.RunE
	versionCmd.RunE = func(c *cobra.Command, args []string) error {
		got = c.Context().Value(ctxKey{})
		return inner(c, args)
	}

	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	if got != "from-execute" {
		t.Errorf("Expected the command to run with the context passed to buildRootCmd, got value %v", got)
	}
}

func writeTestConfigFile(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.WriteFile(config.GetConfigFile(dir), []byte(contents), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
}

func TestFlagPrecedence(t *testing.T) {
	tmpDir, _ := setupTestCommand(t)

	// Create config file with some values
	writeTestConfigFile(t, tmpDir, `api_url: https://file.api.com/v1
service_id: file-service
output: table
analytics: true
`)

	// Set environment variables
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_SERVICE_ID", "env-service")
	os.Setenv("TIGER_OUTPUT", "json")
	os.Setenv("TIGER_ANALYTICS", "false")

	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_SERVICE_ID")
		os.Unsetenv("TIGER_OUTPUT")
		os.Unsetenv("TIGER_ANALYTICS")
	}()

	// CLI flags take precedence over both env vars and the config file
	cfg := loadEffectiveConfig(t,
		"--config-dir", tmpDir,
		"--service-id", "flag-service",
		"--analytics=false",
		"version", // Need a subcommand to execute
	)

	if cfg.ServiceID != "flag-service" {
		t.Errorf("Expected service_id 'flag-service', got '%s'", cfg.ServiceID)
	}
	if cfg.ConfigDir != tmpDir {
		t.Errorf("Expected config dir '%s' from flag, got '%s'", tmpDir, cfg.ConfigDir)
	}
	// Env var wins where no flag was given
	if cfg.Output != "json" {
		t.Errorf("Expected output 'json' from env var, got '%s'", cfg.Output)
	}
	// Config file wins where neither a flag nor an env var was given
	if cfg.APIURL != "https://file.api.com/v1" {
		t.Errorf("Expected api_url from config file, got '%s'", cfg.APIURL)
	}
}

func TestFlagOverridesEnvVar(t *testing.T) {
	tmpDir, _ := setupTestCommand(t)

	// Set environment variable
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	os.Setenv("TIGER_SERVICE_ID", "test-service-1")

	defer func() {
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_SERVICE_ID")
	}()

	// Test 1: Environment variable should be used when no flag is set
	cfg := loadEffectiveConfig(t, "version")
	if cfg.ServiceID != "test-service-1" {
		t.Errorf("Expected service_id 'test-service-1' from env var, got '%s'", cfg.ServiceID)
	}

	// Test 2: Flag should override environment variable
	cfg = loadEffectiveConfig(t, "--service-id", "test-service-2", "version")
	if cfg.ServiceID != "test-service-2" {
		t.Errorf("Expected service_id 'test-service-2' from flag, got '%s'", cfg.ServiceID)
	}
}

func TestConfigFilePrecedence(t *testing.T) {
	tmpDir, _ := setupTestCommand(t)

	// Create config file
	writeTestConfigFile(t, tmpDir, `output: json
analytics: false
`)

	// Set environment that should be overridden by config file
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	defer os.Unsetenv("TIGER_CONFIG_DIR")

	// Values should come from config file since no flags were set
	cfg := loadEffectiveConfig(t, "--config-dir", tmpDir, "version")
	if cfg.Output != "json" {
		t.Errorf("Expected output 'json' from config file, got '%s'", cfg.Output)
	}
	if cfg.Analytics != false {
		t.Errorf("Expected analytics false from config file, got %t", cfg.Analytics)
	}
}

// Only the flags a command actually defines are bound, so a command without an
// --output flag still resolves output from the env and config file.
func TestFlagBindingIsPerCommand(t *testing.T) {
	tmpDir, _ := setupTestCommand(t)

	writeTestConfigFile(t, tmpDir, "output: yaml\n")

	os.Setenv("TIGER_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("TIGER_CONFIG_DIR")

	rootCmd, err := buildRootCmd(t.Context())
	if err != nil {
		t.Fatalf("Failed to build root command: %v", err)
	}
	noOutputCmd, _, err := rootCmd.Find([]string{"config", "unset"})
	if err != nil {
		t.Fatalf("Failed to find command: %v", err)
	}
	if noOutputCmd.Flags().Lookup("output") != nil {
		t.Fatal("Expected `config unset` to have no --output flag")
	}

	cfg, err := config.Load(noOutputCmd.Flags())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if cfg.Output != "yaml" {
		t.Errorf("Expected output 'yaml' from config file, got '%s'", cfg.Output)
	}
}
