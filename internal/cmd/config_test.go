package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/timescale/tiger-cli/internal/config"
)

func setupConfigTest(t *testing.T) (string, func()) {
	t.Helper()

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set environment variable to use test directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Disable analytics for config tests to avoid tracking test events
	os.Setenv("TIGER_ANALYTICS", "false")

	config.UseTestConfig(tmpDir, map[string]any{})

	// Clean up function
	cleanup := func() {
		os.RemoveAll(tmpDir)
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_ANALYTICS")

		// Reset global config in the config package
		// This is important for test isolation
		// We need to clear the singleton
	}

	t.Cleanup(cleanup)

	return tmpDir, cleanup
}

func executeConfigCommand(ctx context.Context, args ...string) (string, error) {
	// Use buildRootCmd() to get a complete root command with all flags and subcommands
	testRoot, err := buildRootCmd(ctx)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	testRoot.SetOut(buf)
	testRoot.SetErr(buf)
	testRoot.SetArgs(args)

	err = testRoot.Execute()
	return buf.String(), err
}

func TestConfigCommands_Integration(t *testing.T) {
	_, _ = setupConfigTest(t)

	// Test full workflow: set -> show -> unset -> reset

	// 1. Set some values
	_, err := executeConfigCommand(t.Context(), "config", "set", "service_id", "integration-test")
	if err != nil {
		t.Fatalf("Failed to set service_id: %v", err)
	}

	_, err = executeConfigCommand(t.Context(), "config", "set", "output", "json")
	if err != nil {
		t.Fatalf("Failed to set output: %v", err)
	}

	// 2. Show config in JSON format (should use the output format we just set)
	showOutput, err := executeConfigCommand(t.Context(), "config", "show")
	if err != nil {
		t.Fatalf("Failed to show config: %v", err)
	}

	// Should be JSON output
	var result map[string]any
	if err := json.Unmarshal([]byte(showOutput), &result); err != nil {
		t.Fatalf("Expected JSON output, got: %s", showOutput)
	}

	if result["service_id"] != "integration-test" {
		t.Errorf("Expected service_id 'integration-test', got %v", result["service_id"])
	}

	// 3. Unset service_id
	_, err = executeConfigCommand(t.Context(), "config", "unset", "service_id")
	if err != nil {
		t.Fatalf("Failed to unset service_id: %v", err)
	}

	// 4. Verify service_id was unset
	showOutput, err = executeConfigCommand(t.Context(), "config", "show")
	if err != nil {
		t.Fatalf("Failed to show config after unset: %v", err)
	}

	result = make(map[string]any)
	json.Unmarshal([]byte(showOutput), &result)
	if result["service_id"] != "" {
		t.Errorf("Expected empty service_id after unset, got %v", result["service_id"])
	}

	// 5. Reset all config
	_, err = executeConfigCommand(t.Context(), "config", "reset")
	if err != nil {
		t.Fatalf("Failed to reset config: %v", err)
	}

	// 6. Verify everything is back to defaults
	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("Failed to load config after reset: %v", err)
	}

	if cfg.Output != config.DefaultOutput {
		t.Errorf("Expected output reset to default %s, got %s", config.DefaultOutput, cfg.Output)
	}
}
