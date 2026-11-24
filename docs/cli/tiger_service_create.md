## tiger service create

Create a new database service

### Synopsis

Create a new database service in the current project.

The default type of service created depends on your plan:
- Free plan: Creates a service with shared CPU/memory and the 'time-series' add-on
- Paid plans: Creates a service with 0.5 CPU / 2 GB memory and the 'time-series' add-on

Services on the fluid platform (--platform fluid) are created with
0.25 CPU / 2 GB memory by default.

By default, the newly created service will be set as your default service for
future commands. Use --no-set-default to prevent this behavior.

Allowed CPU/Memory Configurations (standard platform):
  shared / shared  |  0.5 CPU / 2GB  |  1 CPU / 4GB    |  2 CPU / 8GB
  4 CPU / 16GB     |  8 CPU / 32GB   |  16 CPU / 64GB  |  32 CPU / 128GB

Allowed CPU/Memory Configurations (fluid platform):
  0.25 CPU / 2GB  |  0.5 CPU / 4GB  |  1 CPU / 8GB     |  2 CPU / 16GB
  4 CPU / 32GB    |  8 CPU / 64GB   |  16 CPU / 128GB  |  32 CPU / 256GB

Note: You can specify both CPU and memory together, or specify only one (the
other will be automatically configured).

The allowed --iops values depend on your plan, the service's platform, and its
CPU/memory allocation. See the Tiger Data documentation for details. If --iops
is not given, the service gets the baseline IOPS for your plan and its CPU
allocation.

```
tiger service create [flags]
```

### Examples

```
  # Create a TimescaleDB service with all defaults (0.5 CPU, 2GB, us-east-1, auto-generated name)
  tiger service create

  # Create a free TimescaleDB service
  tiger service create --name free-db --cpu shared

  # Create a plain Postgres service
  tiger service create --name postgres-db --addons none

  # Create a service with more resources (waits for ready by default)
  tiger service create --name resources-db --cpu 2 --memory 8 --replicas 2

  # Create service in a different region
  tiger service create --name eu-db --region eu-central-1

  # Create service without setting it as default
  tiger service create --name temp-db --no-set-default

  # Create service specifying only CPU (memory will be auto-configured to 8GB)
  tiger service create --name auto-memory --cpu 2

  # Create service specifying only memory (CPU will be auto-configured to 4 cores)
  tiger service create --name auto-cpu --memory 16

  # Create service without waiting for completion
  tiger service create --name quick-db --no-wait

  # Create service with custom wait timeout
  tiger service create --name patient-db --wait-timeout 1h

  # Create a service on the fluid platform
  tiger service create --name fluid-db --platform fluid

  # Create a service with provisioned IOPS
  tiger service create --name fast-db --cpu 2 --iops 16000
```

### Options

```
      --addons strings          Addons to enable (time-series, or 'none' for PostgreSQL-only)
      --cpu string              CPU allocation in cores or 'shared'
      --environment string      Environment tag (DEV or PROD) (default "DEV")
  -h, --help                    help for create
      --iops int                Provisioned IOPS
      --memory string           Memory allocation in gigabytes or 'shared'
      --name string             Service name (auto-generated if not provided)
      --no-set-default          Don't set this service as the default service
      --no-wait                 Don't wait for operation to complete
  -o, --output string           Output format (json, yaml, env, table)
      --platform string         Storage platform (standard or fluid) (default "standard")
      --region string           Region code
      --replicas int            Number of high-availability replicas
      --wait-timeout duration   Wait timeout duration (e.g., 30m, 1h30m, 90s) (default 30m0s)
      --with-password           Include password in output
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
