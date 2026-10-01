## tiger service metrics available

List available metric series

### Synopsis

List the names of all metric series available for a service.

The service can be given by ID or name as an argument, or will use the default
service from your configuration.

```
tiger service metrics available [name-or-id] [flags]
```

### Options

```
  -h, --help            help for available
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

* [tiger service metrics](tiger_service_metrics.md)	 - View service metrics
