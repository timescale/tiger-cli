## tiger mcp

Tiger Model Context Protocol (MCP) server

### Synopsis

Tiger Model Context Protocol (MCP) server for AI assistant integration.

The MCP server provides programmatic access to Tiger Cloud platform resources
through Claude and other AI assistants. It exposes Tiger CLI functionality as MCP
tools that can be called by AI agents.

Configuration:
The server automatically uses the CLI's stored authentication and configuration.
No additional setup is required beyond running 'tiger auth login'.

Use 'tiger mcp start' to launch the MCP server.

```
tiger mcp [flags]
```

### Options

```
  -h, --help   help for mcp
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

* [tiger](tiger.md)	 - Tiger CLI - Tiger Cloud Platform command-line interface
* [tiger mcp get](tiger_mcp_get.md)	 - Get detailed information about a specific MCP capability
* [tiger mcp install](tiger_mcp_install.md)	 - Install and configure Tiger MCP server for a client
* [tiger mcp list](tiger_mcp_list.md)	 - List available MCP tools, prompts, and resources
* [tiger mcp start](tiger_mcp_start.md)	 - Start the Tiger MCP server
