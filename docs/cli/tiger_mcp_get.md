## tiger mcp get

Get detailed information about a specific MCP capability

### Synopsis

Get detailed information about a specific MCP tool, prompt, resource, or resource template.

```
tiger mcp get <name> [flags]
```

### Examples

```
  # Get details about a tool
  tiger mcp get service_create

  # Get details about a prompt
  tiger mcp get setup-timescaledb-hypertables

  # Get details as JSON
  tiger mcp get service_create -o json

  # Get details as YAML
  tiger mcp get service_create -o yaml
```

### Options

```
  -h, --help            help for get
  -o, --output string   output format (json, yaml, table)
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
