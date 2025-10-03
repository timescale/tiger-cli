## tiger mcp install

Install and configure Tiger MCP server for a client

### Synopsis

Install and configure the Tiger MCP server for a specific MCP client or AI assistant.

This command automates the configuration process by modifying the appropriate
configuration files for the specified client.

Supported Clients:
  claude-code              Configure for Claude Code
  cursor                   Configure for Cursor
  windsurf                 Configure for Windsurf
  codex                    Configure for Codex
  gemini                   Configure for Gemini CLI
  vscode                   Configure for VS Code

The command will:
- Automatically detect the appropriate configuration file location
- Create the configuration directory if it doesn't exist
- Create a backup of existing configuration by default
- Merge with existing MCP server configurations (doesn't overwrite other servers)
- Validate the configuration after installation

If no client is specified, you'll be prompted to select one interactively.

Examples:
  # Interactive client selection
  tiger mcp install

  # Install for Claude Code (User scope - available in all projects)
  tiger mcp install claude-code

  # Install for Cursor IDE
  tiger mcp install cursor

  # Install without creating backup
  tiger mcp install claude-code --no-backup

  # Use custom configuration file path
  tiger mcp install claude-code --config-path ~/custom/config.json

```
tiger mcp install [client] [flags]
```

### Options

```
      --config-path string   Custom path to configuration file (overrides default locations)
  -h, --help                 help for install
      --no-backup            Skip creating backup of existing configuration (default: create backup)
```

### Options inherited from parent commands

```
      --analytics                 enable/disable usage analytics (default true)
      --config-dir string         config directory (default "/Users/nathan/.config/tiger")
      --debug                     enable debug logging
  -o, --output string             output format (json, yaml, table)
      --password-storage string   password storage method (keyring, pgpass, none) (default "keyring")
      --project-id string         project ID
      --service-id string         service ID
```

### SEE ALSO

* [tiger mcp](tiger_mcp.md)	 - Tiger Model Context Protocol (MCP) server

