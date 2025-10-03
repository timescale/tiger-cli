## tiger service create

Create a new database service

### Synopsis

Create a new database service in the current project.

By default, the newly created service will be set as your default service for future
commands. Use --no-set-default to prevent this behavior.

Examples:
  # Create a TimescaleDB service with all defaults (0.5 CPU, 2GB, us-east-1, auto-generated name)
  tiger service create

  # Create a free TimescaleDB service
  tiger service create --name free-db --free

  # Create a TimescaleDB service with AI add-ons
  tiger service create --name hybrid-db --addons time-series,ai

  # Create a plain Postgres service
  tiger service create --name postgres-db --addons none

  # Create a service with more resources (waits for ready by default)
  tiger service create --name resources-db --cpu 2000 --memory 8 --replicas 2

  # Create service in a different region
  tiger service create --name eu-db --region eu-central-1

  # Create service without setting it as default
  tiger service create --name temp-db --no-set-default

  # Create service specifying only CPU (memory will be auto-configured to 8GB)
  tiger service create --name auto-memory --cpu 2000

  # Create service specifying only memory (CPU will be auto-configured to 4000m)
  tiger service create --name auto-cpu --memory 16

  # Create service without waiting for completion
  tiger service create --name quick-db --no-wait

  # Create service with custom wait timeout
  tiger service create --name patient-db --wait-timeout 1h

Free Tier:
  When using --free, resource flags (--cpu, --memory, --replicas, --addons, --region) cannot be specified

Allowed CPU/Memory Configurations (non-free services):
  0.5 CPU (500m) / 2GB    |  1 CPU (1000m) / 4GB    |  2 CPU (2000m) / 8GB    |  4 CPU (4000m) / 16GB
  8 CPU (8000m) / 32GB    |  16 CPU (16000m) / 64GB  |  32 CPU (32000m) / 128GB

Note: You can specify both CPU and memory together, or specify only one (the other will be automatically configured).

```
tiger service create [flags]
```

### Options

```
      --addons strings          Addons to enable (time-series, ai, or 'none' for PostgreSQL-only) (default [time-series])
      --cpu int                 CPU allocation in millicores (default 500)
      --free                    Create a free tier service (limitations apply)
  -h, --help                    help for create
      --memory int              Memory allocation in gigabytes (default 2)
      --name string             Service name (auto-generated if not provided)
      --no-set-default          Don't set this service as the default service
      --no-wait                 Don't wait for operation to complete
      --region string           Region code (default "us-east-1")
      --replicas int            Number of high-availability replicas
      --wait-timeout duration   Wait timeout duration (e.g., 30m, 1h30m, 90s) (default 30m0s)
      --with-password           Include initial password in output
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

