## tiger db schema

Display database schema information

### Synopsis

Display the schema of a database service: tables (regular, partitioned, and
foreign), views, materialized views, enum types, functions, procedures,
indexes, triggers, and TimescaleDB hypertable and continuous aggregate
metadata.

The service ID can be provided as an argument or will use the default service
from your configuration. You can also pass a read replica set ID to introspect
that replica. Only objects the connecting role can access are returned. The
connection is opened in Tiger Cloud's immutable read-only mode.

By default only user-facing schemas and objects are shown. View and routine
definitions and object comments are omitted unless requested, since they can be
large and may embed implementation details.

```
tiger db schema [service-id] [flags]
```

### Examples

```
  # Show the schema of the default service
  tiger db schema

  # Show the schema of a specific service
  tiger db schema svc-12345

  # Restrict to a single schema
  tiger db schema svc-12345 --schema public

  # Include view/function definitions and comments
  tiger db schema svc-12345 --definitions --comments

  # Include catalog, TimescaleDB internals, and extension-owned objects
  tiger db schema svc-12345 --internal
```

### Options

```
      --comments        Include object comments (COMMENT ON text)
      --definitions     Include full object definitions (view SELECTs, function/procedure bodies)
  -h, --help            help for schema
      --internal        Include system schemas (pg_*, information_schema, TimescaleDB internals) and extension-owned objects
      --pooled          Use connection pooling
      --role string     Database role/username (default "tsdbadmin")
      --schema string   Restrict output to a single schema
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
