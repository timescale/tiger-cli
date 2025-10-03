## tiger service describe

Show detailed information about a service

### Synopsis

Show detailed information about a specific database service.

The service ID can be provided as an argument or will use the default service
from your configuration. This command displays comprehensive information about
the service including configuration, status, endpoints, and resource usage.

Examples:
  # Describe default service
  tiger service describe

  # Describe specific service
  tiger service describe svc-12345

  # Get service details in JSON format
  tiger service describe svc-12345 --output json

  # Get service details in YAML format
  tiger service describe svc-12345 --output yaml

```
tiger service describe [service-id] [flags]
```

### Options

```
  -h, --help            help for describe
      --with-password   Include password in output
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

* [tiger service](tiger_service.md)	 - Manage database services

