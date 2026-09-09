## tiger service stop

Stop a running database service

### Synopsis

Stop a running database service.

This operation stops a service that is currently active/running. The service will transition to an inactive state and will no longer accept connections.

Examples:
  # Stop a service (waits for completion by default)
  tiger service stop svc-12345

  # Stop service without waiting for completion
  tiger service stop svc-12345 --no-wait

  # Stop service with custom wait timeout
  tiger service stop svc-12345 --wait-timeout 10m

```
tiger service stop [service-id] [flags]
```

### Options

```
  -h, --help                    help for stop
      --no-wait                 Don't wait for the operation to complete
      --wait-timeout duration   Maximum time to wait for operation to complete (default 10m0s)
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
