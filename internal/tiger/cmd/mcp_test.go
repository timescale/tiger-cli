package cmd

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/timescale/tiger-cli/internal/tiger/config"
)

func TestMCPListCommand(t *testing.T) {
	// Setup test environment for each subtest
	setupMCPListTest := func(t *testing.T) (*cobra.Command, string) {
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

	// Expected tools and prompts that should be present in all output formats
	expectedTools := []string{
		"db_execute_query",
		"semantic_search_postgres_docs",
		"semantic_search_tiger_docs",
		"service_create",
		"service_fork",
		"service_get",
		"service_list",
		"service_start",
		"service_stop",
		"service_update_password",
		"view_skill",
	}

	expectedPrompts := []string{
		"design-postgres-tables",
		"find-hypertable-candidates",
		"migrate-postgres-tables-to-hypertables",
		"setup-timescaledb-hypertables",
	}

	// Helper function to validate capabilities map structure and contents
	validateCapabilities := func(t *testing.T, capabilities map[string]interface{}) {
		t.Helper()

		// Should have the expected structure
		assert.Contains(t, capabilities, "tools", "should contain tools")
		assert.Contains(t, capabilities, "prompts", "should contain prompts")
		assert.Contains(t, capabilities, "resources", "should contain resources")
		assert.Contains(t, capabilities, "resource_templates", "should contain resource_templates")

		// Verify tools is an array
		tools, ok := capabilities["tools"].([]interface{})
		require.True(t, ok, "tools should be an array")
		assert.NotEmpty(t, tools, "should have at least one tool")

		// Extract tool names
		var toolNames []string
		for _, toolItem := range tools {
			tool, ok := toolItem.(map[string]interface{})
			require.True(t, ok, "tool should be an object")
			assert.Contains(t, tool, "name", "tool should have name field")
			assert.Contains(t, tool, "description", "tool should have description field")
			toolNames = append(toolNames, tool["name"].(string))
		}

		// Check all expected tools are present
		for _, expectedTool := range expectedTools {
			assert.Contains(t, toolNames, expectedTool, "should contain %s tool", expectedTool)
		}

		// Verify prompts is an array
		prompts, ok := capabilities["prompts"].([]interface{})
		require.True(t, ok, "prompts should be an array")
		assert.NotEmpty(t, prompts, "should have at least one prompt")

		// Extract prompt names
		var promptNames []string
		for _, promptItem := range prompts {
			prompt, ok := promptItem.(map[string]interface{})
			require.True(t, ok, "prompt should be an object")
			assert.Contains(t, prompt, "name", "prompt should have name field")
			promptNames = append(promptNames, prompt["name"].(string))
		}

		// Check all expected prompts are present
		for _, expectedPrompt := range expectedPrompts {
			assert.Contains(t, promptNames, expectedPrompt, "should contain %s prompt", expectedPrompt)
		}
	}

	t.Run("lists capabilities in table format by default", func(t *testing.T) {
		rootCmd, _ := setupMCPListTest(t)

		// Execute the list command
		output := captureCommandOutput(t, rootCmd, []string{"mcp", "list"})
		lines := strings.Split(output, "\n")

		// Should contain table headers
		assert.Contains(t, output, "TYPE", "output should contain TYPE header")
		assert.Contains(t, output, "NAME", "output should contain NAME header")

		// Build expected lines with type and name
		expectedLines := make(map[string]string)
		for _, tool := range expectedTools {
			expectedLines[tool] = "tool"
		}
		for _, prompt := range expectedPrompts {
			expectedLines[prompt] = "prompt"
		}

		// Check each expected capability appears with correct type
		for name, expectedType := range expectedLines {
			if !slices.ContainsFunc(lines, func(line string) bool {
				return strings.Contains(line, expectedType) && strings.Contains(line, name)
			}) {
				t.Errorf("Output should contain line with type '%s' and name '%s', got: %s", expectedType, name, output)
			}
		}
	})

	t.Run("lists capabilities in JSON format", func(t *testing.T) {
		rootCmd, _ := setupMCPListTest(t)

		// Execute the list command with JSON output
		output := captureCommandOutput(t, rootCmd, []string{"mcp", "list", "-o", "json"})

		// Should be valid JSON
		var capabilities map[string]interface{}
		err := json.Unmarshal([]byte(output), &capabilities)
		require.NoError(t, err, "output should be valid JSON")

		// Validate structure and contents
		validateCapabilities(t, capabilities)
	})

	t.Run("lists capabilities in YAML format", func(t *testing.T) {
		rootCmd, _ := setupMCPListTest(t)

		// Execute the list command with YAML output
		output := captureCommandOutput(t, rootCmd, []string{"mcp", "list", "-o", "yaml"})

		// Should be valid YAML
		var capabilities map[string]interface{}
		err := yaml.Unmarshal([]byte(output), &capabilities)
		require.NoError(t, err, "output should be valid YAML")

		// Validate structure and contents
		validateCapabilities(t, capabilities)
	})

	t.Run("handles invalid output format", func(t *testing.T) {
		rootCmd, _ := setupMCPListTest(t)

		// Execute with invalid output format should fail
		_, err := executeCommand(t, rootCmd, []string{"mcp", "list", "-o", "invalid"})
		assert.Error(t, err, "should error for invalid output format")
	})
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
