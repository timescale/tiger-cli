package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
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
	// SkillsDir is the user-level skills directory of a client that doesn't
	// read the universal ~/.agents/skills. Empty for clients that do.
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
	},
	{
		ClientType:           Antigravity,
		Name:                 "Google Antigravity",
		EditorNames:          []string{"antigravity", "agy"},
		MCPServersPathPrefix: "/mcpServers",
		MCPConfigPaths: []string{
			"~/.gemini/config/mcp_config.json",
		},
		SkillsDir: "~/.gemini/config/skills",
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
	},
}
