package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/timescale/tiger-cli/internal/util"
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
	ClientType  MCPClient // Our internal client type
	Name        string
	EditorNames []string // Supported client names for this client
	// MCPConfig is the client's user-level MCP config file.
	MCPConfig mcpConfigFile
	// SkillsDir is the user-level skills directory of a client that doesn't
	// read the universal ~/.agents/skills. Empty for clients that do.
	SkillsDir string
	// SkillsDirEnv overrides SkillsDir for clients whose skills directory can
	// be relocated by an env var. It's a path containing env var references
	// (e.g. "${CLAUDE_CONFIG_DIR}/skills"), used only when every variable it
	// references is set and non-empty, and it expands to an absolute path.
	SkillsDirEnv string
}

// mcpConfigFile describes where a client keeps its user-level MCP server
// configuration, and what a server entry in it looks like. The file is JSON,
// or TOML if Path ends in .toml (even when the file is somewhere else).
type mcpConfigFile struct {
	// Path is the file's location.
	Path string
	// PathEnv overrides Path when the client's config can be relocated by an
	// env var, the same way SkillsDirEnv overrides SkillsDir.
	PathEnv string
	// MacPath and WindowsPath replace Path and PathEnv on macOS and Windows,
	// for clients that keep the file somewhere else there. They may reference
	// env vars, which must be set.
	MacPath     string
	WindowsPath string
	// ServersKey is the top-level key holding the servers, keyed by name.
	ServersKey string
	// Entry holds the fields a server entry needs besides its command and
	// args, which are filled in at install time.
	Entry mcpServerEntry
}

// mcpServerEntry is a server entry in an MCP config file. Field order is the
// order written.
type mcpServerEntry struct {
	Type      string   `json:"type,omitempty"`
	Transport string   `json:"transport,omitempty"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Tools     []string `json:"tools,omitempty"`
}

// goos is the operating system that picks between an mcpConfigFile's paths.
// It's a var so tests can check every platform's paths.
var goos = runtime.GOOS

// supportedClients defines the clients we support for Tiger MCP installation.
// Each client's config file location and entry format match what its own
// `mcp add` command writes.
var supportedClients = []clientConfig{
	{
		ClientType:  ClaudeCode,
		Name:        "Claude Code",
		EditorNames: []string{"claude-code"},
		MCPConfig: mcpConfigFile{
			Path:       "~/.claude.json",
			PathEnv:    "${CLAUDE_CONFIG_DIR}/.claude.json",
			ServersKey: "mcpServers",
			Entry:      mcpServerEntry{Type: "stdio"},
		},
		SkillsDir:    "~/.claude/skills",
		SkillsDirEnv: "${CLAUDE_CONFIG_DIR}/skills",
	},
	{
		ClientType:  Cursor,
		Name:        "Cursor",
		EditorNames: []string{"cursor"},
		MCPConfig: mcpConfigFile{
			Path:       "~/.cursor/mcp.json",
			ServersKey: "mcpServers",
		},
	},
	{
		ClientType:  Devin,
		Name:        "Devin",
		EditorNames: []string{"devin"},
		MCPConfig: mcpConfigFile{
			Path:        "~/.config/devin/mcp_config.json",
			PathEnv:     "${XDG_CONFIG_HOME}/devin/mcp_config.json",
			WindowsPath: "${APPDATA}/devin/mcp_config.json",
			ServersKey:  "mcpServers",
			Entry:       mcpServerEntry{Transport: "stdio"},
		},
	},
	{
		ClientType:  Codex,
		Name:        "Codex",
		EditorNames: []string{"codex"},
		MCPConfig: mcpConfigFile{
			Path:       "~/.codex/config.toml",
			PathEnv:    "${CODEX_HOME}/config.toml",
			ServersKey: "mcp_servers",
		},
	},
	{
		ClientType:  Gemini,
		Name:        "Gemini CLI",
		EditorNames: []string{"gemini", "gemini-cli"},
		MCPConfig: mcpConfigFile{
			Path:       "~/.gemini/settings.json",
			PathEnv:    "${GEMINI_CLI_HOME}/.gemini/settings.json",
			ServersKey: "mcpServers",
		},
	},
	{
		ClientType:  VSCode,
		Name:        "VS Code",
		EditorNames: []string{"vscode", "code", "vs-code"},
		MCPConfig: mcpConfigFile{
			Path:        "~/.config/Code/User/mcp.json",
			PathEnv:     "${XDG_CONFIG_HOME}/Code/User/mcp.json",
			MacPath:     "~/Library/Application Support/Code/User/mcp.json",
			WindowsPath: "${APPDATA}/Code/User/mcp.json",
			ServersKey:  "servers",
		},
	},
	{
		ClientType:  Antigravity,
		Name:        "Google Antigravity",
		EditorNames: []string{"antigravity", "agy"},
		MCPConfig: mcpConfigFile{
			Path:       "~/.gemini/config/mcp_config.json",
			ServersKey: "mcpServers",
		},
		SkillsDir: "~/.gemini/config/skills",
	},
	{
		ClientType:  KiroCLI,
		Name:        "Kiro CLI",
		EditorNames: []string{"kiro-cli"},
		MCPConfig: mcpConfigFile{
			Path:       "~/.kiro/settings/mcp.json",
			PathEnv:    "${KIRO_HOME}/settings/mcp.json",
			ServersKey: "mcpServers",
		},
		SkillsDir:    "~/.kiro/skills",
		SkillsDirEnv: "${KIRO_HOME}/skills",
	},
	{
		ClientType:  Copilot,
		Name:        "GitHub Copilot CLI",
		EditorNames: []string{"copilot", "copilot-cli"},
		MCPConfig: mcpConfigFile{
			Path:       "~/.copilot/mcp-config.json",
			PathEnv:    "${COPILOT_HOME}/mcp-config.json",
			ServersKey: "mcpServers",
			Entry: mcpServerEntry{
				Type:  "local",
				Tools: []string{"*"},
			},
		},
	},
}

// isTOML reports whether the file is TOML rather than JSON.
func (f mcpConfigFile) isTOML() bool {
	return strings.EqualFold(filepath.Ext(f.Path), ".toml")
}

// resolvePath returns the config file's absolute location on this platform.
func (f mcpConfigFile) resolvePath() (string, error) {
	switch {
	case goos == "darwin" && f.MacPath != "":
		return resolveClientPath(f.MacPath, "")
	case goos == "windows" && f.WindowsPath != "":
		return resolveClientPath(f.WindowsPath, "")
	}
	return resolveClientPath(f.Path, f.PathEnv)
}

// resolveClientPath returns the absolute location of a client's file or
// directory: pathEnv expanded, if every env var it references is set and
// non-empty and it expands to an absolute path (there's no telling what a
// relative one is relative to), or path otherwise.
func resolveClientPath(path, pathEnv string) (string, error) {
	if pathEnv != "" {
		if expanded, ok := expandEnvStrict(pathEnv); ok && filepath.IsAbs(expanded) {
			return filepath.Clean(expanded), nil
		}
	}
	if _, ok := expandEnvStrict(path); !ok {
		return "", fmt.Errorf("%s references an unset environment variable", path)
	}
	// ExpandPath leaves a ~ in place if the home directory is unknown, which
	// would otherwise resolve relative to the working directory.
	resolved := util.ExpandPath(path)
	if !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("%s is not an absolute path", resolved)
	}
	return filepath.Clean(resolved), nil
}

// expandEnvStrict expands env var references in s, reporting false if any
// referenced variable is unset or empty.
func expandEnvStrict(s string) (string, bool) {
	ok := true
	expanded := os.Expand(s, func(name string) string {
		v := os.Getenv(name)
		if v == "" {
			ok = false
		}
		return v
	})
	return expanded, ok
}
