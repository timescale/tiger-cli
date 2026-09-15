## tiger db psql

Connect to a database with psql

### Synopsis

Connect to a database service using psql client.

The service ID can be provided as an argument or will use the default service
from your configuration. This command will launch an interactive psql session
with the appropriate connection parameters.

Authentication is handled automatically using:
1. Stored password (keyring, ~/.pgpass, or none based on --password-storage setting)
2. PGPASSWORD environment variable
3. If authentication fails, offers interactive options:
   - Enter password manually (will be saved for future use)
   - Reset password (update or generates a new password via the API)

Use --read-only to open the psql session in Tiger Cloud's immutable read-only
mode (writes and DDL are rejected by the server). The global read_only config
option (or TIGER_READ_ONLY) also forces this behavior: read_only=all makes every
session read-only, and read_only=prod makes sessions against services tagged PROD
read-only while leaving DEV services writable.

When run in an interactive terminal, this command checks whether the service has
any read replicas. If it does, it offers to connect to one of them instead of the
primary. Use --no-replica-prompt to skip this prompt and always connect to the
requested service. The prompt is automatically skipped when stdin is not a
terminal (e.g. in scripts) or when the service has no read replicas.

You can also pass a read replica set ID to connect straight to that replica,
skipping the prompt. Read replicas share the primary's credentials.

```
tiger db psql [service-id] [flags]
```

### Examples

```
  # Connect to default service
  tiger db psql

  # Connect directly to a read replica by its ID
  tiger db psql rep1234567

  # Connect without the read replica prompt
  tiger db psql svc-12345 --no-replica-prompt

  # Connect to specific service
  tiger db psql svc-12345

  # Connect using connection pooler
  tiger db psql svc-12345 --pooled

  # Connect with custom role/username
  tiger db psql svc-12345 --role readonly

  # Connect in read-only mode (writes and DDL are rejected by the server)
  tiger db psql svc-12345 --read-only

  # Pass additional flags to psql (use -- to separate)
  tiger db psql svc-12345 -- --single-transaction --quiet
  tiger db psql svc-12345 -- -c "SELECT version();" --no-psqlrc
```

### Options

```
  -h, --help                help for psql
      --no-replica-prompt   Don't prompt to connect to a read replica
      --pooled              Use connection pooling
      --read-only           Open the connection in Tiger Cloud's immutable read-only mode
      --role string         Database role/username (default "tsdbadmin")
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
