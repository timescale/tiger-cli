package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/timescale/tiger-cli/internal/config"
)

// setupMCPTest sets up a test environment for MCP command tests.
// Returns the root command and temporary directory path.
func setupMCPTest(t *testing.T) (*cobra.Command, string) {
	t.Helper()

	// Use a unique service name for this test to avoid keyring conflicts
	setupTestCommand(t)

	// Create temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "tiger-mcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set temporary config directory
	os.Setenv("TIGER_CONFIG_DIR", tmpDir)

	// Disable analytics for tests
	os.Setenv("TIGER_ANALYTICS", "false")

	// Reset global config and viper to ensure test isolation
	config.ResetGlobalConfig()

	t.Cleanup(func() {
		// Reset global config and viper first
		config.ResetGlobalConfig()
		// Clean up environment variables BEFORE cleaning up file system
		os.Unsetenv("TIGER_CONFIG_DIR")
		os.Unsetenv("TIGER_ANALYTICS")
		// Then clean up file system
		os.RemoveAll(tmpDir)
	})

	rootCmd, err := buildRootCmd(t.Context())
	require.NoError(t, err, "should build root command")

	return rootCmd, tmpDir
}

// executeCommand executes a command and returns both output and error
func executeCommand(t *testing.T, rootCmd *cobra.Command, args []string) (string, error) {
	t.Helper()

	var buf strings.Builder
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(args)

	err := rootCmd.Execute()
	return buf.String(), err
}

// captureCommandOutput executes a command and returns its output, failing the test if there's an error
func captureCommandOutput(t *testing.T, rootCmd *cobra.Command, args []string) string {
	t.Helper()

	output, err := executeCommand(t, rootCmd, args)
	require.NoError(t, err, "command should execute successfully")

	return output
}
