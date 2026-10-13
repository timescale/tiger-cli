## tiger mcp install

Install and configure Tiger MCP server for clients

### Synopsis

Install and configure the Tiger MCP server for one or more MCP clients or AI
assistants.

The server is added to each client's user-level MCP configuration file, so it's
available in all projects. Other servers configured there are left alone.

Supported Clients:
  claude-code              Configure for Claude Code
  cursor                   Configure for Cursor
  devin                    Configure for Devin
  codex                    Configure for Codex
  gemini                   Configure for Gemini CLI
  vscode                   Configure for VS Code
  antigravity              Configure for Google Antigravity
  kiro-cli                 Configure for Kiro CLI
  copilot                  Configure for GitHub Copilot CLI

With no arguments, you're prompted to select clients interactively, starting
from the clients the server is already installed for.

Re-running the command replaces an existing Tiger MCP server entry with the
current one. Each configuration file is backed up before it's changed, unless
--no-backup is given.

```
tiger mcp install [client...] [flags]
```

### Examples

```
  # Interactive client selection
  tiger mcp install

  # Install for Claude Code and Cursor
  tiger mcp install claude-code cursor

  # Install without creating a backup
  tiger mcp install claude-code --no-backup

  # Use a custom configuration file path
  tiger mcp install claude-code --config-path ~/custom/config.json
```

### Options

```
      --config-path string   Custom path to configuration file (overrides default locations; requires a single client)
  -h, --help                 help for install
      --no-backup            Skip creating backup of existing configuration (default: create backup)
```

### Options inherited from parent commands

```
      --analytics                 enable/disable usage analytics (default true)
      --color                     enable colored output (default true)
      --config-dir string         config directory (default "~/.config/tiger")
      --password-storage string   password storage method (keyring, pgpass, none) (default "keyring")
      --service-id string         service ID
      --version-check             check for updates on startup (default true)
```

### SEE ALSO

* [tiger mcp](tiger_mcp.md)	 - Tiger Model Context Protocol (MCP) server
