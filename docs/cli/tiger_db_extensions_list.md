## tiger db extensions list

List the PostgreSQL extensions installed in a database

### Synopsis

List the PostgreSQL extensions installed in a database service, with each
extension's version, the schema it lives in, and its description.

The service can be given by ID or name as an argument, or will use the default
service from your configuration. You can also pass a read replica set ID to
list the extensions on that replica. The connection is opened in Tiger Cloud's
immutable read-only mode.

```
tiger db extensions list [name-or-id] [flags]
```

### Examples

```
  # List the extensions installed in the default service
  tiger db extensions list

  # List the extensions installed in a specific service
  tiger db extensions list svc-12345

  # Output as JSON
  tiger db extensions list svc-12345 -o json
```

### Options

```
  -h, --help            help for list
  -o, --output string   Output format (table, json, yaml)
      --role string     Database role/username (default "tsdbadmin")
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

* [tiger db extensions](tiger_db_extensions.md)	 - Manage PostgreSQL extensions
