## tiger db query

Execute a SQL query on a database

### Synopsis

Execute a SQL query against a database service and display the results.

Unlike 'tiger db psql', this runs the query directly and does not require a
local psql installation.

The service ID can be provided as an argument or will use the default service
from your configuration. You can also pass a read replica set ID to query that
replica.

The query comes from --command, from the SQL file named by --file, or, if
neither is given, from stdin.

Multi-statement queries (semicolon-separated) are supported. Results from all
statements that return rows are displayed. The statements run in an implicit
transaction that commits on success and rolls back on error; a transaction
opened with BEGIN must be committed explicitly or it rolls back when the
connection closes.

Use --read-only to open the session in Tiger Cloud's immutable read-only mode
(writes and DDL are rejected by the server). The global read_only config option
(or TIGER_READ_ONLY) also forces this behavior: read_only=all makes every session
read-only, and read_only=prod makes sessions against services tagged PROD
read-only while leaving DEV services writable.

Examples:
  # Select data from a table
  tiger db query svc-12345 -c "SELECT * FROM users LIMIT 5"

  # Query the default service
  tiger db query -c "SELECT now()"

  # Execute DDL
  tiger db query svc-12345 -c "CREATE TABLE todos (id SERIAL PRIMARY KEY, title TEXT)"

  # Multi-statement query
  tiger db query svc-12345 -c "INSERT INTO users (name) VALUES ('alice'); SELECT * FROM users"

  # Run a SQL file
  tiger db query svc-12345 -f schema.sql

  # Read the query from stdin
  echo "SELECT 1" | tiger db query svc-12345
  tiger db query svc-12345 < schema.sql

  # Get the results as JSON
  tiger db query svc-12345 -c "SELECT * FROM users" -o json

  # Query a read replica
  tiger db query rep1234567 -c "SELECT count(*) FROM events"

```
tiger db query [service-id] [flags]
```

### Options

```
  -c, --command string     SQL query to execute (reads from stdin if neither --command nor --file is given)
  -f, --file string        Path to a SQL file to execute
  -h, --help               help for query
  -o, --output string      Output format (table, json, yaml)
      --pooled             Use connection pooling
      --read-only          Open the connection in Tiger Cloud's immutable read-only mode
      --role string        Database role/username (default "tsdbadmin")
      --timeout duration   Query timeout duration (e.g., 30s, 5m). Use 0 for no timeout
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
