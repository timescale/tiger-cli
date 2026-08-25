package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

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
		{name: "search_docs", parameters: true, output: true},
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

	t.Run("Invalid capability name", func(t *testing.T) {
		rootCmd, _ := setupMCPTest(t)

		// Execute with invalid capability name
		_, err := executeCommand(t, rootCmd, []string{"mcp", "get", "nonexistent_capability"})
		assert.Error(t, err, "should error for nonexistent capability")
		assert.Contains(t, err.Error(), "not found", "error should mention capability not found")
	})

	t.Run("Valid tools", func(t *testing.T) {
		for _, tool := range expectedTools {
			t.Run(tool.name, func(t *testing.T) {
				t.Run("Table", func(t *testing.T) {
					rootCmd, _ := setupMCPTest(t)
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", tool.name})

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
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", tool.name, "-o", "json"})

					// Should be valid JSON
					var toolData map[string]any
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
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", tool.name, "-o", "yaml"})

					// Should be valid YAML
					var toolData map[string]any
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
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", prompt.name})

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
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", prompt.name, "-o", "json"})

					// Should be valid JSON
					var promptData map[string]any
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
					output := captureCommandOutput(t, rootCmd, []string{"mcp", "get", prompt.name, "-o", "yaml"})

					// Should be valid YAML
					var promptData map[string]any
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
