## tiger service resize

Resize a database service

### Synopsis

Resize a database service by changing its CPU and memory allocation.

The service ID can be provided as an argument or will use the default service
from your configuration. This command changes the compute and memory resources
allocated to your database service.

The service may be temporarily unavailable during the resize operation. Note
that changing resources will affect your billing - increasing resources will
increase costs.

Examples:
  # Resize default service to 2 CPU cores and 8GB memory
  tiger service resize --cpu 2000 --memory 8

  # Resize specific service to 4 CPU cores and 16GB memory
  tiger service resize svc-12345 --cpu 4000 --memory 16

  # Resize service using only CPU (memory will be auto-configured to 8GB)
  tiger service resize --cpu 2000

  # Resize service using only memory (CPU will be auto-configured to 4000m)
  tiger service resize --memory 16

  # Resize without waiting for completion (waits by default)
  tiger service resize --cpu 2000 --memory 8 --no-wait

  # Resize with custom wait timeout
  tiger service resize --cpu 2000 --memory 8 --wait-timeout 45m

Allowed CPU/Memory Configurations:
  0.5 CPU (500m) / 2GB  |  1 CPU (1000m) / 4GB     |  2 CPU (2000m) / 8GB     |  4 CPU (4000m) / 16GB
  8 CPU (8000m) / 32GB  |  16 CPU (16000m) / 64GB  |  32 CPU (32000m) / 128GB

Note: You can specify both CPU and memory together, or specify only one (the other will be automatically configured).

```
tiger service resize [service-id] [flags]
```

### Options

```
      --cpu string              CPU allocation in millicores
  -h, --help                    help for resize
      --memory string           Memory allocation in gigabytes
      --no-wait                 Don't wait for resize operation to complete
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
