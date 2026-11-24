## tiger branch get

Show detailed information about a branch

### Synopsis

Show detailed information about a branch, including its parent service, status,
compute, and connection details.

The branch can be given by ID or name.

```
tiger branch get <name-or-id> [flags]
```

### Examples

```
  # Get a branch's details
  tiger branch get br-12345

  # Get branch details in JSON format
  tiger branch get br-12345 --output json

  # Include the stored password
  tiger branch get br-12345 --with-password
```

### Options

```
  -h, --help            help for get
  -o, --output string   Output format (json, yaml, env, table)
      --with-password   Include password in output
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
