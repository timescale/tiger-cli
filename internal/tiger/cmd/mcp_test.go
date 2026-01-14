package cmd

import (
	"encoding/json"
	"fmt"
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

func TestMCPListCommand(t *testing.T) {

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

	t.Run("Table format", func(t *testing.T) {
		rootCmd, _ := setupMCPTest(t)

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

	t.Run("JSON format", func(t *testing.T) {
		rootCmd, _ := setupMCPTest(t)

		// Execute the list command with JSON output
		output := captureCommandOutput(t, rootCmd, []string{"mcp", "list", "-o", "json"})

		// Should be valid JSON
		var capabilities map[string]interface{}
		err := json.Unmarshal([]byte(output), &capabilities)
		require.NoError(t, err, "output should be valid JSON")

		// Validate structure and contents
		validateCapabilities(t, capabilities)
	})

	t.Run("YAML format", func(t *testing.T) {
		rootCmd, _ := setupMCPTest(t)

		// Execute the list command with YAML output
		output := captureCommandOutput(t, rootCmd, []string{"mcp", "list", "-o", "yaml"})

		// Should be valid YAML
		var capabilities map[string]interface{}
		err := yaml.Unmarshal([]byte(output), &capabilities)
		require.NoError(t, err, "output should be valid YAML")

		// Validate structure and contents
		validateCapabilities(t, capabilities)
	})

	t.Run("Invalid output format", func(t *testing.T) {
		rootCmd, _ := setupMCPTest(t)

		// Execute with invalid output format should fail
		_, err := executeCommand(t, rootCmd, []string{"mcp", "list", "-o", "invalid"})
		assert.Error(t, err, "should error for invalid output format")
	})
}

func TestMCPGetCommand(t *testing.T) {
	// toolExpectation defines what sections we expect for each tool
	type toolExpectation struct {
		name       string
		parameters bool
		output     bool
	}

	// promptExpectation defines what sections we expect for each prompt
	type promptExpectation struct {
		name      string
		arguments bool
	}

	// Expected tools with their section expectations
	expectedTools := []toolExpectation{
		{name: "db_execute_query", parameters: true, output: true},
		{name: "semantic_search_postgres_docs", parameters: true, output: true},
		{name: "semantic_search_tiger_docs", parameters: true, output: true},
		{name: "service_create", parameters: true, output: true},
		{name: "service_fork", parameters: true, output: true},
		{name: "service_get", parameters: true, output: true},
		{name: "service_list", parameters: false, output: true},
		{name: "service_start", parameters: true, output: true},
		{name: "service_stop", parameters: true, output: true},
		{name: "service_update_password", parameters: true, output: true},
		{name: "view_skill", parameters: true, output: true},
	}

	// Expected prompts with their section expectations
	expectedPrompts := []promptExpectation{
		{name: "design-postgres-tables", arguments: false},
		{name: "find-hypertable-candidates", arguments: false},
		{name: "migrate-postgres-tables-to-hypertables", arguments: false},
		{name: "setup-timescaledb-hypertables", arguments: false},
	}

	t.Run("Invalid capability type", func(t *testing.T) {
		rootCmd, _ := setupMCPTest(t)

		// Execute with invalid capability type
		_, err := executeCommand(t, rootCmd, []string{"mcp", "get", "invalid_type", "some_name"})
		assert.Error(t, err, "should error for invalid capability type")
		assert.Contains(t, err.Error(), "invalid capability type", "error should mention invalid capability type")
	})

	t.Run("Invalid tool name", func(t *testing.T) {
		rootCmd, _ := setupMCPTest(t)

		// Execute with valid type but invalid name
		_, err := executeCommand(t, rootCmd, []string{"mcp", "get", "tool", "nonexistent_tool"})
		assert.Error(t, err, "should error for nonexistent tool")
		assert.Contains(t, err.Error(), "not found", "error should mention tool not found")
	})

	t.Run("Invalid prompt name", func(t *testing.T) {
		rootCmd, _ := setupMCPTest(t)

		// Execute with valid type but invalid name
		_, err := executeCommand(t, rootCmd, []string{"mcp", "get", "prompt", "nonexistent-prompt"})
		assert.Error(t, err, "should error for nonexistent prompt")
		assert.Contains(t, err.Error(), "not found", "error should mention prompt not found")
	})

	t.Run("Valid tools", func(t *testing.T) {
		for _, tool := range expectedTools {
			t.Run(tool.name, func(t *testing.T) {
				t.Run("Table", func(t *testing.T) {
					rootCmd, _ := setupMCPTest(t)
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", "tool", tool.name})

					lines := strings.Split(output, "\n")
					require.NotEmpty(t, lines, "output should not be empty")

					// Check for tool name line
					assert.Contains(t, output, fmt.Sprintf("Tool name: %s", tool.name), "output should contain tool name line")

					// Check for description section
					assert.Contains(t, output, "Description:", "output should contain 'Description:' section")

					// Check for parameters section if expected
					if tool.parameters {
						assert.Contains(t, output, "Parameters:", "output should contain 'Parameters:' section")
					}

					// Check for output section if expected
					if tool.output {
						assert.Contains(t, output, "Output:", "output should contain 'Output:' section")
					}
				})

				t.Run("JSON", func(t *testing.T) {
					rootCmd, _ := setupMCPTest(t)
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", "tool", tool.name, "-o", "json"})

					// Should be valid JSON
					var toolData map[string]interface{}
					err := json.Unmarshal([]byte(output), &toolData)
					require.NoError(t, err, "output should be valid JSON")

					// Check for all expected top-level fields
					assert.Contains(t, toolData, "name", "tool should have name field")
					assert.Contains(t, toolData, "description", "tool should have description field")
					assert.Contains(t, toolData, "title", "tool should have title field")
					assert.Contains(t, toolData, "annotations", "tool should have annotations field")
					assert.Contains(t, toolData, "inputSchema", "tool should have inputSchema field")
					assert.Contains(t, toolData, "outputSchema", "tool should have outputSchema field")
					assert.Equal(t, tool.name, toolData["name"], "tool name should match")
				})

				t.Run("YAML", func(t *testing.T) {
					rootCmd, _ := setupMCPTest(t)
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", "tool", tool.name, "-o", "yaml"})

					// Should be valid YAML
					var toolData map[string]interface{}
					err := yaml.Unmarshal([]byte(output), &toolData)
					require.NoError(t, err, "output should be valid YAML")

					// Check for all expected top-level fields
					assert.Contains(t, toolData, "name", "tool should have name field")
					assert.Contains(t, toolData, "description", "tool should have description field")
					assert.Contains(t, toolData, "title", "tool should have title field")
					assert.Contains(t, toolData, "annotations", "tool should have annotations field")
					assert.Contains(t, toolData, "inputSchema", "tool should have inputSchema field")
					assert.Contains(t, toolData, "outputSchema", "tool should have outputSchema field")
					assert.Equal(t, tool.name, toolData["name"], "tool name should match")
				})
			})
		}
	})

	t.Run("Valid prompts", func(t *testing.T) {
		for _, prompt := range expectedPrompts {
			t.Run(prompt.name, func(t *testing.T) {
				t.Run("Table", func(t *testing.T) {
					rootCmd, _ := setupMCPTest(t)
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", "prompt", prompt.name})

					lines := strings.Split(output, "\n")
					require.NotEmpty(t, lines, "output should not be empty")

					// Check for prompt name line
					assert.Contains(t, output, fmt.Sprintf("Prompt name: %s", prompt.name), "output should contain prompt name line")

					// Check for description section
					assert.Contains(t, output, "Description:", "output should contain 'Description:' section")

					// Check for arguments section if expected
					if prompt.arguments {
						assert.Contains(t, output, "Arguments:", "output should contain 'Arguments:' section")
					}
				})

				t.Run("JSON", func(t *testing.T) {
					rootCmd, _ := setupMCPTest(t)
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", "prompt", prompt.name, "-o", "json"})

					// Should be valid JSON
					var promptData map[string]interface{}
					err := json.Unmarshal([]byte(output), &promptData)
					require.NoError(t, err, "output should be valid JSON")

					// Check for all expected top-level fields
					assert.Contains(t, promptData, "name", "prompt should have name field")
					assert.Contains(t, promptData, "description", "prompt should have description field")
					assert.Contains(t, promptData, "title", "prompt should have title field")
					assert.Equal(t, prompt.name, promptData["name"], "prompt name should match")

					// Check for arguments field if expected
					if prompt.arguments {
						assert.Contains(t, promptData, "arguments", "prompt should have arguments field")
					}
				})

				t.Run("YAML", func(t *testing.T) {
					rootCmd, _ := setupMCPTest(t)
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", "prompt", prompt.name, "-o", "yaml"})

					// Should be valid YAML
					var promptData map[string]interface{}
					err := yaml.Unmarshal([]byte(output), &promptData)
					require.NoError(t, err, "output should be valid YAML")

					// Check for all expected top-level fields
					assert.Contains(t, promptData, "name", "prompt should have name field")
					assert.Contains(t, promptData, "description", "prompt should have description field")
					assert.Contains(t, promptData, "title", "prompt should have title field")
					assert.Equal(t, prompt.name, promptData["name"], "prompt name should match")

					// Check for arguments field if expected
					if prompt.arguments {
						assert.Contains(t, promptData, "arguments", "prompt should have arguments field")
					}
				})
			})
		}
	})
}
