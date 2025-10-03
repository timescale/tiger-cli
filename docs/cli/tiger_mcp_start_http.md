## tiger mcp start http

Start MCP server with HTTP transport

### Synopsis

Start the MCP server using HTTP transport.

The server will automatically find an available port if the specified port is busy.

Examples:
  # Start HTTP server on default port 8080
  tiger mcp start http

  # Start HTTP server on custom port
  tiger mcp start http --port 3001

  # Start HTTP server on all interfaces
  tiger mcp start http --host 0.0.0.0 --port 8080

  # Start server and bind to specific interface
  tiger mcp start http --host 192.168.1.100 --port 9000

```
tiger mcp start http [flags]
```

### Options

```
  -h, --help          help for http
      --host string   Host to bind to (default "localhost")
      --port int      Port to run HTTP server on (default 8080)
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

* [tiger mcp start](tiger_mcp_start.md)	 - Start the Tiger MCP server

