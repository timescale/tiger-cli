// Package mcpinstall provides a public API for installing MCP server configurations
// for various AI coding assistants and editors.
package mcpinstall

import (
	"github.com/timescale/tiger-cli/internal/tiger/cmd"
)

// Options configures the MCP server installation behavior.
type Options = cmd.InstallOptions

// ClientInfo contains information about a supported MCP client.
type ClientInfo = cmd.ClientInfo

// SupportedClients returns information about all supported MCP clients.
// Use this to get valid values for Options.ClientName.
func SupportedClients() []ClientInfo {
	return cmd.SupportedClients()
}

// Install installs an MCP server configuration for the specified client.
//
// Required options:
//   - ClientName: The name of the client to configure (e.g., "claude-code", "cursor", "windsurf")
//   - ServerName: The name to register the MCP server as (e.g., "my-mcp-server")
//   - Command: Path to the MCP server binary (e.g., "/usr/local/bin/my-server")
//   - Args: Arguments to pass to the MCP server binary (e.g., []string{"serve", "--port", "8080"})
//
// Optional fields:
//   - CreateBackup: If true, creates a backup of the existing config file before modification
//   - CustomConfigPath: Custom path to the config file (empty string uses default location)
//
// Returns an error if the client is not supported, required options are missing, or installation fails.
func Install(opts Options) error {
	return cmd.InstallMCPForClient(opts)
}
