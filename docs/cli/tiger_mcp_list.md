## tiger mcp list

List available MCP tools, prompts, and resources

### Synopsis

List all MCP tools, prompts, and resources exposed via the Tiger MCP server.

The output can be formatted as a table, JSON, or YAML.

```
tiger mcp list [flags]
```

### Examples

```
  # List all capabilities in table format (default)
  tiger mcp list

  # List as JSON
  tiger mcp list -o json

  # List as YAML
  tiger mcp list -o yaml
```

### Options

```
  -h, --help            help for list
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
