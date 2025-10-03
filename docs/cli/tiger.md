## tiger

Tiger CLI - TigerData Cloud Platform command-line interface

### Synopsis

Tiger CLI is a command-line interface for managing TigerData Cloud Platform resources.
Built as a single Go binary, it provides comprehensive tools for managing database services,
VPCs, replicas, and related infrastructure components.

To get started, run:

tiger auth login



### Options

```
      --analytics                 enable/disable usage analytics (default true)
      --config-dir string         config directory (default "/Users/nathan/.config/tiger")
      --debug                     enable debug logging
  -h, --help                      help for tiger
  -o, --output string             output format (json, yaml, table)
      --password-storage string   password storage method (keyring, pgpass, none) (default "keyring")
      --project-id string         project ID
      --service-id string         service ID
```

### SEE ALSO

* [tiger auth](tiger_auth.md)	 - Manage authentication and credentials
* [tiger config](tiger_config.md)	 - Manage CLI configuration
* [tiger db](tiger_db.md)	 - Database operations and management
* [tiger mcp](tiger_mcp.md)	 - Tiger Model Context Protocol (MCP) server
* [tiger service](tiger_service.md)	 - Manage database services
* [tiger version](tiger_version.md)	 - Show version information

