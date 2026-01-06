package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tailscale/hujson"
	"go.uber.org/zap"

	"github.com/timescale/tiger-cli/internal/tiger/logging"
	"github.com/timescale/tiger-cli/internal/tiger/mcp"
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// MCPClient represents our internal client types
type MCPClient string

const (
	ClaudeCode  MCPClient = "claude-code"
	Cursor      MCPClient = "cursor" // Both the IDE and the CLI
	Windsurf    MCPClient = "windsurf"
	Codex       MCPClient = "codex"
	Gemini      MCPClient = "gemini"
	VSCode      MCPClient = "vscode"
	Antigravity MCPClient = "antigravity"
	KiroCLI     MCPClient = "kiro-cli"
)

// MCPServerConfig represents the MCP server configuration
type MCPServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// InstallOptions configures the MCP server installation behavior
type InstallOptions struct {
	// ClientName is the name of the client to configure (required)
	ClientName string
	// ServerName is the name to register the MCP server as (required)
	ServerName string
	// Command is the path to the MCP server binary (required)
	Command string
	// Args are the arguments to pass to the MCP server binary (required)
	Args []string
	// CreateBackup creates a backup of existing config files before modification
	CreateBackup bool
	// CustomConfigPath overrides the default config file location
	CustomConfigPath string
}

// clientConfig represents our own client configuration for Tiger MCP installation
type clientConfig struct {
	ClientType           MCPClient // Our internal client type
	Name                 string
	EditorNames          []string // Supported client names for this client
	MCPServersPathPrefix string   // JSON path prefix for MCP servers config (only for JSON config manipulation clients like Cursor/Windsurf)
	ConfigPaths          []string // Config file locations - used for backup on all clients, and for JSON manipulation on JSON-config clients
	// buildInstallCommand builds the CLI install command for CLI-based clients
	// Parameters: serverName (name to register), command (binary path), args (arguments to binary)
	buildInstallCommand func(serverName, command string, args []string) ([]string, error)
}

// BuildInstallCommand constructs the install command with the given parameters
func (c *clientConfig) BuildInstallCommand(serverName, command string, args []string) ([]string, error) {
	if c.buildInstallCommand == nil {
		return nil, nil
	}
	return c.buildInstallCommand(serverName, command, args)
}

// supportedClients defines the clients we support for Tiger MCP installation
// Note: A good place to find the json config location for MCPServersPathPrefix
// is in the supportedClientIntegrations map found in:
// https://github.com/stacklok/toolhive/blob/main/pkg/client/config.go
var supportedClients = []clientConfig{
	{
		ClientType:  ClaudeCode,
		Name:        "Claude Code",
		EditorNames: []string{"claude-code"},
		ConfigPaths: []string{
			"~/.claude.json",
		},
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"claude", "mcp", "add", "-s", "user", serverName, command}, args...), nil
		},
	},
	{
		ClientType:           Cursor,
		Name:                 "Cursor",
		EditorNames:          []string{"cursor"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.cursor/mcp.json",
		},
	},
	{
		ClientType:           Windsurf,
		Name:                 "Windsurf",
		EditorNames:          []string{"windsurf"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.codeium/windsurf/mcp_config.json",
		},
	},
	{
		ClientType:  Codex,
		Name:        "Codex",
		EditorNames: []string{"codex"},
		ConfigPaths: []string{
			"~/.codex/config.toml",
			"$CODEX_HOME/config.toml",
		},
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"codex", "mcp", "add", serverName, command}, args...), nil
		},
	},
	{
		ClientType:  Gemini,
		Name:        "Gemini CLI",
		EditorNames: []string{"gemini", "gemini-cli"},
		ConfigPaths: []string{
			"~/.gemini/settings.json",
		},
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"gemini", "mcp", "add", "-s", "user", serverName, command}, args...), nil
		},
	},
	{
		ClientType:  VSCode,
		Name:        "VS Code",
		EditorNames: []string{"vscode", "code", "vs-code"},
		ConfigPaths: []string{
			"~/.config/Code/User/mcp.json",
			"~/Library/Application Support/Code/User/mcp.json",
			"~/AppData/Roaming/Code/User/mcp.json",
		},
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			j, err := json.Marshal(map[string]any{
				"name":    serverName,
				"command": command,
				"args":    args,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to marshal MCP config: %w", err)
			}
			return []string{"code", "--add-mcp", string(j)}, nil
		},
	},
	{
		ClientType:           Antigravity,
		Name:                 "Google Antigravity",
		EditorNames:          []string{"antigravity", "agy"},
		MCPServersPathPrefix: "/mcpServers",
		ConfigPaths: []string{
			"~/.gemini/antigravity/mcp_config.json",
		},
	},
	{
		ClientType:  KiroCLI,
		Name:        "Kiro CLI",
		EditorNames: []string{"kiro-cli"},
		ConfigPaths: []string{
			"~/.kiro/settings/mcp.json",
		},
		buildInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return []string{"kiro-cli", "mcp", "add", "--name", serverName, "--command", command, "--args", strings.Join(args, ",")}, nil
		},
	},
}

// getValidEditorNames returns all valid client names from supportedClients
func getValidEditorNames() []string {
	var validNames []string
	for _, client := range supportedClients {
		validNames = append(validNames, client.EditorNames...)
	}
	return validNames
}

// ClientInfo contains information about a supported MCP client.
type ClientInfo struct {
	// Name is the human-readable display name (e.g., "Claude Code", "Cursor")
	Name string
	// ClientName is the identifier to use in InstallOptions.ClientName (e.g., "claude-code", "cursor")
	ClientName string
}

// SupportedClients returns information about all supported MCP clients.
func SupportedClients() []ClientInfo {
	clients := make([]ClientInfo, 0, len(supportedClients))
	for _, c := range supportedClients {
		clients = append(clients, ClientInfo{
			Name:       c.Name,
			ClientName: c.EditorNames[0],
		})
	}
	return clients
}

// InstallMCPForClient installs an MCP server configuration for the specified client.
// This is a generic, configurable function exported for use by external projects via pkg/mcpinstall.
// Required options: ServerName, Command, Args must all be provided.
func InstallMCPForClient(opts InstallOptions) error {
	// Validate required options
	if opts.ClientName == "" {
		return fmt.Errorf("ClientName is required")
	}
	if opts.ServerName == "" {
		return fmt.Errorf("ServerName is required")
	}
	if opts.Command == "" {
		return fmt.Errorf("Command is required")
	}
	if opts.Args == nil {
		return fmt.Errorf("Args is required")
	}

	// Find the client configuration by name
	clientCfg, err := findClientConfig(opts.ClientName)
	if err != nil {
		return err
	}

	mcpServersPathPrefix := clientCfg.MCPServersPathPrefix

	var configPath string
	if opts.CustomConfigPath != "" {
		// Expand custom config path for ~ and environment variables, then use it directly
		configPath = util.ExpandPath(opts.CustomConfigPath)
	} else if len(clientCfg.ConfigPaths) > 0 {
		// Use manual config path discovery for clients with configured paths
		configPath, err = findClientConfigFile(clientCfg.ConfigPaths)
		if err != nil {
			return fmt.Errorf("failed to find configuration for %s: %w", opts.ClientName, err)
		}
	} else if clientCfg.buildInstallCommand == nil {
		// Client has neither ConfigPaths nor buildInstallCommand
		return fmt.Errorf("client %s has no ConfigPaths or buildInstallCommand defined", opts.ClientName)
	}
	// else: CLI-only client - configPath remains empty, will use buildInstallCommand

	logging.Info("Installing MCP server configuration",
		zap.String("client", opts.ClientName),
		zap.String("server_name", opts.ServerName),
		zap.String("command", opts.Command),
		zap.Strings("args", opts.Args),
		zap.String("config_path", configPath),
		zap.String("mcp_servers_path", mcpServersPathPrefix),
		zap.Bool("create_backup", opts.CreateBackup),
	)

	// Create backup if requested and we have a config file
	if opts.CreateBackup && configPath != "" {
		_, err = createConfigBackup(configPath)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Add MCP server to configuration
	if clientCfg.buildInstallCommand != nil {
		// Use CLI approach when install command builder is configured
		if err := addMCPServerViaCLI(clientCfg, opts.ServerName, opts.Command, opts.Args); err != nil {
			return fmt.Errorf("failed to add MCP server configuration: %w", err)
		}
	} else {
		// Use JSON patching approach for JSON-config clients
		if err := addMCPServerViaJSON(configPath, mcpServersPathPrefix, opts.ServerName, opts.Command, opts.Args); err != nil {
			return fmt.Errorf("failed to add MCP server configuration: %w", err)
		}
	}

	return nil
}

// installTigerMCPForClient installs the Tiger MCP server configuration for the specified client.
// This is the Tiger-specific wrapper used by the CLI that handles defaults and success messages.
func installTigerMCPForClient(clientName string, createBackup bool, customConfigPath string) error {
	// Get the Tiger executable path
	command, err := getTigerExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	opts := InstallOptions{
		ClientName:       clientName,
		ServerName:       mcp.ServerName,
		Command:          command,
		Args:             []string{"mcp", "start"},
		CreateBackup:     createBackup,
		CustomConfigPath: customConfigPath,
	}

	if err := InstallMCPForClient(opts); err != nil {
		return err
	}

	// Print Tiger-specific success messages
	configPath := customConfigPath
	if configPath == "" {
		clientCfg, _ := findClientConfig(clientName)
		if clientCfg != nil && len(clientCfg.ConfigPaths) > 0 {
			configPath, _ = findClientConfigFile(clientCfg.ConfigPaths)
		}
	}

	fmt.Printf("✅ Successfully installed Tiger MCP server configuration for %s\n", clientName)
	if configPath != "" {
		fmt.Printf("📁 Configuration file: %s\n", configPath)
	} else {
		fmt.Printf("⚙️  Configuration managed by %s\n", clientName)
	}

	fmt.Printf("\n💡 Next steps:\n")
	fmt.Printf("   1. Restart %s to load the new configuration\n", clientName)
	fmt.Printf("   2. The Tiger MCP server will be available as '%s'\n", mcp.ServerName)
	fmt.Printf("\n🤖 Try asking your AI assistant:\n")
	fmt.Printf("\n   📊 List and manage your Tiger Cloud services:\n")
	fmt.Printf("   • \"List my Tiger Cloud services\"\n")
	fmt.Printf("   • \"Show me details for service xyz-123\"\n")
	fmt.Printf("   • \"Create a new database service called my-app-db\"\n")
	fmt.Printf("   • \"Update the password for my database service\"\n")
	fmt.Printf("   • \"What Tiger Cloud services do I have access to?\"\n")
	fmt.Printf("\n   📚 Ask questions from the PostgreSQL and Tiger Cloud documentation:\n")
	fmt.Printf("   • \"Show me Tiger Cloud documentation about hypertables?\"\n")
	fmt.Printf("   • \"What are the best practices for PostgreSQL indexing?\"\n")
	fmt.Printf("   • \"What is the command for renaming a table?\"\n")
	fmt.Printf("   • \"Help me optimize my PostgreSQL queries\"\n")
	fmt.Printf("\n   📋 Make use of our optimized AI guides for common workflows:\n")
	fmt.Printf("   • \"Help me create a new database schema for my application\"\n")
	fmt.Printf("   • \"Help me set up hypertables for the device_readings table\"\n")
	fmt.Printf("   • \"Help me figure out which tables should be hypertables\"\n")
	fmt.Printf("   • \"What's the best way to structure time-series data?\"\n")

	return nil
}

// findClientConfig finds the client configuration for a given client name
// This consolidates the logic of mapping client names to client types and finding the config
func findClientConfig(clientName string) (*clientConfig, error) {
	normalizedName := strings.ToLower(clientName)

	// Look up in our supported clients config
	for i := range supportedClients {
		for _, name := range supportedClients[i].EditorNames {
			if strings.ToLower(name) == normalizedName {
				return &supportedClients[i], nil
			}
		}
	}

	// Build list of supported clients from our config for error message
	supportedNames := getValidEditorNames()

	return nil, fmt.Errorf("unsupported client: %s. Supported clients: %s", clientName, strings.Join(supportedNames, ", "))
}

// generateSupportedEditorsHelp generates the supported clients section for help text
func generateSupportedEditorsHelp() string {
	result := "Supported Clients:\n"
	for _, cfg := range supportedClients {
		// Show only the primary editor name in help text
		primaryName := cfg.EditorNames[0]
		result += fmt.Sprintf("  %-24s Configure for %s\n", primaryName, cfg.Name)
	}
	return result
}

// findClientConfigFile finds a client configuration file from a list of possible paths
func findClientConfigFile(configPaths []string) (string, error) {
	for _, path := range configPaths {
		// Expand environment variables and home directory
		expandedPath := util.ExpandPath(path)

		// Check if file exists
		if _, err := os.Stat(expandedPath); err == nil {
			logging.Info("Found existing config file", zap.String("path", expandedPath))
			return expandedPath, nil
		}
	}

	// If no existing config found, use the first path as default
	if len(configPaths) == 0 {
		return "", fmt.Errorf("no config paths provided")
	}

	defaultPath := util.ExpandPath(configPaths[0]) // Use first path as default
	logging.Info("No existing config found, will create at default location",
		zap.String("path", defaultPath))
	return defaultPath, nil
}

// tigerExecutablePathFunc can be overridden in tests to return a fixed path
var tigerExecutablePathFunc = defaultGetTigerExecutablePath

// getTigerExecutablePath returns the full path to the currently executing Tiger binary
func getTigerExecutablePath() (string, error) {
	return tigerExecutablePathFunc()
}

// defaultGetTigerExecutablePath is the default implementation
func defaultGetTigerExecutablePath() (string, error) {
	tigerPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// If running via 'go run', os.Executable() returns a temp path like /tmp/go-build*/exe/tiger
	// In this case, return "tiger" assuming it's in PATH for development
	if strings.Contains(tigerPath, "go-build") && strings.Contains(tigerPath, "/exe/") {
		return "tiger", nil
	}

	return tigerPath, nil
}

// ClientOption represents a client choice for interactive selection
type ClientOption struct {
	Name       string // Display name
	ClientName string // Client name to pass to installMCPForClient
}

// selectClientInteractively prompts the user to select a client using Bubble Tea
func selectClientInteractively(out io.Writer) (string, error) {
	// Build client options from supportedClients
	var options []ClientOption
	for _, cfg := range supportedClients {
		// Use the first client name as the primary identifier
		primaryName := cfg.EditorNames[0]
		options = append(options, ClientOption{
			Name:       cfg.Name,
			ClientName: primaryName,
		})
	}

	// Sort options alphabetically by name
	sort.Slice(options, func(i, j int) bool {
		return options[i].Name < options[j].Name
	})

	model := clientSelectModel{
		options: options,
		cursor:  0,
	}

	program := tea.NewProgram(model, tea.WithOutput(out))
	finalModel, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run editor selection: %w", err)
	}

	result := finalModel.(clientSelectModel)
	if result.selected == "" {
		return "", fmt.Errorf("no editor selected")
	}

	return result.selected, nil
}

// clientSelectModel represents the Bubble Tea model for client selection
type clientSelectModel struct {
	options      []ClientOption
	cursor       int
	selected     string
	numberBuffer string
}

func (m clientSelectModel) Init() tea.Cmd {
	return nil
}

func (m clientSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			// Clear buffer when using arrows
			m.numberBuffer = ""
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			// Clear buffer when using arrows
			m.numberBuffer = ""
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.selected = m.options[m.cursor].ClientName
			return m, tea.Quit
		case "backspace":
			// Handle backspace to remove last character from buffer
			if len(m.numberBuffer) > 0 {
				m.updateNumberBuffer(m.numberBuffer[:len(m.numberBuffer)-1])
			}
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Add digit to buffer and update cursor position
			m.updateNumberBuffer(m.numberBuffer + msg.String())
		case "ctrl+w":
			// Clear buffer
			m.numberBuffer = ""
		}
	}
	return m, nil
}

// updateNumberBuffer moves the cursor to the editor matching the number buffer
func (m *clientSelectModel) updateNumberBuffer(newBuffer string) {
	if newBuffer == "" {
		m.numberBuffer = newBuffer
		return
	}

	// Parse the buffer as a number
	num, err := strconv.Atoi(newBuffer)
	if err != nil {
		return
	}

	// Convert from 1-based to 0-based index and validate bounds
	index := num - 1
	if index >= 0 && index < len(m.options) {
		m.numberBuffer = newBuffer
		m.cursor = index
	}
}

func (m clientSelectModel) View() string {
	s := "Select an MCP client to configure:\n\n"

	for i, option := range m.options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %d. %s\n", cursor, i+1, option.Name)
	}

	// Show the current number buffer if user is typing
	if m.numberBuffer != "" {
		s += fmt.Sprintf("\nTyping: %s", m.numberBuffer)
	}

	s += "\nUse ↑/↓ arrows or number keys to navigate, enter to select, q to quit"
	return s
}

// addMCPServerViaCLI adds an MCP server using a CLI command configured in clientConfig
func addMCPServerViaCLI(clientCfg *clientConfig, serverName, command string, args []string) error {
	if clientCfg.buildInstallCommand == nil {
		return fmt.Errorf("no install command configured for client %s", clientCfg.Name)
	}

	// Build the install command with the provided parameters
	installCommand, err := clientCfg.BuildInstallCommand(serverName, command, args)
	if err != nil {
		return fmt.Errorf("failed to build install command: %w", err)
	}

	logging.Info("Adding MCP server using CLI",
		zap.String("client", clientCfg.Name),
		zap.Strings("command", installCommand))

	// Run the configured CLI command
	cmd := exec.Command(installCommand[0], installCommand[1:]...)

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		cmdStr := strings.Join(installCommand, " ")
		if string(output) != "" {
			return fmt.Errorf("failed to run %s installation command: %w\nCommand: %s\nOutput: %s", clientCfg.Name, err, cmdStr, string(output))
		}
		return fmt.Errorf("failed to run %s installation command: %w\nCommand: %s", clientCfg.Name, err, cmdStr)
	}

	logging.Info("Successfully added MCP server via CLI",
		zap.String("client", clientCfg.Name),
		zap.String("output", string(output)))

	return nil
}

// createConfigBackup creates a backup of the existing configuration file and returns the backup path
func createConfigBackup(configPath string) (string, error) {
	// Check if config file exists
	if _, err := os.Stat(configPath); errors.Is(err, fs.ErrNotExist) {
		// No existing config file, no backup needed
		logging.Info("No existing configuration file found, skipping backup")
		return "", nil
	}

	backupPath := fmt.Sprintf("%s.backup.%d", configPath, time.Now().UnixNano())

	// Get original file mode, fallback to 0600 if unavailable
	origInfo, err := os.Stat(configPath)
	var mode fs.FileMode = 0600
	if err == nil {
		mode = origInfo.Mode().Perm()
	}

	// Read original file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read original config file: %w", err)
	}

	// Write backup file
	if err := os.WriteFile(backupPath, data, mode); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	logging.Info("Created configuration backup", zap.String("backup_path", backupPath))
	return backupPath, nil
}

// addMCPServerViaJSON adds an MCP server to the configuration file using JSON patching
func addMCPServerViaJSON(configPath, mcpServersPathPrefix, serverName, command string, args []string) error {
	// Create configuration directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create configuration directory %s: %w", configDir, err)
	}

	// MCP server configuration
	serverConfig := MCPServerConfig{
		Command: command,
		Args:    args,
	}

	// Get original file mode to preserve it, fallback to 0600 for new files
	var fileMode fs.FileMode = 0600
	if info, err := os.Stat(configPath); err == nil {
		fileMode = info.Mode().Perm()
	}

	// Read existing configuration or create empty one
	content, err := os.ReadFile(configPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to read config file: %w", err)
		}
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

	// Check if the parent path exists using hujson's Find method
	// Find uses JSON Pointer format (RFC 6901) which matches our path format
	if value.Find(mcpServersPathPrefix) == nil {
		// Path doesn't exist, create it
		parentPatch := fmt.Sprintf(`[{ "op": "add", "path": "%s", "value": {} }]`, mcpServersPathPrefix)
		if err := value.Patch([]byte(parentPatch)); err != nil {
			return fmt.Errorf("failed to create MCP servers path: %w", err)
		}
	}

	// Marshal the MCP server data
	dataJSON, err := json.Marshal(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal MCP server data: %w", err)
	}

	// Create JSON patch to add the MCP server
	patch := fmt.Sprintf(`[{ "op": "add", "path": "%s/%s", "value": %s }]`, mcpServersPathPrefix, serverName, dataJSON)

	// Apply the patch
	if err := value.Patch([]byte(patch)); err != nil {
		return fmt.Errorf("failed to apply JSON patch: %w", err)
	}

	// Format the result
	formatted, err := hujson.Format(value.Pack())
	if err != nil {
		return fmt.Errorf("failed to format patched JSON: %w", err)
	}

	// Write back to file (preserve original file mode)
	if err := os.WriteFile(configPath, formatted, fileMode); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logging.Info("Added MCP server to configuration",
		zap.String("server_name", serverName),
		zap.String("command", serverConfig.Command),
		zap.Strings("args", serverConfig.Args),
	)

	return nil
}
