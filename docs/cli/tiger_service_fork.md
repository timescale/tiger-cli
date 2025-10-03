## tiger service fork

Fork an existing database service

### Synopsis

Fork an existing database service to create a new independent copy.

You must specify exactly one timing option for the fork strategy:
- --now: Fork at the current database state (creates new snapshot or uses WAL replay)
- --last-snapshot: Fork at the last existing snapshot (faster fork)
- --to-timestamp: Fork at a specific point in time (point-in-time recovery)

By default:
- Name will be auto-generated as '{source-service-name}-fork'
- CPU and memory will be inherited from the source service
- The forked service will be set as your default service

You can override any of these defaults with the corresponding flags.

Examples:
  # Fork a service at the current state
  tiger service fork svc-12345 --now

  # Fork a service at the last snapshot
  tiger service fork svc-12345 --last-snapshot

  # Fork a service at a specific point in time
  tiger service fork svc-12345 --to-timestamp 2025-01-15T10:30:00Z

  # Fork with custom name
  tiger service fork svc-12345 --now --name my-forked-db

  # Fork with custom resources
  tiger service fork svc-12345 --now --cpu 2000 --memory 8

  # Fork without setting as default service
  tiger service fork svc-12345 --now --no-set-default

  # Fork without waiting for completion
  tiger service fork svc-12345 --now --no-wait

  # Fork with custom wait timeout
  tiger service fork svc-12345 --now --wait-timeout 45m

```
tiger service fork [service-id] [flags]
```

### Options

```
      --cpu int                 CPU allocation in millicores (inherits from source if not specified)
  -h, --help                    help for fork
      --last-snapshot           Fork at the last existing snapshot (faster)
      --memory int              Memory allocation in gigabytes (inherits from source if not specified)
      --name string             Name for the forked service (defaults to '{source-name}-fork')
      --no-set-default          Don't set this service as the default service
      --no-wait                 Don't wait for fork operation to complete
      --now                     Fork at the current database state (creates new snapshot or uses WAL replay)
      --to-timestamp string     Fork at a specific point in time (RFC3339 format, e.g., 2025-01-15T10:30:00Z)
      --wait-timeout duration   Wait timeout duration (e.g., 30m, 1h30m, 90s) (default 30m0s)
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

