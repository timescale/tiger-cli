## tiger service rename

Rename a database service

### Synopsis

Rename a database service.

Only the service's display name changes: its ID, endpoints, and data are
untouched, so existing connections and connection strings keep working.

Both the service and the new name are required. There is no default service
fallback, since a single argument would be ambiguous between the service to
rename and the name to give it.

```
tiger service rename <service-id> <new-name> [flags]
```

### Examples

```
  # Rename a service
  tiger service rename svc-12345 analytics-prod
```

### Options

```
  -h, --help   help for rename
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
