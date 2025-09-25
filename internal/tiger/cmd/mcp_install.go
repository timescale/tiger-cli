package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/renameio/v2"
	"github.com/stacklok/toolhive/pkg/client"
	"github.com/tailscale/hujson"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/logging"
)

// lockTimeout is the maximum time to wait for a file lock
const lockTimeout = 1 * time.Second

// TigerMCPClient represents our internal client types (extends beyond toolhive support)
type TigerMCPClient string

const (
	TigerClaudeCode TigerMCPClient = "claude-code"
	TigerCursor     TigerMCPClient = "cursor"
	TigerWindsurf   TigerMCPClient = "windsurf"
	TigerCodex      TigerMCPClient = "codex" // Not supported by toolhive - uses CLI
)

// TigerMCPServer represents the Tiger MCP server configuration
type TigerMCPServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// clientConfig represents our own client configuration for Tiger MCP installation
type clientConfig struct {
	TigerClientType      TigerMCPClient   // Our internal client type
	ToolhiveClientType   client.MCPClient // Toolhive client type (empty if not supported)
	Name                 string
	EditorNames          []string // supported editor names for this client
	MCPServersPathPrefix string
	ConfigPaths          []string // possible config file locations (for non-toolhive clients)
	InstallCommand       []string // CLI command to install MCP server (for CLI-based clients)
}

// supportedClients defines the clients we support for Tiger MCP installation
// Note: MCPServersPathPrefix can be found in the supportedClientIntegrations map found in:
// https://github.com/stacklok/toolhive/blob/main/pkg/client/config.go
var supportedClients = []clientConfig{
	{
		TigerClientType:      TigerClaudeCode,
		ToolhiveClientType:   client.ClaudeCode,
		Name:                 "Claude Code",
		EditorNames:          []string{"claude-code", "claude"},
		MCPServersPathPrefix: "/mcpServers",
	},
	{
		TigerClientType:      TigerCursor,
		ToolhiveClientType:   client.Cursor,
		Name:                 "Cursor",
		EditorNames:          []string{"cursor"},
		MCPServersPathPrefix: "/mcpServers",
	},
	{
		TigerClientType:      TigerWindsurf,
		ToolhiveClientType:   client.Windsurf,
		Name:                 "Windsurf",
		EditorNames:          []string{"windsurf"},
		MCPServersPathPrefix: "/mcpServers",
	},
	{
		TigerClientType:      TigerCodex,
		ToolhiveClientType:   "", // Not supported by toolhive - use CLI approach
		Name:                 "Codex",
		EditorNames:          []string{"codex"},
		MCPServersPathPrefix: "", // Not used for Codex - uses TOML instead
		ConfigPaths: []string{
			"$CODEX_HOME/config.toml",
			"~/.codex/config.toml", // Default fallback
		},
		InstallCommand: []string{"codex", "mcp", "add", "tigerdata", "tiger", "mcp"},
	},
}

// installMCPForEditor installs the Tiger MCP server configuration for the specified editor
func installMCPForEditor(editorName string, createBackup bool, customConfigPath string) error {
	// Map editor names to our client types
	tigerClientType, err := mapEditorToTigerClientType(editorName)
	if err != nil {
		return err
	}

	// Get the MCP servers path prefix from our own configuration
	clientCfg, err := findOurClientConfig(tigerClientType)
	if err != nil {
		return err
	}

	mcpServersPathPrefix := clientCfg.MCPServersPathPrefix

	var configPath string
	if customConfigPath != "" {
		// Use custom config path directly, skip discovery
		configPath = customConfigPath
	} else if len(clientCfg.ConfigPaths) > 0 {
		// Use manual config path discovery for clients with configured paths
		configPath, err = findClientConfigFile(clientCfg.ConfigPaths)
		if err != nil {
			return fmt.Errorf("failed to find configuration for %s: %w", editorName, err)
		}
	} else {
		// Use toolhive to find the client configuration file path
		configFile, err := client.FindClientConfig(clientCfg.ToolhiveClientType)
		if err != nil {
			return fmt.Errorf("failed to find configuration for %s: %w", editorName, err)
		}
		configPath = configFile.Path
	}

	logging.Info("Installing Tiger MCP server configuration",
		zap.String("editor", editorName),
		zap.String("config_path", configPath),
		zap.String("mcp_servers_path", mcpServersPathPrefix),
		zap.Bool("create_backup", createBackup),
	)

	// Create backup if requested
	var backupPath string
	if createBackup {
		var err error
		backupPath, err = createConfigBackup(configPath)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Add Tiger MCP server to configuration
	if len(clientCfg.InstallCommand) > 0 {
		// Use CLI approach when install command is configured
		if err := addTigerMCPServerViaCLI(clientCfg); err != nil {
			return fmt.Errorf("failed to add Tiger MCP server configuration: %w", err)
		}
	} else {
		// Use JSON patching approach for toolhive-supported clients
		if err := addTigerMCPServer(configPath, mcpServersPathPrefix); err != nil {
			return fmt.Errorf("failed to add Tiger MCP server configuration: %w", err)
		}
	}

	fmt.Printf("✅ Successfully installed Tiger MCP server configuration for %s\n", editorName)
	fmt.Printf("📁 Configuration file: %s\n", configPath)

	if createBackup && backupPath != "" {
		fmt.Printf("💾 Backup created: %s\n", backupPath)
	}

	fmt.Printf("\n💡 Next steps:\n")
	fmt.Printf("   1. Restart %s to load the new configuration\n", editorName)
	fmt.Printf("   2. The TigerData MCP server will be available as 'tigerdata'\n")

	return nil
}

// mapEditorToTigerClientType maps editor names to our Tiger client types using our supportedClients config
func mapEditorToTigerClientType(editorName string) (TigerMCPClient, error) {
	normalizedName := strings.ToLower(editorName)

	// Look up in our supported clients config
	for _, cfg := range supportedClients {
		for _, name := range cfg.EditorNames {
			if strings.ToLower(name) == normalizedName {
				return cfg.TigerClientType, nil
			}
		}
	}

	// Build list of supported editors from our config
	var supportedNames []string
	for _, cfg := range supportedClients {
		supportedNames = append(supportedNames, cfg.EditorNames...)
	}

	return "", fmt.Errorf("unsupported editor: %s. Supported editors: %s", editorName, strings.Join(supportedNames, ", "))
}

// findOurClientConfig finds our client configuration for a given Tiger client type
func findOurClientConfig(tigerClientType TigerMCPClient) (*clientConfig, error) {
	for _, cfg := range supportedClients {
		if cfg.TigerClientType == tigerClientType {
			return &cfg, nil
		}
	}
	return nil, fmt.Errorf("unsupported client type: %s", tigerClientType)
}

// generateSupportedEditorsHelp generates the supported editors section for help text
func generateSupportedEditorsHelp() string {
	result := "Supported Editors:\n"
	for _, cfg := range supportedClients {
		// Show all editor names for this client
		editorNames := strings.Join(cfg.EditorNames, " (or ")
		if len(cfg.EditorNames) > 1 {
			editorNames = fmt.Sprintf("%s)", editorNames)
		}
		result += fmt.Sprintf("  %-24s Configure for %s\n", editorNames, cfg.Name)
	}
	return result
}

// findClientConfigFile finds a client configuration file from a list of possible paths
func findClientConfigFile(configPaths []string) (string, error) {
	for _, path := range configPaths {
		// Expand environment variables and home directory
		expandedPath := expandPath(path)

		// Check if file exists
		if _, err := os.Stat(expandedPath); err == nil {
			logging.Info("Found existing config file", zap.String("path", expandedPath))
			return expandedPath, nil
		}
	}

	// If no existing config found, use the last path as default
	if len(configPaths) == 0 {
		return "", fmt.Errorf("no config paths provided")
	}

	defaultPath := expandPath(configPaths[len(configPaths)-1]) // Use last path as fallback
	logging.Info("No existing config found, will create at default location",
		zap.String("path", defaultPath))
	return defaultPath, nil
}

// expandPath expands environment variables and tilde in file paths
func expandPath(path string) string {
	// Expand environment variables
	expanded := os.ExpandEnv(path)

	// Expand home directory
	if strings.HasPrefix(expanded, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(homeDir, expanded[2:])
		}
	}

	return expanded
}

// addTigerMCPServerViaCLI adds Tiger MCP server using a CLI command configured in clientConfig
func addTigerMCPServerViaCLI(clientCfg *clientConfig) error {
	if len(clientCfg.InstallCommand) == 0 {
		return fmt.Errorf("no install command configured for client %s", clientCfg.Name)
	}

	logging.Info("Adding Tiger MCP server using CLI",
		zap.String("client", clientCfg.Name),
		zap.Strings("command", clientCfg.InstallCommand))

	// Run the configured CLI command
	cmd := exec.Command(clientCfg.InstallCommand[0], clientCfg.InstallCommand[1:]...)

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run %s CLI command: %w\nOutput: %s", clientCfg.Name, err, string(output))
	}

	logging.Info("Successfully added Tiger MCP server via CLI",
		zap.String("client", clientCfg.Name),
		zap.String("output", string(output)))

	return nil
}

// createConfigBackup creates a backup of the existing configuration file and returns the backup path
func createConfigBackup(configPath string) (string, error) {
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// No existing config file, no backup needed
		logging.Info("No existing configuration file found, skipping backup")
		return "", nil
	}

	backupPath := fmt.Sprintf("%s.backup.%d", configPath, time.Now().Unix())

	// Read original file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read original config file: %w", err)
	}

	// Write backup file atomically
	if err := renameio.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	logging.Info("Created configuration backup", zap.String("backup_path", backupPath))
	return backupPath, nil
}

// addTigerMCPServer adds the Tiger MCP server to the configuration file using JSON patching with file locking
func addTigerMCPServer(configPath string, mcpServersPathPrefix string) error {
	// Create configuration directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create configuration directory %s: %w", configDir, err)
	}
	// Tiger MCP server configuration
	tigerServer := TigerMCPServer{
		Command: "tiger",
		Args:    []string{"mcp"},
	}

	// Create a lock file
	lockPath := configPath + ".lock"
	fileLock := flock.New(lockPath)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()

	// Try to acquire the lock with a timeout
	locked, err := fileLock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("failed to acquire lock: timeout after %v", lockTimeout)
	}
	defer func() {
		if err := fileLock.Unlock(); err != nil {
			logging.Error("Failed to release file lock", zap.Error(err))
		}
	}()

	// Read existing configuration or create empty one
	content, err := os.ReadFile(configPath)
	if err != nil {
		logging.Info("Config file not found, creating new one")
		content = []byte("{}")
	}

	if len(content) == 0 {
		// If the file is empty, initialize with empty JSON object
		content = []byte("{}")
	}

	// Parse the JSON with hujson
	value, err := hujson.Parse(content)
	if err != nil {
		return fmt.Errorf("failed to parse existing config: %w", err)
	}

	// Ensure the MCP servers path exists
	if err := ensurePathExists(&value, content, mcpServersPathPrefix); err != nil {
		return fmt.Errorf("failed to ensure MCP servers path exists: %w", err)
	}

	// Marshal the Tiger MCP server data
	dataJSON, err := json.Marshal(tigerServer)
	if err != nil {
		return fmt.Errorf("failed to marshal Tiger MCP server data: %w", err)
	}

	// Create JSON patch to add the Tiger MCP server
	patch := fmt.Sprintf(`[{ "op": "add", "path": "%s/tigerdata", "value": %s }]`, mcpServersPathPrefix, dataJSON)

	// Apply the patch
	if err := value.Patch([]byte(patch)); err != nil {
		return fmt.Errorf("failed to apply JSON patch: %w", err)
	}

	// Format the result
	formatted, err := hujson.Format(value.Pack())
	if err != nil {
		return fmt.Errorf("failed to format patched JSON: %w", err)
	}

	// Write back to file atomically
	if err := renameio.WriteFile(configPath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logging.Info("Added Tiger MCP server to configuration",
		zap.String("server_name", "tigerdata"),
		zap.String("command", tigerServer.Command),
		zap.Strings("args", tigerServer.Args),
	)

	return nil
}

// ensurePathExists ensures that the MCP servers path exists in the parsed JSON value.
// This simplified version only handles single-level paths like "/mcpServers", "/servers", "/amp.mcpServers".
// It returns an error if nested paths (containing "/") are detected.
// The function modifies the provided hujson.Value in place.
func ensurePathExists(value *hujson.Value, content []byte, path string) error {
	// Validate that this is a single-level path
	key := strings.TrimPrefix(path, "/")
	if strings.Contains(key, "/") {
		return fmt.Errorf("nested paths are not supported, got: %s", path)
	}

	// Check if the key already exists using gjson
	// For keys with dots (like "amp.mcpServers"), we need to escape the dots for gjson
	escapedKey := strings.ReplaceAll(key, ".", `\.`)
	if gjson.GetBytes(content, escapedKey).Exists() {
		// Path already exists, nothing to do
		return nil
	}

	// Create a JSON patch to add an empty object at this path
	patch := fmt.Sprintf(`[{ "op": "add", "path": "%s", "value": {} }]`, path)

	// Apply the patch
	if err := value.Patch([]byte(patch)); err != nil {
		return fmt.Errorf("failed to apply JSON patch: %w", err)
	}

	return nil
}
