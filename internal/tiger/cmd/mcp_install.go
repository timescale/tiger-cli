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
	"github.com/timescale/tiger-cli/internal/tiger/util"
)

// MCPClient represents our internal client types
type MCPClient string

const (
	ClaudeCode MCPClient = "claude-code"
	Cursor     MCPClient = "cursor" //both the ide and the cli.
	Windsurf   MCPClient = "windsurf"
	Codex      MCPClient = "codex"
	Gemini     MCPClient = "gemini"
	VSCode     MCPClient = "vscode"
)

// TigerMCPServer represents the Tiger MCP server configuration
type TigerMCPServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// clientConfig represents our own client configuration for Tiger MCP installation
type clientConfig struct {
	ClientType           MCPClient // Our internal client type
	Name                 string
	EditorNames          []string // Supported client names for this client
	MCPServersPathPrefix string   // JSON path prefix for MCP servers config (only for JSON config manipulation clients like Cursor/Windsurf)
	ConfigPaths          []string // Config file locations - used for backup on all clients, and for JSON manipulation on JSON-config clients
	InstallCommand       []string // CLI command to install MCP server (for CLI-based installation clients like Claude Code/Codex/Gemini/VSCode)
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
		InstallCommand: []string{"claude", "mcp", "add", "-s", "user", "tigerdata", "tiger", "mcp", "start"},
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
		InstallCommand: []string{"codex", "mcp", "add", "tigerdata", "tiger", "mcp", "start"},
	},
	{
		ClientType:  Gemini,
		Name:        "Gemini CLI",
		EditorNames: []string{"gemini", "gemini-cli"},
		ConfigPaths: []string{
			"~/.gemini/settings.json",
		},
		InstallCommand: []string{"gemini", "mcp", "add", "-s", "user", "tigerdata", "tiger", "mcp", "start"},
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
		InstallCommand: []string{"code", "--add-mcp", `{"name":"tigerdata","command":"tiger","args":["mcp","start"]}`},
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

// installMCPForClient installs the Tiger MCP server configuration for the specified client
func installMCPForClient(clientName string, createBackup bool, customConfigPath string) error {
	// Find the client configuration by name
	clientCfg, err := findClientConfig(clientName)
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
			return fmt.Errorf("failed to find configuration for %s: %w", clientName, err)
		}
	} else if len(clientCfg.InstallCommand) == 0 {
		// Client has neither ConfigPaths nor InstallCommand
		return fmt.Errorf("client %s has no ConfigPaths or InstallCommand defined", clientName)
	}
	// else: CLI-only client - configPath remains empty, will use InstallCommand

	logging.Info("Installing Tiger MCP server configuration",
		zap.String("client", clientName),
		zap.String("config_path", configPath),
		zap.String("mcp_servers_path", mcpServersPathPrefix),
		zap.Bool("create_backup", createBackup),
	)

	// Create backup if requested and we have a config file
	var backupPath string
	if createBackup && configPath != "" {
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
		// Use JSON patching approach for JSON-config clients
		if err := addTigerMCPServerViaJSON(configPath, mcpServersPathPrefix); err != nil {
			return fmt.Errorf("failed to add Tiger MCP server configuration: %w", err)
		}
	}

	fmt.Printf("✅ Successfully installed Tiger MCP server configuration for %s\n", clientName)
	if configPath != "" {
		fmt.Printf("📁 Configuration file: %s\n", configPath)
	} else {
		fmt.Printf("⚙️  Configuration managed by %s\n", clientName)
	}

	if createBackup && backupPath != "" {
		fmt.Printf("💾 Backup created: %s\n", backupPath)
	}

	fmt.Printf("\n💡 Next steps:\n")
	fmt.Printf("   1. Restart %s to load the new configuration\n", clientName)
	fmt.Printf("   2. The TigerData MCP server will be available as 'tigerdata'\n")
	fmt.Printf("\n🤖 Try asking your AI assistant:\n")
	fmt.Printf("\n   📊 List and manage your TigerData services:\n")
	fmt.Printf("   • \"List my TigerData services\"\n")
	fmt.Printf("   • \"Show me details for service xyz-123\"\n")
	fmt.Printf("   • \"Create a new database service called my-app-db\"\n")
	fmt.Printf("   • \"Update the password for my database service\"\n")
	fmt.Printf("   • \"What TigerData services do I have access to?\"\n")
	fmt.Printf("\n   📚 Ask questions from the PostgreSQL and TigerData documentation:\n")
	fmt.Printf("   • \"Show me TigerData documentation about hypertables?\"\n")
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
	for _, cfg := range supportedClients {
		for _, name := range cfg.EditorNames {
			if strings.ToLower(name) == normalizedName {
				return &cfg, nil
			}
		}
	}

	// Build list of supported clients from our config for error message
	var supportedNames []string
	for _, cfg := range supportedClients {
		supportedNames = append(supportedNames, cfg.EditorNames...)
	}

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
		case "ctrl+c", "q":
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
		case "ctrl+w", "esc":
			// Clear buffer on escape
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

// addTigerMCPServerViaCLI adds Tiger MCP server using a CLI command configured in clientConfig
func addTigerMCPServerViaCLI(clientCfg *clientConfig) error {
	if len(clientCfg.InstallCommand) == 0 {
		return fmt.Errorf("no install command configured for client %s", clientCfg.Name)
	}

	// Get the full path of the currently executing Tiger binary
	tigerPath, err := getTigerExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to get Tiger executable path: %w", err)
	}

	// Build command with full Tiger path replacing "tiger" placeholder
	installCommand := make([]string, len(clientCfg.InstallCommand))
	copy(installCommand, clientCfg.InstallCommand)
	for i, arg := range installCommand {
		if arg == "tiger" {
			installCommand[i] = tigerPath
		} else if strings.Contains(arg, `"command":"tiger"`) {
			// Handle JSON format for VS Code: replace "tiger" in JSON string
			installCommand[i] = strings.Replace(arg, `"command":"tiger"`, fmt.Sprintf(`"command":"%s"`, tigerPath), 1)
		}
	}

	logging.Info("Adding Tiger MCP server using CLI",
		zap.String("client", clientCfg.Name),
		zap.Strings("command", installCommand))

	// Run the configured CLI command
	cmd := exec.Command(installCommand[0], installCommand[1:]...)

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
	if _, err := os.Stat(configPath); errors.Is(err, fs.ErrNotExist) {
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

	// Write backup file
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	logging.Info("Created configuration backup", zap.String("backup_path", backupPath))
	return backupPath, nil
}

// addTigerMCPServerViaJSON adds the Tiger MCP server to the configuration file using JSON patching with file locking
func addTigerMCPServerViaJSON(configPath string, mcpServersPathPrefix string) error {
	// Create configuration directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create configuration directory %s: %w", configDir, err)
	}
	// Get the full path of the currently executing Tiger binary
	tigerPath, err := getTigerExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to get Tiger executable path: %w", err)
	}

	// Tiger MCP server configuration
	tigerServer := TigerMCPServer{
		Command: tigerPath,
		Args:    []string{"mcp", "start"},
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

	// Write back to file
	if err := os.WriteFile(configPath, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logging.Info("Added Tiger MCP server to configuration",
		zap.String("server_name", "tigerdata"),
		zap.String("command", tigerServer.Command),
		zap.Strings("args", tigerServer.Args),
	)

	return nil
}
