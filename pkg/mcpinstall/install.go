// Package mcpinstall provides a public API for installing MCP server configurations
// for various AI coding assistants and editors.
package mcpinstall

import (
	"github.com/timescale/tiger-cli/internal/tiger/cmd"
)

// Options configures the MCP server installation behavior.
type Options = cmd.InstallOptions

// InstallForClient installs an MCP server configuration for the specified client.
//
// Parameters:
//   - clientName: The name of the client to configure (e.g., "claude-code", "cursor", "windsurf")
//   - opts: Configuration options for the installation
//
// Required options:
//   - ServerName: The name to register the MCP server as (e.g., "my-mcp-server")
//   - Command: Path to the MCP server binary (e.g., "/usr/local/bin/my-server")
//   - Args: Arguments to pass to the MCP server binary (e.g., []string{"serve", "--port", "8080"})
//
// Optional fields:
//   - CreateBackup: If true, creates a backup of the existing config file before modification
//   - CustomConfigPath: Custom path to the config file (empty string uses default location)
//
// Supported clients: claude-code, cursor, windsurf, codex, gemini, vscode, antigravity, kiro-cli
//
// Returns an error if the client is not supported, required options are missing, or installation fails.
func InstallForClient(clientName string, opts Options) error {
	return cmd.InstallMCPForClient(clientName, opts)
}
