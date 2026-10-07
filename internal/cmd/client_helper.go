package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
)

// MCPClient represents our internal client types
type MCPClient string

const (
	ClaudeCode  MCPClient = "claude-code"
	Cursor      MCPClient = "cursor" // Both the IDE and the CLI
	Devin       MCPClient = "devin"
	Codex       MCPClient = "codex"
	Gemini      MCPClient = "gemini"
	VSCode      MCPClient = "vscode"
	Antigravity MCPClient = "antigravity"
	KiroCLI     MCPClient = "kiro-cli"
	Copilot     MCPClient = "copilot" // Both the IDE and the CLI
)

// clientConfig represents a supported client, for both MCP server and skills
// installation
type clientConfig struct {
	ClientType           MCPClient // Our internal client type
	Name                 string
	EditorNames          []string // Supported client names for this client
	MCPServersPathPrefix string   // JSON path prefix for MCP servers config (only for JSON config manipulation clients like Cursor)
	MCPConfigPaths       []string // Config file locations - used for backup on all clients, and for JSON manipulation on JSON-config clients
	// buildMCPInstallCommand builds the CLI install command for CLI-based clients
	// Parameters: serverName (name to register), command (binary path), args (arguments to binary)
	buildMCPInstallCommand func(serverName, command string, args []string) ([]string, error)
	// SkillsDir is the user-level directory skills are installed into.
	SkillsDir string
	// SkillsDirEnv overrides SkillsDir for clients whose skills directory can
	// be relocated by an env var. It's a path containing env var references
	// (e.g. "${CLAUDE_CONFIG_DIR}/skills"), used only when every variable it
	// references is set and non-empty.
	SkillsDirEnv string
}

// BuildMCPInstallCommand constructs the install command with the given parameters
func (c *clientConfig) BuildMCPInstallCommand(serverName, command string, args []string) ([]string, error) {
	if c.buildMCPInstallCommand == nil {
		return nil, nil
	}
	return c.buildMCPInstallCommand(serverName, command, args)
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
		MCPConfigPaths: []string{
			"~/.claude.json",
		},
		buildMCPInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"claude", "mcp", "add", "-s", "user", serverName, command}, args...), nil
		},
		SkillsDir:    "~/.claude/skills",
		SkillsDirEnv: "${CLAUDE_CONFIG_DIR}/skills",
	},
	{
		ClientType:           Cursor,
		Name:                 "Cursor",
		EditorNames:          []string{"cursor"},
		MCPServersPathPrefix: "/mcpServers",
		MCPConfigPaths: []string{
			"~/.cursor/mcp.json",
		},
		SkillsDir: "~/.agents/skills",
	},
	{
		ClientType:  Devin,
		Name:        "Devin",
		EditorNames: []string{"devin"},
		MCPConfigPaths: []string{
			"~/.config/devin/mcp_config.json",
		},
		buildMCPInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"devin", "mcp", "add", "-s", "user", serverName, "--", command}, args...), nil
		},
		SkillsDir: "~/.agents/skills",
	},
	{
		ClientType:  Codex,
		Name:        "Codex",
		EditorNames: []string{"codex"},
		MCPConfigPaths: []string{
			"~/.codex/config.toml",
			"$CODEX_HOME/config.toml",
		},
		buildMCPInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"codex", "mcp", "add", serverName, command}, args...), nil
		},
		SkillsDir: "~/.agents/skills",
	},
	{
		ClientType:  Gemini,
		Name:        "Gemini CLI",
		EditorNames: []string{"gemini", "gemini-cli"},
		MCPConfigPaths: []string{
			"~/.gemini/settings.json",
		},
		buildMCPInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"gemini", "mcp", "add", "-s", "user", serverName, command}, args...), nil
		},
		SkillsDir: "~/.agents/skills",
	},
	{
		ClientType:  VSCode,
		Name:        "VS Code",
		EditorNames: []string{"vscode", "code", "vs-code"},
		MCPConfigPaths: []string{
			"~/.config/Code/User/mcp.json",
			"~/Library/Application Support/Code/User/mcp.json",
			"~/AppData/Roaming/Code/User/mcp.json",
		},
		buildMCPInstallCommand: func(serverName, command string, args []string) ([]string, error) {
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
		SkillsDir: "~/.agents/skills",
	},
	{
		ClientType:           Antigravity,
		Name:                 "Google Antigravity",
		EditorNames:          []string{"antigravity", "agy"},
		MCPServersPathPrefix: "/mcpServers",
		MCPConfigPaths: []string{
			"~/.gemini/antigravity/mcp_config.json",
		},
		SkillsDir: "~/.gemini/antigravity/skills",
	},
	{
		ClientType:  KiroCLI,
		Name:        "Kiro CLI",
		EditorNames: []string{"kiro-cli"},
		MCPConfigPaths: []string{
			"~/.kiro/settings/mcp.json",
		},
		buildMCPInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return []string{"kiro-cli", "mcp", "add", "--name", serverName, "--command", command, "--args", strings.Join(args, ",")}, nil
		},
		SkillsDir:    "~/.kiro/skills",
		SkillsDirEnv: "${KIRO_HOME}/skills",
	},
	{
		ClientType:  Copilot,
		Name:        "GitHub Copilot CLI",
		EditorNames: []string{"copilot", "copilot-cli"},
		MCPConfigPaths: []string{
			"~/.copilot/mcp-config.json",
		},
		buildMCPInstallCommand: func(serverName, command string, args []string) ([]string, error) {
			return append([]string{"copilot", "mcp", "add", serverName, "--", command}, args...), nil
		},
		SkillsDir: "~/.agents/skills",
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

// ClientOption represents a client choice for interactive selection
type ClientOption struct {
	Name       string // Display name
	ClientName string // Client name to pass to installMCPForClient
}

// selectClientInteractively prompts the user to select a client using Bubble Tea
func selectClientInteractively(cmd *cobra.Command, title string) (string, error) {
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
		title:   title,
		options: options,
		cursor:  0,
	}

	program := tea.NewProgram(model,
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.ErrOrStderr()),
		tea.WithContext(cmd.Context()),
		tea.WithoutSignalHandler())
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
	title        string
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
	case tea.KeyPressMsg:
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
		case "enter", "space":
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

func (m clientSelectModel) View() tea.View {
	var s strings.Builder
	s.WriteString(m.title + "\n\n")

	for i, option := range m.options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s.WriteString(fmt.Sprintf("%s %d. %s\n", cursor, i+1, option.Name))
	}

	// Show the current number buffer if user is typing
	if m.numberBuffer != "" {
		s.WriteString(fmt.Sprintf("\nTyping: %s", m.numberBuffer))
	}

	s.WriteString("\nUse ↑/↓ arrows or number keys to navigate, enter to select, q to quit")
	return tea.NewView(s.String())
}
