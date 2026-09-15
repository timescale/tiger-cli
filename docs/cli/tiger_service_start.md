## tiger service start

Start a stopped database service

### Synopsis

Start a stopped database service.

This operation starts a service that is currently in an inactive/stopped state. The service will transition to an active state and become available for connections.

```
tiger service start [service-id] [flags]
```

### Examples

```
  # Start a service (waits for completion by default)
  tiger service start svc-12345

  # Start service without waiting for completion
  tiger service start svc-12345 --no-wait

  # Start service with custom wait timeout
  tiger service start svc-12345 --wait-timeout 10m
```

### Options

```
  -h, --help                    help for start
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
