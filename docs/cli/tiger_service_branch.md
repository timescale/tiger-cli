## tiger service branch

Create a branch of a database service

### Synopsis

Create a branch of a Fluid Service.

A branch is a lightweight, ephemeral copy of a service. It starts with a copy
of the service's data as of the moment it is created, and gets its own compute.
The tsdbadmin user gets a new password, which is saved the same way as a new
service's. Unless --cpu or --memory is given, the branch gets the same CPU and
memory as the service.

The service can be given by ID or name as an argument, or will use the default
service from your configuration. Branching from a branch isn't supported.

Allowed CPU/Memory Configurations:
  0.25 CPU / 2GB  |  0.5 CPU / 4GB  |  1 CPU / 8GB     |  2 CPU / 16GB
  4 CPU / 32GB    |  8 CPU / 64GB   |  16 CPU / 128GB  |  32 CPU / 256GB

The allowed --iops values depend on your plan and the branch's CPU/memory
allocation. See the Tiger Data documentation for details. If --iops is not
given, the branch gets the same IOPS as the service, unless --cpu or --memory
is given, in which case it gets the baseline for its size.

```
tiger service branch [name-or-id] [flags]
```

### Examples

```
  # Branch a service
  tiger service branch svc-12345

  # Branch a service with a custom name
  tiger service branch svc-12345 --name ci-4821

  # Branch a service with custom compute
  tiger service branch svc-12345 --cpu 2 --memory 16

  # Branch a service with provisioned IOPS
  tiger service branch svc-12345 --cpu 2 --iops 16000

  # Branch for a script
  tiger service branch svc-12345 --name ci-4821 -o json
```

### Options

```
      --cpu string              CPU allocation in cores
  -h, --help                    help for branch
      --iops int                Provisioned IOPS
      --memory string           Memory allocation in gigabytes
      --name string             Name for the branch (auto-generated if not provided)
      --no-wait                 Don't wait for the branch to be ready
  -o, --output string           Output format (json, yaml, env, table)
      --wait-timeout duration   Wait timeout duration (e.g., 30m, 1h30m, 90s) (default 10m0s)
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
