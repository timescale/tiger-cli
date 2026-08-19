package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/timescale/tiger-cli/internal/api"
	"github.com/timescale/tiger-cli/internal/config"
)

func setupAuthTest(t *testing.T) string {
	t.Helper()

	// Use a unique service name for this test to avoid conflicts
	config.SetTestServiceName(t)

	// Mock the API key validation for testing
	originalValidator := validateAPIKey
	validateAPIKey = func(ctx context.Context, cfg *config.Config, client api.ClientWithResponsesInterface) (*api.AuthInfo, error) {
		authInfo := &api.AuthInfo{}
		json.Unmarshal([]byte(`{"type":"apiKey","api_key":{"public_key":"test-access-key","project":{"id":"test-project-id"}}}`), authInfo)
		return authInfo, nil
	}

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-auth-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set TIGER_CONFIG_DIR environment variable so that commands executed by
	// the test load their config from the test directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Disable analytics for auth tests to avoid tracking test events
	os.Setenv("TIGER_ANALYTICS", "false")

	// Write an empty config file in the test directory
	if _, err := config.UseTestConfig(tmpDir, map[string]any{}); err != nil {
		t.Fatalf("Failed to use test config: %v", err)
	}

	// Clean up any existing test credentials
	testConfig(t).RemoveCredentials()

	t.Cleanup(func() {
		// Clean up test credentials
		testConfig(t).RemoveCredentials()
		validateAPIKey = originalValidator // Restore original validator
		// Remove config file explicitly
		configFile := config.GetConfigFile(tmpDir)
		os.Remove(configFile)
		// Clean up environment variables BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_ANALYTICS")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func executeAuthCommand(ctx context.Context, args ...string) (string, error) {
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
