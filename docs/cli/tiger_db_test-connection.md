## tiger db test-connection

Test database connectivity

### Synopsis

Test database connectivity to a service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command tests if the database is accepting
connections and returns appropriate exit codes following pg_isready conventions.

Return Codes:
  0: Server is accepting connections normally
  1: Server is rejecting connections (e.g., during startup)
  2: No response to connection attempt (server unreachable)
  3: No attempt made (e.g., invalid parameters)

Examples:
  # Test connection to default service
  tiger db test-connection

  # Test connection to specific service
  tiger db test-connection svc-12345

  # Test connection with custom timeout (10 seconds)
  tiger db test-connection svc-12345 --timeout 10s

  # Test connection with longer timeout (5 minutes)
  tiger db test-connection svc-12345 --timeout 5m

  # Test connection with no timeout (wait indefinitely)
  tiger db test-connection svc-12345 --timeout 0

```
tiger db test-connection [service-id] [flags]
```

### Options

```
  -h, --help               help for test-connection
      --pooled             Use connection pooling
      --role string        Database role/username (default "tsdbadmin")
  -t, --timeout duration   Timeout duration (e.g., 30s, 5m, 1h). Use 0 for no timeout (default 3s)
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

* [tiger db](tiger_db.md)	 - Database operations and management

