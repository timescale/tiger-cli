## tiger branch resize

Resize a branch

### Synopsis

Resize a branch by changing its CPU, memory, and provisioned IOPS allocation.

The branch can be given by ID or name. The command returns once the resize
request is accepted, without waiting for the resize to complete, which may take
several minutes. The branch may be temporarily unavailable during the resize.

Allowed CPU/Memory Configurations:
  0.25 CPU / 2GB  |  0.5 CPU / 4GB  |  1 CPU / 8GB     |  2 CPU / 16GB
  4 CPU / 32GB    |  8 CPU / 64GB   |  16 CPU / 128GB  |  32 CPU / 256GB

Note: You can specify both CPU and memory together, or specify only one (the
other will be automatically configured).

The allowed --iops values depend on your plan and the branch's CPU/memory
allocation. See the Tiger Data documentation for details. If only --iops is
given, the branch keeps its current CPU and memory. If --cpu or --memory is
given without --iops, the IOPS is set to the baseline for the new size.

```
tiger branch resize <name-or-id> [flags]
```

### Examples

```
  # Resize a branch to 2 CPU cores and 16GB memory
  tiger branch resize br-12345 --cpu 2 --memory 16

  # Resize a branch using only CPU (memory will be auto-configured)
  tiger branch resize br-12345 --cpu 2

  # Change only the provisioned IOPS
  tiger branch resize br-12345 --iops 16000
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

* [tiger branch](tiger_branch.md)	 - Manage branches of database services
