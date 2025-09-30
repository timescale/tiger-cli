package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stacklok/toolhive/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tailscale/hujson"
)

// testClientMapping pairs our Tiger client types with their corresponding toolhive types for testing
type testClientMapping struct {
	ClientType         MCPClient
	ToolhiveClientType client.MCPClient
}

// testClientMappings defines which clients we want to test for equivalence between ConfigPaths and toolhive
var testClientMappings = []testClientMapping{
	{
		ClientType:         ClaudeCode,
		ToolhiveClientType: client.ClaudeCode,
	},
	{
		ClientType:         Cursor,
		ToolhiveClientType: client.Cursor,
	},
	{
		ClientType:         Windsurf,
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
			if cfg.ClientType == mapping.ClientType {
				ourClientConfig = &cfg
				break
			}
		}
		require.NotNil(t, ourClientConfig, "should find client config for %s", mapping.ClientType)
		require.NotEmpty(t, ourClientConfig.ConfigPaths, "client should have ConfigPaths defined for %s", mapping.ClientType)

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

func TestAddTigerMCPServer(t *testing.T) {
	// Override getTigerExecutablePath to return "tiger" for tests
	oldFunc := tigerExecutablePathFunc
	tigerExecutablePathFunc = func() (string, error) {
		return "tiger", nil
	}
	defer func() {
		tigerExecutablePathFunc = oldFunc
	}()

	tests := []struct {
		name                 string
		initialConfig        string
		mcpServersPathPrefix string
		expectedResult       map[string]interface{}
		expectError          bool
	}{
		{
			name:                 "empty config file",
			initialConfig:        `{}`,
			mcpServersPathPrefix: "/mcpServers",
			expectedResult: map[string]interface{}{
				"mcpServers": map[string]interface{}{
					"tigerdata": map[string]interface{}{
						"command": "tiger",
						"args":    []interface{}{"mcp", "start"},
					},
				},
			},
			expectError: false,
		},
		{
			name:                 "config with existing mcpServers",
			initialConfig:        `{"mcpServers": {"existing": {"command": "existing", "args": ["test"]}}}`,
			mcpServersPathPrefix: "/mcpServers",
			expectedResult: map[string]interface{}{
				"mcpServers": map[string]interface{}{
					"existing": map[string]interface{}{
						"command": "existing",
						"args":    []interface{}{"test"},
					},
					"tigerdata": map[string]interface{}{
						"command": "tiger",
						"args":    []interface{}{"mcp", "start"},
					},
				},
			},
			expectError: false,
		},
		{
			name:                 "config without mcpServers section",
			initialConfig:        `{"other": "config"}`,
			mcpServersPathPrefix: "/mcpServers",
			expectedResult: map[string]interface{}{
				"other": "config",
				"mcpServers": map[string]interface{}{
					"tigerdata": map[string]interface{}{
						"command": "tiger",
						"args":    []interface{}{"mcp", "start"},
					},
				},
			},
			expectError: false,
		},
		{
			name:                 "different path prefix",
			initialConfig:        `{}`,
			mcpServersPathPrefix: "/servers",
			expectedResult: map[string]interface{}{
				"servers": map[string]interface{}{
					"tigerdata": map[string]interface{}{
						"command": "tiger",
						"args":    []interface{}{"mcp", "start"},
					},
				},
			},
			expectError: false,
		},
		{
			name:                 "invalid JSON",
			initialConfig:        `{invalid json`,
			mcpServersPathPrefix: "/mcpServers",
			expectedResult:       nil,
			expectError:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory and config file
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.json")

			// Write initial config
			err := os.WriteFile(configPath, []byte(tt.initialConfig), 0644)
			require.NoError(t, err)

			// Call the function under test
			err = addTigerMCPServerViaJSON(configPath, tt.mcpServersPathPrefix)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Read the result
			resultBytes, err := os.ReadFile(configPath)
			require.NoError(t, err)

			// Parse the result
			var result map[string]interface{}
			err = json.Unmarshal(resultBytes, &result)
			require.NoError(t, err)

			// Compare with expected result
			if tt.expectedResult != nil {
				assert.Equal(t, tt.expectedResult, result)
			}

			// Verify the file is valid JSON
			assert.True(t, json.Valid(resultBytes), "Result should be valid JSON")
		})
	}
}

func TestAddTigerMCPServerFileOperations(t *testing.T) {
	// Override getTigerExecutablePath to return "tiger" for tests
	oldFunc := tigerExecutablePathFunc
	tigerExecutablePathFunc = func() (string, error) {
		return "tiger", nil
	}
	defer func() {
		tigerExecutablePathFunc = oldFunc
	}()

	t.Run("creates directory if it doesn't exist", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "nested", "dir", "config.json")

		// Directory doesn't exist yet
		_, err := os.Stat(filepath.Dir(configPath))
		assert.True(t, os.IsNotExist(err))

		err = addTigerMCPServerViaJSON(configPath, "/mcpServers")
		require.NoError(t, err)

		// Directory should now exist
		_, err = os.Stat(filepath.Dir(configPath))
		assert.NoError(t, err)

		// Config file should exist
		_, err = os.Stat(configPath)
		assert.NoError(t, err)
	})

	t.Run("handles non-existent config file", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "nonexistent.json")

		err := addTigerMCPServerViaJSON(configPath, "/mcpServers")
		require.NoError(t, err)

		// File should now exist with correct content
		resultBytes, err := os.ReadFile(configPath)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(resultBytes, &result)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"tigerdata": map[string]interface{}{
					"command": "tiger",
					"args":    []interface{}{"mcp", "start"},
				},
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("handles empty config file", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "empty.json")

		// Create empty file
		err := os.WriteFile(configPath, []byte(""), 0644)
		require.NoError(t, err)

		err = addTigerMCPServerViaJSON(configPath, "/mcpServers")
		require.NoError(t, err)

		// File should now have correct content
		resultBytes, err := os.ReadFile(configPath)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(resultBytes, &result)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"tigerdata": map[string]interface{}{
					"command": "tiger",
					"args":    []interface{}{"mcp", "start"},
				},
			},
		}
		assert.Equal(t, expected, result)
	})
}

func TestCreateConfigBackup(t *testing.T) {
	t.Run("creates backup for existing config file", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.json")
		originalContent := `{"mcpServers": {"test": {"command": "test", "args": ["arg1"]}}}`

		// Create original config file
		err := os.WriteFile(configPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Create backup
		backupPath, err := createConfigBackup(configPath)
		require.NoError(t, err)
		require.NotEmpty(t, backupPath, "backup path should not be empty")

		// Verify backup path format
		expectedPrefix := configPath + ".backup."
		assert.True(t, strings.HasPrefix(backupPath, expectedPrefix), "backup path should have correct prefix")

		// Verify backup file exists
		_, err = os.Stat(backupPath)
		assert.NoError(t, err, "backup file should exist")

		// Verify backup content matches original
		backupContent, err := os.ReadFile(backupPath)
		require.NoError(t, err)
		assert.Equal(t, originalContent, string(backupContent), "backup content should match original")

		// Verify original file is unchanged
		originalAfterBackup, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Equal(t, originalContent, string(originalAfterBackup), "original file should be unchanged")
	})

	t.Run("returns empty string for non-existent config file", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "nonexistent.json")

		// Config file doesn't exist
		_, err := os.Stat(configPath)
		assert.True(t, os.IsNotExist(err), "config file should not exist")

		// Create backup should return empty string and no error
		backupPath, err := createConfigBackup(configPath)
		require.NoError(t, err)
		assert.Empty(t, backupPath, "backup path should be empty for non-existent file")
	})

	t.Run("creates backup with unique timestamp", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.json")
		originalContent := `{"test": "data"}`

		// Create original config file
		err := os.WriteFile(configPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Create first backup
		backupPath1, err := createConfigBackup(configPath)
		require.NoError(t, err)
		require.NotEmpty(t, backupPath1)

		// Wait a moment to ensure different timestamp
		time.Sleep(time.Second + 10*time.Millisecond)

		// Create second backup
		backupPath2, err := createConfigBackup(configPath)
		require.NoError(t, err)
		require.NotEmpty(t, backupPath2)

		// Backup paths should be different
		assert.NotEqual(t, backupPath1, backupPath2, "backup paths should have different timestamps")

		// Both backup files should exist
		_, err = os.Stat(backupPath1)
		assert.NoError(t, err, "first backup should exist")
		_, err = os.Stat(backupPath2)
		assert.NoError(t, err, "second backup should exist")
	})

	t.Run("handles empty config file", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "empty.json")

		// Create empty config file
		err := os.WriteFile(configPath, []byte(""), 0644)
		require.NoError(t, err)

		// Create backup
		backupPath, err := createConfigBackup(configPath)
		require.NoError(t, err)
		require.NotEmpty(t, backupPath)

		// Verify backup exists and is empty
		backupContent, err := os.ReadFile(backupPath)
		require.NoError(t, err)
		assert.Empty(t, backupContent, "backup of empty file should be empty")
	})

	t.Run("preserves file permissions", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.json")
		originalContent := `{"test": "data"}`

		// Create original config file with specific permissions
		err := os.WriteFile(configPath, []byte(originalContent), 0600)
		require.NoError(t, err)

		// Create backup
		backupPath, err := createConfigBackup(configPath)
		require.NoError(t, err)
		require.NotEmpty(t, backupPath)

		// Check backup file permissions
		backupInfo, err := os.Stat(backupPath)
		require.NoError(t, err)

		// The backup is created with 0644 permissions (as specified in the function)
		expectedMode := os.FileMode(0644)
		assert.Equal(t, expectedMode, backupInfo.Mode().Perm(), "backup should have 0644 permissions")
	})

	t.Run("handles permission errors gracefully", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("Cannot test permission errors as root user")
		}

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.json")
		originalContent := `{"test": "data"}`

		// Create original config file
		err := os.WriteFile(configPath, []byte(originalContent), 0644)
		require.NoError(t, err)

		// Make the directory read-only to simulate permission error
		err = os.Chmod(tempDir, 0444)
		require.NoError(t, err)

		// Restore permissions after test
		defer func() {
			os.Chmod(tempDir, 0755)
		}()

		// Create backup should fail due to permission error
		backupPath, err := createConfigBackup(configPath)
		assert.Error(t, err, "should fail due to permission error")
		assert.Empty(t, backupPath, "backup path should be empty on error")
		assert.Contains(t, err.Error(), "failed to read original config file", "error should mention read failure")
	})
}

func TestExpandPath(t *testing.T) {
	// Get the actual home directory for comparison
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err, "should be able to get user home directory")

	t.Run("expands tilde to home directory", func(t *testing.T) {
		result := expandPath("~/config.json")
		expected := filepath.Join(homeDir, "config.json")
		assert.Equal(t, expected, result, "should expand tilde to home directory")
	})

	t.Run("expands tilde with subdirectory", func(t *testing.T) {
		result := expandPath("~/.config/tiger/config.json")
		expected := filepath.Join(homeDir, ".config/tiger/config.json")
		assert.Equal(t, expected, result, "should expand tilde with subdirectory path")
	})

	t.Run("does not modify paths without tilde", func(t *testing.T) {
		testPath := "/absolute/path/config.json"
		result := expandPath(testPath)
		assert.Equal(t, testPath, result, "should not modify absolute paths without tilde")
	})

	t.Run("does not modify relative paths without tilde", func(t *testing.T) {
		testPath := "relative/path/config.json"
		result := expandPath(testPath)
		assert.Equal(t, testPath, result, "should not modify relative paths without tilde")
	})

	t.Run("expands environment variables", func(t *testing.T) {
		// Set a test environment variable
		testEnvVar := "TEST_EXPAND_PATH_VAR"
		testValue := "/test/env/path"
		t.Setenv(testEnvVar, testValue)

		result := expandPath("$" + testEnvVar + "/config.json")
		expected := testValue + "/config.json"
		assert.Equal(t, expected, result, "should expand environment variables")
	})

	t.Run("expands environment variables with braces", func(t *testing.T) {
		testEnvVar := "TEST_EXPAND_PATH_BRACES"
		testValue := "/test/env/braces"
		t.Setenv(testEnvVar, testValue)

		result := expandPath("${" + testEnvVar + "}/config.json")
		expected := testValue + "/config.json"
		assert.Equal(t, expected, result, "should expand environment variables with braces")
	})

	t.Run("expands both environment variables and tilde", func(t *testing.T) {
		testEnvVar := "TEST_EXPAND_PATH_BOTH"
		testValue := "Documents"
		t.Setenv(testEnvVar, testValue)

		result := expandPath("~/$" + testEnvVar + "/config.json")
		expected := filepath.Join(homeDir, testValue, "config.json")
		assert.Equal(t, expected, result, "should expand both environment variables and tilde")
	})

	t.Run("handles undefined environment variables", func(t *testing.T) {
		result := expandPath("$UNDEFINED_ENV_VAR/config.json")
		// os.ExpandEnv replaces undefined variables with empty string
		expected := "/config.json"
		assert.Equal(t, expected, result, "should replace undefined env vars with empty string")
	})

	t.Run("handles tilde not at beginning", func(t *testing.T) {
		testPath := "/some/path/~/config.json"
		result := expandPath(testPath)
		// Should not expand tilde that's not at the beginning
		assert.Equal(t, testPath, result, "should not expand tilde that's not at path beginning")
	})

	t.Run("handles just tilde", func(t *testing.T) {
		result := expandPath("~")
		// Just tilde should not be expanded (needs to be ~/something)
		assert.Equal(t, "~", result, "should not expand bare tilde")
	})

	t.Run("handles tilde with just slash", func(t *testing.T) {
		result := expandPath("~/")
		expected := filepath.Join(homeDir, "")
		assert.Equal(t, expected, result, "should expand tilde with just slash")
	})

	t.Run("handles empty path", func(t *testing.T) {
		result := expandPath("")
		assert.Equal(t, "", result, "should handle empty path")
	})
}

func TestMapEditorToTigerClientType(t *testing.T) {
	t.Run("maps supported editors correctly", func(t *testing.T) {
		testCases := []struct {
			editorName   string
			expectedType MCPClient
		}{
			{"claude-code", ClaudeCode},
			{"cursor", Cursor},
			{"windsurf", Windsurf},
			{"codex", Codex},
		}

		for _, tc := range testCases {
			t.Run(tc.editorName, func(t *testing.T) {
				result, err := mapClientToTigerClientType(tc.editorName)
				require.NoError(t, err, "should not error for supported editor")
				assert.Equal(t, tc.expectedType, result, "should map to correct client type")
			})
		}
	})

	t.Run("handles case insensitive editor names", func(t *testing.T) {
		testCases := []struct {
			editorName   string
			expectedType MCPClient
		}{
			{"CLAUDE-CODE", ClaudeCode},
			{"CURSOR", Cursor},
			{"WindSurf", Windsurf},
			{"CODEX", Codex},
		}

		for _, tc := range testCases {
			t.Run(tc.editorName, func(t *testing.T) {
				result, err := mapClientToTigerClientType(tc.editorName)
				require.NoError(t, err, "should not error for supported editor regardless of case")
				assert.Equal(t, tc.expectedType, result, "should map to correct client type")
			})
		}
	})

	t.Run("returns error for unsupported editor", func(t *testing.T) {
		result, err := mapClientToTigerClientType("unsupported-editor")
		assert.Error(t, err, "should error for unsupported editor")
		assert.Empty(t, result, "should return empty client type")
		assert.Contains(t, err.Error(), "unsupported client: unsupported-editor", "error should mention the unsupported client")
		assert.Contains(t, err.Error(), "Supported clients:", "error should list supported clients")
		// Verify it includes some known supported editors
		assert.Contains(t, err.Error(), "claude-code", "error should include claude-code in supported list")
		assert.Contains(t, err.Error(), "cursor", "error should include cursor in supported list")
	})

	t.Run("handles empty editor name", func(t *testing.T) {
		result, err := mapClientToTigerClientType("")
		assert.Error(t, err, "should error for empty editor name")
		assert.Empty(t, result, "should return empty client type")
		assert.Contains(t, err.Error(), "unsupported client:", "error should mention unsupported client")
	})
}

func TestFindOurClientConfig(t *testing.T) {
	t.Run("finds client config for supported client types", func(t *testing.T) {
		testCases := []struct {
			clientType   MCPClient
			expectedName string
		}{
			{ClaudeCode, "Claude Code"},
			{Cursor, "Cursor"},
			{Windsurf, "Windsurf"},
			{Codex, "Codex"},
		}

		for _, tc := range testCases {
			t.Run(string(tc.clientType), func(t *testing.T) {
				result, err := findOurClientConfig(tc.clientType)
				require.NoError(t, err, "should not error for supported client type")
				require.NotNil(t, result, "should return a config")
				assert.Equal(t, tc.clientType, result.ClientType, "should have correct client type")
				assert.Equal(t, tc.expectedName, result.Name, "should have correct name")
				assert.NotEmpty(t, result.EditorNames, "should have editor names")
			})
		}
	})

	t.Run("returns error for unsupported client type", func(t *testing.T) {
		result, err := findOurClientConfig("unsupported-client")
		assert.Error(t, err, "should error for unsupported client type")
		assert.Nil(t, result, "should return nil config")
		assert.Contains(t, err.Error(), "unsupported client type: unsupported-client", "error should mention the unsupported client type")
	})

	t.Run("returns error for empty client type", func(t *testing.T) {
		result, err := findOurClientConfig("")
		assert.Error(t, err, "should error for empty client type")
		assert.Nil(t, result, "should return nil config")
		assert.Contains(t, err.Error(), "unsupported client type:", "error should mention unsupported client type")
	})

	t.Run("verifies client config structure", func(t *testing.T) {
		// Test that each client config has required fields populated
		for _, cfg := range supportedClients {
			t.Run(string(cfg.ClientType), func(t *testing.T) {
				config, err := findOurClientConfig(cfg.ClientType)
				require.NoError(t, err)
				require.NotNil(t, config)

				assert.NotEmpty(t, config.Name, "Name should not be empty")
				assert.NotEmpty(t, config.EditorNames, "EditorNames should not be empty")

				// ConfigPaths can be empty for CLI-only clients (like VS Code)
				// Either MCPServersPathPrefix or InstallCommand should be set
				hasPathPrefix := config.MCPServersPathPrefix != ""
				hasInstallCommand := len(config.InstallCommand) > 0
				assert.True(t, hasPathPrefix || hasInstallCommand,
					"Either MCPServersPathPrefix or InstallCommand should be set for %s", cfg.ClientType)

				// If ConfigPaths is empty, InstallCommand must be set (CLI-only client)
				if len(config.ConfigPaths) == 0 {
					assert.NotEmpty(t, config.InstallCommand,
						"CLI-only clients must have InstallCommand set for %s", cfg.ClientType)
				}
			})
		}
	})
}

func TestAddTigerMCPServerViaCLI(t *testing.T) {
	t.Run("returns error when no install command configured", func(t *testing.T) {
		clientCfg := &clientConfig{
			ClientType:     "test-client",
			Name:           "Test Client",
			InstallCommand: []string{}, // Empty command
		}

		err := addTigerMCPServerViaCLI(clientCfg)
		assert.Error(t, err, "should error when no install command configured")
		assert.Contains(t, err.Error(), "no install command configured for client Test Client", "error should mention missing install command")
	})

	t.Run("returns error when install command is nil", func(t *testing.T) {
		clientCfg := &clientConfig{
			ClientType:     "test-client",
			Name:           "Test Client",
			InstallCommand: nil, // Nil command
		}

		err := addTigerMCPServerViaCLI(clientCfg)
		assert.Error(t, err, "should error when install command is nil")
		assert.Contains(t, err.Error(), "no install command configured for client Test Client", "error should mention missing install command")
	})

	t.Run("attempts to execute command when configured", func(t *testing.T) {
		// Use a command that will fail but test that we get to the execution stage
		clientCfg := &clientConfig{
			ClientType:     "test-client",
			Name:           "Test Client",
			InstallCommand: []string{"nonexistent-command-12345", "arg1", "arg2"},
		}

		err := addTigerMCPServerViaCLI(clientCfg)
		// We expect this to fail since the command doesn't exist, but it shows we got past validation
		assert.Error(t, err, "should error when command execution fails")
		assert.Contains(t, err.Error(), "failed to run Test Client CLI command", "error should mention CLI command failure")
	})

	t.Run("handles client config with single command", func(t *testing.T) {
		clientCfg := &clientConfig{
			ClientType:     "test-client",
			Name:           "Test Client",
			InstallCommand: []string{"echo"}, // Command with no args - should work
		}

		err := addTigerMCPServerViaCLI(clientCfg)
		// echo command should succeed
		assert.NoError(t, err, "should not error for valid echo command")
	})

	t.Run("handles client config with command and args", func(t *testing.T) {
		clientCfg := &clientConfig{
			ClientType:     "test-client",
			Name:           "Test Client",
			InstallCommand: []string{"echo", "test", "output"}, // Command with args
		}

		err := addTigerMCPServerViaCLI(clientCfg)
		// echo command should succeed
		assert.NoError(t, err, "should not error for valid echo command with args")
	})
}

func TestFindClientConfigFile(t *testing.T) {
	t.Run("returns error when no config paths provided", func(t *testing.T) {
		result, err := findClientConfigFile([]string{})
		assert.Error(t, err, "should error when no config paths provided")
		assert.Empty(t, result, "should return empty path")
		assert.Contains(t, err.Error(), "no config paths provided", "error should mention no config paths")
	})

	t.Run("returns error when config paths is nil", func(t *testing.T) {
		result, err := findClientConfigFile(nil)
		assert.Error(t, err, "should error when config paths is nil")
		assert.Empty(t, result, "should return empty path")
		assert.Contains(t, err.Error(), "no config paths provided", "error should mention no config paths")
	})

	t.Run("finds existing config file", func(t *testing.T) {
		// Create a temporary file
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.json")
		err := os.WriteFile(configPath, []byte(`{}`), 0644)
		require.NoError(t, err)

		result, err := findClientConfigFile([]string{configPath})
		assert.NoError(t, err, "should not error when file exists")
		assert.Equal(t, configPath, result, "should return the existing file path")
	})

	t.Run("returns fallback path when no files exist", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentPath1 := filepath.Join(tempDir, "nonexistent1.json")
		nonExistentPath2 := filepath.Join(tempDir, "nonexistent2.json")
		fallbackPath := filepath.Join(tempDir, "fallback.json")

		result, err := findClientConfigFile([]string{nonExistentPath1, nonExistentPath2, fallbackPath})
		assert.NoError(t, err, "should not error when using fallback")
		assert.Equal(t, fallbackPath, result, "should return the fallback (last) path")
	})

	t.Run("finds first existing file when multiple exist", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create multiple config files
		firstPath := filepath.Join(tempDir, "first.json")
		secondPath := filepath.Join(tempDir, "second.json")
		err := os.WriteFile(firstPath, []byte(`{}`), 0644)
		require.NoError(t, err)
		err = os.WriteFile(secondPath, []byte(`{}`), 0644)
		require.NoError(t, err)

		result, err := findClientConfigFile([]string{firstPath, secondPath})
		assert.NoError(t, err, "should not error when files exist")
		assert.Equal(t, firstPath, result, "should return the first existing file")
	})

	t.Run("expands paths with environment variables and tilde", func(t *testing.T) {
		// Create a file in a temporary directory
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.json")
		err := os.WriteFile(configPath, []byte(`{}`), 0644)
		require.NoError(t, err)

		// Set up environment variable
		testVar := "FINDCONFIGFILE_TEST_DIR"
		t.Setenv(testVar, tempDir)

		// Use environment variable in path
		envPath := "$" + testVar + "/config.json"
		result, err := findClientConfigFile([]string{envPath})
		assert.NoError(t, err, "should not error with environment variable path")
		assert.Equal(t, configPath, result, "should expand environment variable and find file")
	})
}

func TestEnsurePathExists(t *testing.T) {
	t.Run("returns error for nested paths", func(t *testing.T) {
		content := []byte(`{}`)
		value, err := hujson.Parse(content)
		require.NoError(t, err)

		err = ensurePathExists(&value, content, "/nested/path")
		assert.Error(t, err, "should error for nested paths")
		assert.Contains(t, err.Error(), "nested paths are not supported", "error should mention nested paths not supported")
		assert.Contains(t, err.Error(), "/nested/path", "error should include the problematic path")
	})

	t.Run("handles paths with dots", func(t *testing.T) {
		content := []byte(`{}`)
		value, err := hujson.Parse(content)
		require.NoError(t, err)

		err = ensurePathExists(&value, content, "/amp.mcpServers")
		assert.NoError(t, err, "should not error when creating path with dots")

		// Verify the path was created
		result := value.Pack()
		var parsed map[string]interface{}
		err = json.Unmarshal(result, &parsed)
		require.NoError(t, err)

		ampMcpServers, exists := parsed["amp.mcpServers"].(map[string]interface{})
		assert.True(t, exists, "amp.mcpServers should be created")
		assert.Empty(t, ampMcpServers, "amp.mcpServers should be an empty object")
	})

	t.Run("does nothing when path with dots already exists", func(t *testing.T) {
		content := []byte(`{"amp.mcpServers": {"existing": "value"}}`)
		value, err := hujson.Parse(content)
		require.NoError(t, err)

		err = ensurePathExists(&value, content, "/amp.mcpServers")
		assert.NoError(t, err, "should not error when path with dots already exists")

		// Verify the value is unchanged
		result := value.Pack()
		var parsed map[string]interface{}
		err = json.Unmarshal(result, &parsed)
		require.NoError(t, err)

		ampMcpServers, exists := parsed["amp.mcpServers"].(map[string]interface{})
		assert.True(t, exists, "amp.mcpServers should still exist")
		assert.Equal(t, "value", ampMcpServers["existing"], "existing value should be unchanged")
	})
}

func TestInstallMCPForEditor_Integration(t *testing.T) {
	// Override getTigerExecutablePath to return "tiger" for tests
	oldFunc := tigerExecutablePathFunc
	tigerExecutablePathFunc = func() (string, error) {
		return "tiger", nil
	}
	defer func() {
		tigerExecutablePathFunc = oldFunc
	}()

	t.Run("installs for Cursor with JSON config", func(t *testing.T) {
		// Use Cursor since it uses JSON-based config that we can fully control
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "mcp.json")

		// Create initial empty config
		initialConfig := `{"mcpServers": {}}`
		err := os.WriteFile(configPath, []byte(initialConfig), 0644)
		require.NoError(t, err, "should create initial config file")

		// Call installMCPForClient to install Tiger MCP server
		err = installMCPForClient("cursor", false, configPath)
		require.NoError(t, err, "installMCPForClient should succeed")

		// Verify the config file was modified
		configContent, err := os.ReadFile(configPath)
		require.NoError(t, err, "should be able to read config file")

		var config map[string]interface{}
		err = json.Unmarshal(configContent, &config)
		require.NoError(t, err, "config should be valid JSON")

		// Check that mcpServers exists and contains tigerdata
		mcpServers, exists := config["mcpServers"].(map[string]interface{})
		require.True(t, exists, "mcpServers should exist in config")

		tigerdata, exists := mcpServers["tigerdata"].(map[string]interface{})
		require.True(t, exists, "tigerdata should be added to mcpServers")

		assert.Equal(t, "tiger", tigerdata["command"], "command should be 'tiger'")
		args, ok := tigerdata["args"].([]interface{})
		require.True(t, ok, "args should be an array")
		require.Len(t, args, 2, "should have two arguments")
		assert.Equal(t, "mcp", args[0], "first arg should be 'mcp'")
		assert.Equal(t, "start", args[1], "second arg should be 'start'")
	})

	t.Run("creates backup when requested", func(t *testing.T) {
		// Create a temporary config file for Cursor
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "mcp.json")

		initialConfig := `{"mcpServers": {"existing": {"command": "test", "args": ["arg1"]}}}`
		err := os.WriteFile(configPath, []byte(initialConfig), 0644)
		require.NoError(t, err)

		// Call installMCPForClient with backup enabled for Cursor
		err = installMCPForClient("cursor", true, configPath)
		require.NoError(t, err, "installMCPForClient should succeed with backup")

		// Check that a backup file was created
		backupFiles, err := filepath.Glob(configPath + ".backup.*")
		require.NoError(t, err, "should be able to glob for backup files")
		assert.NotEmpty(t, backupFiles, "backup file should be created")

		// Verify backup contains original content
		if len(backupFiles) > 0 {
			backupContent, err := os.ReadFile(backupFiles[0])
			require.NoError(t, err, "should be able to read backup file")
			assert.Equal(t, initialConfig, string(backupContent), "backup should contain original config")
		}

		// Verify config was modified to include tigerdata
		configContent, err := os.ReadFile(configPath)
		require.NoError(t, err)

		var config map[string]interface{}
		err = json.Unmarshal(configContent, &config)
		require.NoError(t, err)

		mcpServers := config["mcpServers"].(map[string]interface{})
		assert.Contains(t, mcpServers, "tigerdata", "tigerdata should be added")
		assert.Contains(t, mcpServers, "existing", "existing server should be preserved")
	})

	t.Run("handles unsupported editor", func(t *testing.T) {
		err := installMCPForClient("unsupported-editor", false, "")
		assert.Error(t, err, "should error for unsupported editor")
		assert.Contains(t, err.Error(), "unsupported client", "error should mention unsupported client")
	})
}
