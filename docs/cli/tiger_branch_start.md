## tiger branch start

Start a stopped branch

### Synopsis

Start a stopped branch.

Starting a branch resumes charging for its compute.

The branch can be given by ID or name.

```
tiger branch start <name-or-id> [flags]
```

### Examples

```
  # Start a branch (waits for completion by default)
  tiger branch start br-12345

  # Start a branch without waiting for completion
  tiger branch start br-12345 --no-wait
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

* [tiger branch](tiger_branch.md)	 - Manage branches of database services
