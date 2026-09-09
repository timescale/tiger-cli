## tiger service logs

View logs for a service

### Synopsis

View logs for a database service.

Fetches and displays logs from the specified service. By default, shows the last
100 log entries. Supports filtering by time range.

The service ID can be provided as an argument or will use the default service
from your configuration.

Examples:
  # View last 100 logs for default service (default behavior)
  tiger service logs

  # View logs for specific service
  tiger service logs svc-12345

  # View logs within a time range
  tiger service logs --since "2024-01-15T09:00:00Z" --until "2024-01-15T10:00:00Z"

  # View logs for a specific node (for services with HA replicas)
  tiger service logs --node 1

  # View last 50 lines
  tiger service logs --tail 50

  # View last 1000 lines
  tiger service logs --tail 1000

```
tiger service logs [service-id] [flags]
```

### Options

```
  -h, --help            help for logs
      --node int        Specific service node to fetch logs from (for services with HA replicas, 0 is valid)
  -o, --output string   Output format (text, json, yaml)
      --since time      Fetch logs after this timestamp (RFC3339 format, e.g., 2024-01-15T09:00:00Z)
      --tail int        Number of log lines to show (default 100)
      --until time      Fetch logs before this timestamp (RFC3339 format, e.g., 2024-01-15T10:00:00Z)
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

* [tiger service](tiger_service.md)	 - Manage database services
