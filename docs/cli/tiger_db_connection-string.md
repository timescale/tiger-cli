## tiger db connection-string

Get connection string for a service

### Synopsis

Get a PostgreSQL connection string for connecting to a database service.

The service ID can be provided as an argument or will use the default service
from your configuration. The connection string includes all necessary parameters
for establishing a database connection to the TimescaleDB/PostgreSQL service.

By default, passwords are excluded from the connection string for security.
Use --with-password to include the password directly in the connection string.

Examples:
  # Get connection string for default service
  tiger db connection-string

  # Get connection string for specific service
  tiger db connection-string svc-12345

  # Get pooled connection string (uses connection pooler if available)
  tiger db connection-string svc-12345 --pooled

  # Get connection string with custom role/username
  tiger db connection-string svc-12345 --role readonly

  # Get connection string with password included (less secure)
  tiger db connection-string svc-12345 --with-password

```
tiger db connection-string [service-id] [flags]
```

### Options

```
  -h, --help            help for connection-string
      --pooled          Use connection pooling
      --role string     Database role/username (default "tsdbadmin")
      --with-password   Include password in connection string (less secure)
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

