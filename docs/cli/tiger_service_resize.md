## tiger service resize

Resize a database service

### Synopsis

Resize a database service by changing its CPU and memory allocation.

The service can be given by ID or name as an argument, or will use the default
service from your configuration. This command changes the compute and memory
resources allocated to your database service.

The command returns once the resize request is accepted, without waiting for
the resize to complete, which may take several minutes. The service may be
temporarily unavailable during the resize operation. Note that changing
resources will affect your billing - increasing resources will increase costs.

Allowed CPU/Memory Configurations:
  0.5 CPU / 2GB  |  1 CPU / 4GB    |  2 CPU / 8GB     |  4 CPU / 16GB
  8 CPU / 32GB   |  16 CPU / 64GB  |  32 CPU / 128GB

Note: You can specify both CPU and memory together, or specify only one (the
other will be automatically configured).

```
tiger service resize [name-or-id] [flags]
```

### Examples

```
  # Resize default service to 2 CPU cores and 8GB memory
  tiger service resize --cpu 2 --memory 8

  # Resize specific service to 4 CPU cores and 16GB memory
  tiger service resize svc-12345 --cpu 4 --memory 16

  # Resize service using only CPU (memory will be auto-configured to 8GB)
  tiger service resize --cpu 2

  # Resize service using only memory (CPU will be auto-configured to 4 cores)
  tiger service resize --memory 16
```

### Options

```
      --cpu string      CPU allocation in cores
  -h, --help            help for resize
      --memory string   Memory allocation in gigabytes
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
