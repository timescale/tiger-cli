## tiger branch list

List the branches of a service

### Synopsis

List the branches of a database service.

The service can be given by ID or name as an argument, or will use the default
service from your configuration.

```
tiger branch list [service-name-or-id] [flags]
```

### Examples

```
  # List the default service's branches
  tiger branch list

  # List a specific service's branches
  tiger branch list svc-12345
```

### Options

```
  -h, --help            help for list
  -o, --output string   Output format (json, yaml, table)
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

* [tiger branch](tiger_branch.md)	 - Manage branches of database services
