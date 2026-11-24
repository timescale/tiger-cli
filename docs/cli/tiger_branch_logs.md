## tiger branch logs

View logs for a branch

### Synopsis

View logs for a branch.

Fetches and displays logs from the specified branch. By default, shows the last
100 log entries. Supports filtering by time range.

The branch can be given by ID or name.

```
tiger branch logs <name-or-id> [flags]
```

### Examples

```
  # View last 100 logs for a branch (default behavior)
  tiger branch logs br-12345

  # View logs within a time range
  tiger branch logs br-12345 --since "2024-01-15T09:00:00Z" --until "2024-01-15T10:00:00Z"

  # View last 50 lines
  tiger branch logs br-12345 --tail 50

  # View last 1000 lines
  tiger branch logs br-12345 --tail 1000
```

### Options

```
  -h, --help            help for logs
  -o, --output string   Output format (text, json, yaml)
      --since time      Fetch logs after this timestamp (RFC3339 format, e.g., 2024-01-15T09:00:00Z)
      --tail int        Number of log lines to show (default 100)
      --until time      Fetch logs before this timestamp (RFC3339 format, e.g., 2024-01-15T10:00:00Z)
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
