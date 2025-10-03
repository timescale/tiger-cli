## tiger mcp start

Start the Tiger MCP server

### Synopsis

Start the Tiger Model Context Protocol (MCP) server for AI assistant integration.

The MCP server provides programmatic access to TigerData Cloud Platform resources
through Claude and other AI assistants. By default, it uses stdio transport.

Examples:
  # Start with stdio transport (default)
  tiger mcp start

  # Start with stdio transport (explicit)
  tiger mcp start stdio

  # Start with HTTP transport
  tiger mcp start http

```
tiger mcp start [flags]
```

### Options

```
  -h, --help   help for start
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
* [tiger mcp start http](tiger_mcp_start_http.md)	 - Start MCP server with HTTP transport
* [tiger mcp start stdio](tiger_mcp_start_stdio.md)	 - Start MCP server with stdio transport

