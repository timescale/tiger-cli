## tiger db uri

Get connection URI for a service

### Synopsis

Get a PostgreSQL connection URI for connecting to a database service.

The service ID can be provided as an argument or will use the default service
from your configuration. The connection string includes all necessary parameters
for establishing a database connection to the TimescaleDB/PostgreSQL service.

You can also pass a read replica set ID to get a connection string for that replica.

By default, passwords are excluded from the connection string for security.
Use --with-password to include the password directly in the connection string.

Use --read-only to emit a connection string that opens the session in Tiger
Cloud's immutable read-only mode (writes and DDL are rejected by the server).
The global read_only config option (or TIGER_READ_ONLY) also forces this
behavior: read_only=all makes every connection string read-only, and
read_only=prod makes those for services tagged PROD read-only while leaving DEV
services writable.

```
tiger db uri [service-id] [flags]
```

### Examples

```
  # Get connection string for default service
  tiger db uri

  # Get connection string for specific service
  tiger db uri svc-12345

  # Get pooled connection string (uses connection pooler if available)
  tiger db uri svc-12345 --pooled

  # Get connection string with custom role/username
  tiger db uri svc-12345 --role readonly

  # Get a read-only connection string
  tiger db uri svc-12345 --read-only

  # Get connection string with password included (less secure)
  tiger db uri svc-12345 --with-password
```

### Options

```
  -h, --help            help for uri
      --pooled          Use connection pooling
      --read-only       Open the connection in Tiger Cloud's immutable read-only mode
      --role string     Database role/username (default "tsdbadmin")
      --with-password   Include password in connection string (less secure)
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
