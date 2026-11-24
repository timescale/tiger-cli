## tiger service resize

Resize a database service

### Synopsis

Resize a database service by changing its CPU, memory, and provisioned IOPS
allocation.

The service can be given by ID or name as an argument, or will use the default
service from your configuration. This command changes the compute and memory
resources allocated to your database service.

The command returns once the resize request is accepted, without waiting for
the resize to complete, which may take several minutes. The service may be
temporarily unavailable during the resize operation. Note that changing
resources will affect your billing - increasing resources will increase costs.

Allowed CPU/Memory Configurations (standard platform):
  0.5 CPU / 2GB  |  1 CPU / 4GB    |  2 CPU / 8GB     |  4 CPU / 16GB
  8 CPU / 32GB   |  16 CPU / 64GB  |  32 CPU / 128GB

Allowed CPU/Memory Configurations (fluid platform):
  0.25 CPU / 2GB  |  0.5 CPU / 4GB  |  1 CPU / 8GB     |  2 CPU / 16GB
  4 CPU / 32GB    |  8 CPU / 64GB   |  16 CPU / 128GB  |  32 CPU / 256GB

Note: You can specify both CPU and memory together, or specify only one (the
other will be automatically configured).

The allowed --iops values depend on your plan, the service's platform, and its
CPU/memory allocation. See the Tiger Data documentation for details. If only
--iops is given, the service keeps its current CPU and memory. If --cpu or
--memory is given without --iops on the fluid platform, the IOPS is set to
the baseline for the new size.

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

  # Change only the provisioned IOPS
  tiger service resize --iops 16000
```

### Options

```
      --cpu string      CPU allocation in cores
  -h, --help            help for resize
      --iops int        Provisioned IOPS
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
