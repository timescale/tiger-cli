## tiger db ping

Test database connectivity

### Synopsis

Test database connectivity to a service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command tests if the database is accepting
connections and returns appropriate exit codes following pg_isready conventions.

You can also pass a read replica set ID to test connectivity to that replica.

Return Codes:
  0: Server is accepting connections normally
  1: Server is rejecting connections (e.g., during startup)
  2: No response to connection attempt (server unreachable)
  3: No attempt made (e.g., invalid parameters)

```
tiger db ping [service-id] [flags]
```

### Examples

```
  # Test connection to default service
  tiger db ping

  # Test connection to specific service
  tiger db ping svc-12345

  # Test connection with custom timeout (10 seconds)
  tiger db ping svc-12345 --timeout 10s

  # Test connection with longer timeout (5 minutes)
  tiger db ping svc-12345 --timeout 5m

  # Test connection with no timeout (wait indefinitely)
  tiger db ping svc-12345 --timeout 0
```

### Options

```
  -h, --help               help for ping
      --pooled             Use connection pooling
      --role string        Database role/username (default "tsdbadmin")
  -t, --timeout duration   Timeout duration (e.g., 30s, 5m, 1h). Use 0 for no timeout (default 3s)
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

* [tiger db](tiger_db.md)	 - Database operations and management
