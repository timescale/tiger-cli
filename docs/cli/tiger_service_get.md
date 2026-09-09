## tiger service get

Show detailed information about a service

### Synopsis

Show detailed information about a specific database service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command displays comprehensive information about
the service including configuration, status, endpoints, and resource usage.

Examples:
  # Get default service details
  tiger service get

  # Get specific service details
  tiger service get svc-12345

  # Get service details in JSON format
  tiger service get svc-12345 --output json

  # Get service details in YAML format
  tiger service get svc-12345 --output yaml

```
tiger service get [service-id] [flags]
```

### Options

```
  -h, --help            help for get
  -o, --output string   Output format (json, yaml, env, table)
      --with-password   Include password in output
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
