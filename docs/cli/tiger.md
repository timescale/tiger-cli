## tiger

Tiger CLI - Tiger Cloud Platform command-line interface

### Synopsis

Tiger CLI is a command-line interface for managing Tiger Cloud platform resources.
It provides comprehensive tools for managing the lifecycle of database services,
as well as for connecting to and querying them. It also includes Tiger MCP, a
Model Context Protocol server that exposes the same operations to AI assistants.

To get started, run:

  tiger auth login

### Options

```
      --analytics                 enable/disable usage analytics (default true)
      --color                     enable colored output (default true)
      --config-dir string         config directory (default "~/.config/tiger")
  -h, --help                      help for tiger
      --password-storage string   password storage method (keyring, pgpass, none) (default "keyring")
      --service-id string         service ID
      --version-check             check for updates on startup (default true)
```

### SEE ALSO

* [tiger auth](tiger_auth.md)	 - Manage authentication and credentials
* [tiger config](tiger_config.md)	 - Manage CLI configuration
* [tiger db](tiger_db.md)	 - Database operations and management
* [tiger feedback](tiger_feedback.md)	 - Submit feedback or a bug report
* [tiger mcp](tiger_mcp.md)	 - Tiger Model Context Protocol (MCP) server
* [tiger project](tiger_project.md)	 - Manage Tiger Cloud projects
* [tiger service](tiger_service.md)	 - Manage database services
* [tiger upgrade](tiger_upgrade.md)	 - Upgrade the Tiger CLI to the latest version
* [tiger version](tiger_version.md)	 - Show version information
