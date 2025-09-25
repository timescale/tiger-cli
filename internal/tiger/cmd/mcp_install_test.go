package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stacklok/toolhive/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testClientMapping pairs our Tiger client types with their corresponding toolhive types for testing
type testClientMapping struct {
	TigerClientType    TigerMCPClient
	ToolhiveClientType client.MCPClient
}

// testClientMappings defines which clients we want to test for equivalence between ConfigPaths and toolhive
var testClientMappings = []testClientMapping{
	{
		TigerClientType:    TigerClaudeCode,
		ToolhiveClientType: client.ClaudeCode,
	},
	{
		TigerClientType:    TigerCursor,
		ToolhiveClientType: client.Cursor,
	},
	{
		TigerClientType:    TigerWindsurf,
		ToolhiveClientType: client.Windsurf,
	},
}

func TestFindClientConfigFileFallback(t *testing.T) {
	// Create temporary home directory for controlled testing
	tempHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Setenv("HOME", tempHome)
	defer func() {
		os.Setenv("HOME", originalHome)
	}()

	for _, cfg := range supportedClients {
		// Skip clients without ConfigPaths defined
		if len(cfg.ConfigPaths) == 0 {
			continue
		}

		t.Run(cfg.Name+" fallback when no file exists", func(t *testing.T) {
			// Test our ConfigPaths approach - this should succeed with fallback path
			ourPath, err := findClientConfigFile(cfg.ConfigPaths)
			require.NoError(t, err, "findClientConfigFile should not error")

			// Verify our path matches the expected fallback (last path in ConfigPaths)
			expectedPath := expandPath(cfg.ConfigPaths[len(cfg.ConfigPaths)-1])
			ourAbsPath, err := filepath.Abs(ourPath)
			require.NoError(t, err, "should be able to get absolute path for our result")
			expectedAbsPath, err := filepath.Abs(expectedPath)
			require.NoError(t, err, "should be able to get absolute path for expected result")

			assert.Equal(t, expectedAbsPath, ourAbsPath,
				"findClientConfigFile should return expected fallback path for %s", cfg.Name)
		})
	}
}

func TestFindClientConfigFileEquivalentToToolhive(t *testing.T) {
	// Test that our ConfigPaths system produces identical results to toolhive when config files exist
	tempHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Setenv("HOME", tempHome)
	defer func() {
		os.Setenv("HOME", originalHome)
	}()

	for _, mapping := range testClientMappings {
		// Find our client config
		var ourClientConfig *clientConfig
		for _, cfg := range supportedClients {
			if cfg.TigerClientType == mapping.TigerClientType {
				ourClientConfig = &cfg
				break
			}
		}
		require.NotNil(t, ourClientConfig, "should find client config for %s", mapping.TigerClientType)
		require.NotEmpty(t, ourClientConfig.ConfigPaths, "client should have ConfigPaths defined for %s", mapping.TigerClientType)

		t.Run(ourClientConfig.Name+" equivalent to toolhive when file exists", func(t *testing.T) {
			// Create the config file at the first ConfigPath location
			expandedPath := expandPath(ourClientConfig.ConfigPaths[0])

			// Create directory structure
			dir := filepath.Dir(expandedPath)
			err := os.MkdirAll(dir, 0755)
			require.NoError(t, err, "should be able to create directory structure")

			// Create the config file
			err = os.WriteFile(expandedPath, []byte(`{"mcpServers":{}}`), 0644)
			require.NoError(t, err, "should be able to create config file")

			// Test our ConfigPaths approach
			ourPath, err := findClientConfigFile(ourClientConfig.ConfigPaths)
			require.NoError(t, err, "findClientConfigFile should not error")

			// Test toolhive approach (should succeed now that file exists)
			toolhiveConfig, err := client.FindClientConfig(mapping.ToolhiveClientType)
			require.NoError(t, err, "toolhive FindClientConfig should not error when file exists")

			// Convert both paths to absolute paths for comparison
			ourAbsPath, err := filepath.Abs(ourPath)
			require.NoError(t, err, "should be able to get absolute path for our result")

			toolhiveAbsPath, err := filepath.Abs(toolhiveConfig.Path)
			require.NoError(t, err, "should be able to get absolute path for toolhive result")

			// Both systems should find the same existing file
			assert.Equal(t, ourAbsPath, toolhiveAbsPath,
				"findClientConfigFile and toolhive should find same existing file for %s", ourClientConfig.Name)
		})
	}
}